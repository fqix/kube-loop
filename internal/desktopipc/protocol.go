// Package desktopipc carries the desktop shell ↔ Go backend protocol. The Go
// backend runs as a child process of the Electron main process and speaks
// newline-delimited JSON-RPC 2.0 over its stdin/stdout. No socket or port is
// opened, so nothing else on the machine can reach the backend.
//
// Both sides may send requests. The shell calls bound application methods by
// name; the backend calls a small set of host methods (dialogs, browser) and
// pushes "event" notifications that the renderer subscribes to.
package desktopipc

import (
	"encoding/json"
	"fmt"
)

const jsonRPCVersion = "2.0"

// Method names the backend sends to the shell.
const (
	HostMethodEvent               = "host.event"
	HostMethodOpenURL             = "host.openURL"
	HostMethodShowWindow          = "host.showWindow"
	HostMethodQuit                = "host.quit"
	HostMethodOpenFileDialog      = "host.openFileDialog"
	HostMethodOpenDirectoryDialog = "host.openDirectoryDialog"
	HostMethodSaveFileDialog      = "host.saveFileDialog"
	// HostMethodReady is sent once the backend has bound its methods and is
	// accepting requests.
	HostMethodReady = "host.ready"
)

// Method names the shell sends to the backend besides bound app methods.
const (
	BackendMethodShutdown = "backend.shutdown"
)

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
	// CodeApplication carries an error returned by a bound method.
	CodeApplication = -32000
)

// message is the wire shape shared by requests, notifications and responses.
type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
}

func (m *message) isRequest() bool  { return m.Method != "" && m.ID != nil }
func (m *message) isNotify() bool   { return m.Method != "" && m.ID == nil }
func (m *message) isResponse() bool { return m.Method == "" && m.ID != nil }

// Error is the JSON-RPC error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// EventParams is the payload of a HostMethodEvent notification.
type EventParams struct {
	Name    string `json:"name"`
	Payload any    `json:"payload"`
}

// DialogParams is shared by the file and directory dialog host requests.
type DialogParams struct {
	Title       string `json:"title"`
	DefaultName string `json:"defaultName,omitempty"`
}

// DialogResult is returned by all dialog host requests. Path is empty when the
// user cancels.
type DialogResult struct {
	Path string `json:"path"`
}

// OpenURLParams is the payload of a HostMethodOpenURL request.
type OpenURLParams struct {
	URL string `json:"url"`
}

// ReadyParams is the payload of HostMethodReady.
type ReadyParams struct {
	Methods []string `json:"methods"`
}

func newError(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
