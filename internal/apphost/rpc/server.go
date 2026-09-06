package rpc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// shellCallTimeout bounds a request the application makes into the shell, so a
// wedged or disconnected shell cannot block an application goroutine forever.
const shellCallTimeout = 5 * time.Minute

const (
	// frameCall asks the shell to run something and, when it carries an id,
	// to answer with a frameResult.
	frameCall   = "call"
	frameEvent  = "event"
	frameResult = "result"

	// dialogTitle is the parameter name every dialog request uses.
	dialogTitle = "title"
)

// Handshake is the single line the sidecar prints on stdout once it is
// listening. The shell reads it to learn where to connect and how to authorize.
type Handshake struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// Server serves the application to an out-of-process shell and, in the other
// direction, implements app.Host on top of the shell's WebSocket connection.
type Server struct {
	dispatcher *dispatcher
	token      string
	logger     *slog.Logger

	listener net.Listener
	http     *http.Server

	shellMu sync.RWMutex
	shell   *websocket.Conn
	writeMu sync.Mutex

	pending *pending
}

// New binds an application to a new server. The caller must call Listen and
// Serve; the returned server is already usable as an app.Host.
func New(application any, logger *slog.Logger) (*Server, error) {
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		dispatcher: newDispatcher(application),
		token:      token,
		logger:     logger,
		pending:    newPending(),
	}, nil
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate shell token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// Listen binds a loopback port and returns the handshake the shell needs.
func (s *Server) Listen() (Handshake, error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return Handshake{}, fmt.Errorf("listen on loopback: %w", err)
	}
	s.listener = listener
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return Handshake{}, errors.New("loopback listener did not report a TCP address")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rpc", s.handleRPC)
	mux.HandleFunc("GET /shell", s.handleShell)
	s.http = &http.Server{Handler: s.authorize(mux), ReadHeaderTimeout: 10 * time.Second}
	return Handshake{Port: address.Port, Token: s.token}, nil
}

