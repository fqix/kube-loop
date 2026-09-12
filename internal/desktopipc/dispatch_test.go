package desktopipc

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type sampleHost interface{ Ping() }

type sampleRequest struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type sampleTarget struct{}

func (s *sampleTarget) Plain()                                            {}
func (s *sampleTarget) Fails() error                                      { return errors.New("boom") }
func (s *sampleTarget) Version() string                                   { return "1.2.3" }
func (s *sampleTarget) Echo(request sampleRequest) (sampleRequest, error) { return request, nil }
func (s *sampleTarget) Sum(left, right int) int                           { return left + right }
func (s *sampleTarget) SetHost(sampleHost)                                {}
func (s *sampleTarget) Close()                                            {}
func (s *sampleTarget) Both() (string, int)                               { return "", 0 }

func newTestDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	target := &sampleTarget{}
	dispatcher := NewDispatcher()
	if err := dispatcher.Bind(target, "Close"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	return dispatcher
}

func TestBindSkipsIncompatibleAndExcludedMethods(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	want := []string{"Echo", "Fails", "Plain", "Sum", "Version"}
	if got := dispatcher.Methods(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Methods() = %v, want %v", got, want)
	}
}

func TestBindRejectsNilTarget(t *testing.T) {
	if err := NewDispatcher().Bind((*sampleTarget)(nil)); err == nil {
		t.Fatal("expected error for nil target")
	}
}

func TestCallResultShapes(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	cases := []struct {
		name   string
		method string
		params string
		want   string
		code   int
	}{
		{name: "no result", method: "Plain", params: "[]", want: "null"},
		{name: "null params", method: "Plain", params: "", want: "null"},
		{name: "value", method: "Version", params: "[]", want: `"1.2.3"`},
		{name: "positional", method: "Sum", params: "[2,3]", want: "5"},
		{name: "struct", method: "Echo", params: `[{"name":"a","count":2}]`, want: `{"name":"a","count":2}`},
		{name: "application error", method: "Fails", params: "[]", code: CodeApplication},
		{name: "unknown", method: "Missing", params: "[]", code: CodeMethodNotFound},
		{name: "arity", method: "Sum", params: "[1]", code: CodeInvalidParams},
		{name: "type", method: "Sum", params: `["a",1]`, code: CodeInvalidParams},
		{name: "not array", method: "Sum", params: `{"a":1}`, code: CodeInvalidParams},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, rpcErr := dispatcher.Call(testCase.method, json.RawMessage(testCase.params))
			if testCase.code != 0 {
				if rpcErr == nil || rpcErr.Code != testCase.code {
					t.Fatalf("Call error = %v, want code %d", rpcErr, testCase.code)
				}
				return
			}
			if rpcErr != nil {
				t.Fatalf("Call: %v", rpcErr)
			}
			if string(result) != testCase.want {
				t.Fatalf("Call result = %s, want %s", result, testCase.want)
			}
		})
	}
}

func TestApplicationErrorCarriesMessage(t *testing.T) {
	dispatcher := newTestDispatcher(t)
	_, rpcErr := dispatcher.Call("Fails", json.RawMessage("[]"))
	if rpcErr == nil || rpcErr.Message != "boom" {
		t.Fatalf("Call error = %v, want boom", rpcErr)
	}
}
