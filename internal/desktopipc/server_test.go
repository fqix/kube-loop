package desktopipc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// frameSink is an asynchronous writer so the server never blocks on the test
// reading its output, unlike a synchronous io.Pipe.
type frameSink struct {
	frames chan string
}

func (sink *frameSink) Write(payload []byte) (int, error) {
	sink.frames <- string(payload)
	return len(payload), nil
}

// shellHarness plays the Electron side over in-memory transports.
type shellHarness struct {
	t        *testing.T
	toServer io.WriteCloser
	sink     *frameSink
	writeMu  sync.Mutex
	server   *Server
	done     chan error
	stopOnce sync.Once
	stopErr  error
}

// waitStopped returns Serve's result once, blocking until the server exits.
func (h *shellHarness) waitStopped() (bool, error) {
	stopped := true
	h.stopOnce.Do(func() {
		select {
		case h.stopErr = <-h.done:
		case <-time.After(5 * time.Second):
			stopped = false
		}
	})
	return stopped, h.stopErr
}

func newShellHarness(t *testing.T, target any, options ...Option) *shellHarness {
	t.Helper()
	dispatcher := NewDispatcher()
	if err := dispatcher.Bind(target, "Close"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	serverIn, shellOut := io.Pipe()
	sink := &frameSink{frames: make(chan string, 64)}
	server := NewServer(dispatcher, serverIn, sink, options...)
	harness := &shellHarness{
		t: t, toServer: shellOut, sink: sink, server: server, done: make(chan error, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { harness.done <- server.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = shellOut.Close()
		if stopped, _ := harness.waitStopped(); !stopped {
			t.Error("server did not stop")
		}
	})
	return harness
}

func (h *shellHarness) send(frame string) {
	h.t.Helper()
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	if _, err := io.WriteString(h.toServer, frame+"\n"); err != nil {
		h.t.Fatalf("write: %v", err)
	}
}

func (h *shellHarness) read() message {
	h.t.Helper()
	var line string
	select {
	case line = <-h.sink.frames:
	case <-time.After(5 * time.Second):
		h.t.Fatal("no frame from server")
	}
	var decoded message
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		h.t.Fatalf("decode %q: %v", line, err)
	}
	return decoded
}

func (h *shellHarness) expectReady() {
	h.t.Helper()
	ready := h.read()
	if ready.Method != HostMethodReady {
		h.t.Fatalf("first frame = %+v, want ready", ready)
	}
}

func TestServerAnnouncesBoundMethods(t *testing.T) {
	harness := newShellHarness(t, &sampleTarget{})
	ready := harness.read()
	var params ReadyParams
	if err := json.Unmarshal(ready.Params, &params); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(params.Methods, ","), "Echo") {
		t.Fatalf("ready methods = %v", params.Methods)
	}
}

func TestServerAnswersRequests(t *testing.T) {
	harness := newShellHarness(t, &sampleTarget{})
	harness.expectReady()
	harness.send(`{"jsonrpc":"2.0","id":7,"method":"Sum","params":[4,5]}`)
	response := harness.read()
	if string(*response.ID) != "7" || string(response.Result) != "9" {
		t.Fatalf("response = %+v", response)
	}
	harness.send(`{"jsonrpc":"2.0","id":"x","method":"Fails","params":[]}`)
	response = harness.read()
	if response.Error == nil || response.Error.Code != CodeApplication || response.Error.Message != "boom" {
		t.Fatalf("response = %+v", response)
	}
}

func TestServerReportsParseErrors(t *testing.T) {
	harness := newShellHarness(t, &sampleTarget{})
	harness.expectReady()
	harness.send(`{not json`)
	response := harness.read()
	if response.Error == nil || response.Error.Code != CodeParseError {
		t.Fatalf("response = %+v", response)
	}
}

func TestServerHostRoundTrip(t *testing.T) {
	harness := newShellHarness(t, &sampleTarget{})
	harness.expectReady()

	result := make(chan string, 1)
	go func() {
		path, err := harness.server.OpenFileDialog("Pick")
		if err != nil {
			t.Error(err)
		}
		result <- path
	}()
	request := harness.read()
	if request.Method != HostMethodOpenFileDialog || request.ID == nil {
		t.Fatalf("request = %+v", request)
	}
	var params DialogParams
	if err := json.Unmarshal(request.Params, &params); err != nil || params.Title != "Pick" {
		t.Fatalf("params = %s, err %v", request.Params, err)
	}
	harness.send(`{"jsonrpc":"2.0","id":` + string(*request.ID) + `,"result":{"path":"/tmp/file"}}`)
	select {
	case path := <-result:
		if path != "/tmp/file" {
			t.Fatalf("path = %q", path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dialog result never delivered")
	}
}

func TestServerEmitsEvents(t *testing.T) {
	harness := newShellHarness(t, &sampleTarget{})
	harness.expectReady()
	harness.server.Emit("session:state", map[string]int{"count": 1})
	event := harness.read()
	if event.Method != HostMethodEvent || event.ID != nil {
		t.Fatalf("event = %+v", event)
	}
	var params EventParams
	if err := json.Unmarshal(event.Params, &params); err != nil || params.Name != "session:state" {
		t.Fatalf("params = %s, err %v", event.Params, err)
	}
}

func TestServerShutdownRunsHandlerThenStops(t *testing.T) {
	handled := make(chan struct{})
	harness := newShellHarness(t, &sampleTarget{}, WithShutdownHandler(func(context.Context) { close(handled) }))
	harness.expectReady()
	harness.send(`{"jsonrpc":"2.0","id":1,"method":"backend.shutdown"}`)
	response := harness.read()
	if string(*response.ID) != "1" || response.Error != nil {
		t.Fatalf("response = %+v", response)
	}
	select {
	case <-handled:
	default:
		t.Fatal("shutdown handler did not run before the reply")
	}
	if stopped, err := harness.waitStopped(); !stopped || err != nil {
		t.Fatalf("Serve stopped=%v err=%v after shutdown", stopped, err)
	}
}

func TestServerStopsWhenShellCloses(t *testing.T) {
	harness := newShellHarness(t, &sampleTarget{})
	harness.expectReady()
	_ = harness.toServer.Close()
	if stopped, err := harness.waitStopped(); !stopped || err != nil {
		t.Fatalf("Serve stopped=%v err=%v after EOF", stopped, err)
	}
}