// Serve blocks until the server is closed.
func (s *Server) Serve() error {
	if s.http == nil || s.listener == nil {
		return errors.New("Listen must be called before Serve")
	}
	if err := s.http.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Close stops serving and releases every waiting shell call.
func (s *Server) Close(ctx context.Context) error {
	s.pending.failAll(errors.New("desktop shell is shutting down"))
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// authorize rejects any request that does not carry the launch token. The
// listener is already loopback-only; the token additionally keeps other local
// processes and any browser page off the endpoint.
func (s *Server) authorize(next http.Handler) http.Handler {
	expected := []byte("Bearer " + s.token)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		presented := request.Header.Get("Authorization")
		if presented == "" {
			// A browser cannot set headers on a WebSocket handshake, so the
			// shell passes the token as the negotiated subprotocol instead.
			presented = "Bearer " + request.Header.Get("Sec-WebSocket-Protocol")
		}
		if subtle.ConstantTimeCompare([]byte(presented), expected) != 1 {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

type callRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

type callResponse struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) handleRPC(writer http.ResponseWriter, request *http.Request) {
	var call callRequest
	if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
		writeJSON(writer, http.StatusBadRequest, callResponse{Error: "decode request: " + err.Error()})
		return
	}
	result, err := s.dispatcher.call(call.Method, call.Args)
	if err != nil {
		writeJSON(writer, http.StatusOK, callResponse{Error: err.Error()})
		return
	}
	writeJSON(writer, http.StatusOK, callResponse{Result: result})
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(body); err != nil {
		slog.Default().Warn("desktop shell response failed", "error", err)
	}
}

// Methods lists every method the shell can call. It backs a discovery endpoint
// used by tests to guard the bridge against an unreachable binding.
func (s *Server) Methods() []string { return s.dispatcher.names() }

// --- shell connection ---

type shellFrame struct {
	Type   string          `json:"type"`
	ID     uint64          `json:"id,omitempty"`
	Event  string          `json:"event,omitempty"`
	Method string          `json:"method,omitempty"`
	Params any             `json:"params,omitempty"`
	Data   any             `json:"data,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func (s *Server) handleShell(writer http.ResponseWriter, request *http.Request) {
	upgrader := websocket.Upgrader{
		Subprotocols: []string{request.Header.Get("Sec-WebSocket-Protocol")},
		CheckOrigin: func(*http.Request) bool {
			// The token check in authorize already gates this endpoint, and the
			// Electron renderer's origin is not a stable value to match on.
			return true
		},
	}
	connection, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		s.logger.Warn("desktop shell connection rejected", "error", err)
		return
	}

	s.shellMu.Lock()
	previous := s.shell
	s.shell = connection
	s.shellMu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}

	defer func() {
		s.shellMu.Lock()
		if s.shell == connection {
			s.shell = nil
		}
		s.shellMu.Unlock()
		_ = connection.Close()
		s.pending.failAll(errors.New("desktop shell disconnected"))
	}()

	for {
		var frame shellFrame
		if err := connection.ReadJSON(&frame); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				s.logger.Warn("desktop shell connection closed", "error", err)
			}
			return
		}
		if frame.Type != frameResult {
			continue
		}
		result := shellResult{value: frame.Result}
		if frame.Error != "" {
			result.err = errors.New(frame.Error)
		}
		s.pending.complete(frame.ID, result)
	}
}

func (s *Server) connection() *websocket.Conn {
	s.shellMu.RLock()
	defer s.shellMu.RUnlock()
	return s.shell
}

func (s *Server) send(frame shellFrame) error {
	connection := s.connection()
	if connection == nil {
		return errors.New("desktop shell is not connected")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return connection.WriteJSON(frame)
}

// invoke asks the shell to run a method and waits for its reply.
func (s *Server) invoke(method string, params any, result any) error {
	id, waiter := s.pending.begin()
	if err := s.send(shellFrame{Type: frameCall, ID: id, Method: method, Params: params}); err != nil {
		s.pending.cancel(id)
		return err
	}
	timer := time.NewTimer(shellCallTimeout)
	defer timer.Stop()
	select {
	case reply := <-waiter:
		if reply.err != nil {
			return reply.err
		}
		if result == nil || len(reply.value) == 0 {
			return nil
		}
		return json.Unmarshal(reply.value, result)
	case <-timer.C:
		s.pending.cancel(id)
		return fmt.Errorf("desktop shell did not answer %s in %s", method, shellCallTimeout)
	}
}

// --- app.Host ---

func (s *Server) Emit(event string, payload any) {
	if err := s.send(shellFrame{Type: frameEvent, Event: event, Data: payload}); err != nil {
		s.logger.Debug("dropped event for disconnected shell", "event", event, "error", err)
	}
}

func (s *Server) ShowWindow() {
	if err := s.send(shellFrame{Type: frameCall, Method: "window.show"}); err != nil {
		s.logger.Warn("show window failed", "error", err)
	}
}

func (s *Server) Quit() {
	if err := s.send(shellFrame{Type: frameCall, Method: "app.quit"}); err != nil {
		s.logger.Warn("quit failed", "error", err)
	}
}

func (s *Server) OpenURL(target string) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("cannot open an empty URL")
	}
	return s.invoke("shell.openExternal", map[string]any{"url": target}, nil)
}

func (s *Server) OpenFileDialog(title string) (string, error) {
	var path string
	err := s.invoke("dialog.openFile", map[string]any{dialogTitle: title}, &path)
	return path, err
}

func (s *Server) OpenDirectoryDialog(title string) (string, error) {
	var path string
	err := s.invoke("dialog.openDirectory", map[string]any{dialogTitle: title}, &path)
	return path, err
}

func (s *Server) SaveFileDialog(title, defaultFilename string) (string, error) {
	var path string
	err := s.invoke("dialog.saveFile", map[string]any{
		dialogTitle: title, "defaultFilename": defaultFilename,
	}, &path)
	return path, err
}
