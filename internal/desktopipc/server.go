package desktopipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// maxLineBytes bounds a single frame. File listings and inventories are the
// largest payloads and stay far below this.
const maxLineBytes = 64 << 20

// Server runs the backend side of the protocol over a reader/writer pair,
// normally the process stdin and stdout. It implements the application Host
// interface so the Go side can push events and ask the shell for dialogs.
type Server struct {
	dispatcher *Dispatcher
	reader     *bufio.Reader
	writeMu    sync.Mutex
	writer     io.Writer
	logger     *slog.Logger

	nextID  atomic.Int64
	pending sync.Map // id string → chan *message

	onShutdown func(context.Context)
	inflight   sync.WaitGroup
	closed     chan struct{}
	closeOnce  sync.Once
}

// Option configures a Server.
type Option func(*Server)

// WithLogger sets the logger used for protocol diagnostics.
func WithLogger(logger *slog.Logger) Option {
	return func(server *Server) { server.logger = logger }
}

// WithShutdownHandler runs when the shell requests BackendMethodShutdown. The
// server replies once the handler returns and then stops serving.
func WithShutdownHandler(handler func(context.Context)) Option {
	return func(server *Server) { server.onShutdown = handler }
}

// NewServer wires a dispatcher to the transport. The dispatcher must already
// have its targets bound.
func NewServer(dispatcher *Dispatcher, reader io.Reader, writer io.Writer, options ...Option) *Server {
	server := &Server{
		dispatcher: dispatcher,
		reader:     bufio.NewReaderSize(reader, 1<<20),
		writer:     writer,
		logger:     slog.Default(),
		closed:     make(chan struct{}),
	}
	for _, option := range options {
		option(server)
	}
	return server
}

// Serve announces readiness and processes frames until the reader closes, the
// context is cancelled or the shell requests shutdown. Requests run
// concurrently; bound methods must be safe for that, as they were under the
// previous binding layer.
func (server *Server) Serve(ctx context.Context) error {
	if err := server.notify(HostMethodReady, ReadyParams{Methods: server.dispatcher.Methods()}); err != nil {
		return fmt.Errorf("announce ready: %w", err)
	}
	lines := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		defer close(lines)
		for {
			line, err := server.readFrame()
			if err != nil {
				readErr <- err
				return
			}
			select {
			case lines <- line:
			case <-server.closed:
				return
			}
		}
	}()
	defer server.inflight.Wait()
	for {
		select {
		case <-ctx.Done():
			server.close()
			return ctx.Err()
		case <-server.closed:
			return nil
		case line, ok := <-lines:
			if !ok {
				err := <-readErr
				server.close()
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
			server.handleFrame(ctx, line)
		}
	}
}

func (server *Server) readFrame() ([]byte, error) {
	var frame []byte
	for {
		chunk, isPrefix, err := server.reader.ReadLine()
		if err != nil {
			return nil, err
		}
		if len(frame)+len(chunk) > maxLineBytes {
			return nil, fmt.Errorf("frame exceeds %d bytes", maxLineBytes)
		}
		frame = append(frame, chunk...)
		if !isPrefix {
			return frame, nil
		}
	}
}

func (server *Server) close() {
	server.closeOnce.Do(func() { close(server.closed) })
}

func (server *Server) handleFrame(ctx context.Context, line []byte) {
	var incoming message
	if err := json.Unmarshal(line, &incoming); err != nil {
		_ = server.reply(nil, nil, newError(CodeParseError, "%v", err))
		return
	}
	switch {
	case incoming.isResponse():
		server.settle(&incoming)
	case incoming.isRequest():
		if incoming.Method == BackendMethodShutdown {
			server.handleShutdown(ctx, incoming.ID)
			return
		}
		server.inflight.Go(func() {
			result, rpcErr := server.dispatcher.Call(incoming.Method, incoming.Params)
			_ = server.reply(incoming.ID, result, rpcErr)
		})
	case incoming.isNotify():
		server.logger.Warn("ignoring unexpected notification", "method", incoming.Method)
	default:
		_ = server.reply(incoming.ID, nil, newError(CodeInvalidRequest, "message is neither request nor response"))
	}
}

