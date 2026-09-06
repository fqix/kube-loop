package rpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func startServer(t *testing.T, application any) (*Server, Handshake) {
	t.Helper()
	server, err := New(application, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	handshake, err := server.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	go func() { _ = server.Serve() }()
	t.Cleanup(func() { _ = server.Close(t.Context()) })
	return server, handshake
}

func post(t *testing.T, handshake Handshake, token, body string) callResponse {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/rpc", handshake.Port)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call /rpc: %v", err)
	}
	defer response.Body.Close()
	if token != handshake.Token {
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status with a bad token = %d, want 401", response.StatusCode)
		}
		return callResponse{}
	}
	var decoded callResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return decoded
}

func TestServerDispatchesOverHTTP(t *testing.T) {
	_, handshake := startServer(t, &sample{})

	got := post(t, handshake, handshake.Token, `{"method":"Greet","args":["world"]}`)
	if got.Error != "" || got.Result != "hello world" {
		t.Fatalf("Greet response = %#v", got)
	}

	got = post(t, handshake, handshake.Token, `{"method":"Fail","args":[]}`)
	if got.Error != "always fails" {
		t.Fatalf("Fail response = %#v, want the method error", got)
	}
}

func TestServerRejectsUnauthorizedCalls(t *testing.T) {
	_, handshake := startServer(t, &sample{})
	post(t, handshake, "not-the-token", `{"method":"Greet","args":["world"]}`)
}

func dialShell(t *testing.T, handshake Handshake) *websocket.Conn {
	t.Helper()
	url := fmt.Sprintf("ws://127.0.0.1:%d/shell", handshake.Port)
	dialer := websocket.Dialer{Subprotocols: []string{handshake.Token}, HandshakeTimeout: 5 * time.Second}
	connection, _, err := dialer.DialContext(t.Context(), url, nil)
	if err != nil {
		t.Fatalf("dial shell: %v", err)
	}
	t.Cleanup(func() { connection.Close() })
	return connection
}

func TestServerEmitsEventsToShell(t *testing.T) {
	server, handshake := startServer(t, &sample{})
	shell := dialShell(t, handshake)

	// The shell registers asynchronously; retry until the first event lands.
	deadline := time.Now().Add(5 * time.Second)
	for server.connection() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	server.Emit("update:state", map[string]any{"available": true})

	if err := shell.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var frame shellFrame
	if err := shell.ReadJSON(&frame); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if frame.Type != "event" || frame.Event != "update:state" {
		t.Fatalf("frame = %#v, want an update:state event", frame)
	}
}

func TestServerCallsIntoShellAndWaits(t *testing.T) {
	server, handshake := startServer(t, &sample{})
	shell := dialShell(t, handshake)

	answered := make(chan struct{})
	go func() {
		defer close(answered)
		var frame shellFrame
		if err := shell.ReadJSON(&frame); err != nil {
			return
		}
		if frame.Method != "dialog.openFile" {
			return
		}
		_ = shell.WriteJSON(shellFrame{Type: "result", ID: frame.ID, Result: json.RawMessage(`"/tmp/pick.txt"`)})
	}()

	deadline := time.Now().Add(5 * time.Second)
	for server.connection() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	path, err := server.OpenFileDialog("Select file to upload")
	if err != nil || path != "/tmp/pick.txt" {
		t.Fatalf("OpenFileDialog = %q, %v", path, err)
	}
	<-answered
}

func TestShellCallFailsWithoutAShell(t *testing.T) {
	server, _ := startServer(t, &sample{})

	if _, err := server.OpenFileDialog("Select"); err == nil {
		t.Fatal("OpenFileDialog succeeded without a connected shell")
	}
	if err := server.OpenURL("https://example.test"); err == nil {
		t.Fatal("OpenURL succeeded without a connected shell")
	}
	if err := server.OpenURL("  "); err == nil {
		t.Fatal("OpenURL accepted an empty target")
	}
	// Fire-and-forget shell calls must not panic when nothing is listening.
	server.Emit("update:state", nil)
	server.ShowWindow()
	server.Quit()
}