func (server *Server) handleShutdown(ctx context.Context, id *json.RawMessage) {
	if server.onShutdown != nil {
		server.onShutdown(ctx)
	}
	_ = server.reply(id, json.RawMessage("null"), nil)
	server.close()
}

func (server *Server) settle(response *message) {
	key := string(*response.ID)
	value, ok := server.pending.LoadAndDelete(key)
	if !ok {
		server.logger.Warn("response for unknown request", "id", key)
		return
	}
	waiter, _ := value.(chan *message)
	waiter <- response
}

func (server *Server) write(payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	server.writeMu.Lock()
	defer server.writeMu.Unlock()
	if _, err := server.writer.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func (server *Server) reply(id *json.RawMessage, result json.RawMessage, rpcErr *Error) error {
	response := message{JSONRPC: jsonRPCVersion, ID: id}
	if id == nil {
		null := json.RawMessage("null")
		response.ID = &null
	}
	if rpcErr != nil {
		response.Error = rpcErr
	} else {
		response.Result = result
	}
	return server.write(response)
}

func (server *Server) notify(method string, params any) error {
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return server.write(message{JSONRPC: jsonRPCVersion, Method: method, Params: encoded})
}

// request sends a host request and blocks until the shell answers or the
// server stops. A nil timeout waits indefinitely, which dialogs rely on.
func (server *Server) request(method string, params any, result any, timeout time.Duration) error {
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	id := strconv.FormatInt(server.nextID.Add(1), 10)
	rawID := json.RawMessage(strconv.Quote(id))
	waiter := make(chan *message, 1)
	server.pending.Store(strconv.Quote(id), waiter)
	if err := server.write(message{JSONRPC: jsonRPCVersion, ID: &rawID, Method: method, Params: encoded}); err != nil {
		server.pending.Delete(strconv.Quote(id))
		return err
	}
	var timer <-chan time.Time
	if timeout > 0 {
		ticker := time.NewTimer(timeout)
		defer ticker.Stop()
		timer = ticker.C
	}
	select {
	case response := <-waiter:
		if response.Error != nil {
			return response.Error
		}
		if result == nil || len(response.Result) == 0 {
			return nil
		}
		return json.Unmarshal(response.Result, result)
	case <-timer:
		server.pending.Delete(strconv.Quote(id))
		return fmt.Errorf("%s: shell did not answer within %s", method, timeout)
	case <-server.closed:
		server.pending.Delete(strconv.Quote(id))
		return errors.New("desktop shell disconnected")
	}
}

const hostRequestTimeout = 10 * time.Second

// Emit implements app.Host.
func (server *Server) Emit(name string, payload any) {
	if err := server.notify(HostMethodEvent, EventParams{Name: name, Payload: payload}); err != nil {
		server.logger.Warn("emit event failed", "event", name, "error", err)
	}
}

// OpenURL implements app.Host.
func (server *Server) OpenURL(target string) error {
	return server.request(HostMethodOpenURL, OpenURLParams{URL: target}, nil, hostRequestTimeout)
}

// ShowWindow implements app.Host.
func (server *Server) ShowWindow() {
	if err := server.notify(HostMethodShowWindow, struct{}{}); err != nil {
		server.logger.Warn("show window failed", "error", err)
	}
}

// Quit implements app.Host.
func (server *Server) Quit() {
	if err := server.notify(HostMethodQuit, struct{}{}); err != nil {
		server.logger.Warn("quit request failed", "error", err)
	}
}

// OpenFileDialog implements app.Host.
func (server *Server) OpenFileDialog(title string) (string, error) {
	return server.dialog(HostMethodOpenFileDialog, DialogParams{Title: title})
}

// OpenDirectoryDialog implements app.Host.
func (server *Server) OpenDirectoryDialog(title string) (string, error) {
	return server.dialog(HostMethodOpenDirectoryDialog, DialogParams{Title: title})
}

// SaveFileDialog implements app.Host.
func (server *Server) SaveFileDialog(title, defaultName string) (string, error) {
	return server.dialog(HostMethodSaveFileDialog, DialogParams{Title: title, DefaultName: defaultName})
}

func (server *Server) dialog(method string, params DialogParams) (string, error) {
	var result DialogResult
	if err := server.request(method, params, &result, 0); err != nil {
		return "", err
	}
	return result.Path, nil
}
