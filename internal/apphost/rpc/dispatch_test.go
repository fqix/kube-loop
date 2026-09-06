package rpc

import (
	"encoding/json"
	"errors"
	"testing"
)

type sample struct {
	lastName string
	lastAge  int
}

type profile struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func (s *sample) Greet(name string) string { s.lastName = name; return "hello " + name }

func (s *sample) Save(person profile) (profile, error) {
	if person.Name == "" {
		return profile{}, errors.New("name is required")
	}
	s.lastAge = person.Age
	return person, nil
}

func (s *sample) Ping() {}

func (s *sample) Pair() (string, int) { return "pair", 7 }

func (s *sample) Fail() error { return errors.New("always fails") }

func raw(t *testing.T, values ...any) []json.RawMessage {
	t.Helper()
	args := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %v: %v", value, err)
		}
		args = append(args, encoded)
	}
	return args
}

func TestDispatchDecodesArgumentsAndResults(t *testing.T) {
	receiver := &sample{}
	dispatch := newDispatcher(receiver)

	result, err := dispatch.call("Greet", raw(t, "world"))
	if err != nil || result != "hello world" {
		t.Fatalf("Greet = %v, %v", result, err)
	}
	if receiver.lastName != "world" {
		t.Fatalf("receiver.lastName = %q, want world", receiver.lastName)
	}

	result, err = dispatch.call("Save", raw(t, profile{Name: "ada", Age: 36}))
	if err != nil {
		t.Fatalf("Save error = %v", err)
	}
	if saved, ok := result.(profile); !ok || saved.Age != 36 {
		t.Fatalf("Save result = %#v, want the saved profile", result)
	}

	if result, err = dispatch.call("Ping", nil); err != nil || result != nil {
		t.Fatalf("Ping = %v, %v, want nil, nil", result, err)
	}

	result, err = dispatch.call("Pair", nil)
	if err != nil {
		t.Fatalf("Pair error = %v", err)
	}
	pair, ok := result.([]any)
	if !ok || len(pair) != 2 || pair[0] != "pair" || pair[1] != 7 {
		t.Fatalf("Pair result = %#v, want [pair 7]", result)
	}
}

func TestDispatchSurfacesTrailingError(t *testing.T) {
	dispatch := newDispatcher(&sample{})

	if _, err := dispatch.call("Fail", nil); err == nil || err.Error() != "always fails" {
		t.Fatalf("Fail error = %v, want always fails", err)
	}
	if _, err := dispatch.call("Save", raw(t, profile{})); err == nil {
		t.Fatal("Save with an empty name succeeded, want an error")
	}
}

func TestDispatchRejectsBadCalls(t *testing.T) {
	dispatch := newDispatcher(&sample{})

	if _, err := dispatch.call("Missing", nil); err == nil {
		t.Fatal("unknown method dispatched")
	}
	if _, err := dispatch.call("Greet", nil); err == nil {
		t.Fatal("Greet accepted zero arguments, want an arity error")
	}
	if _, err := dispatch.call("Greet", []json.RawMessage{json.RawMessage(`{}`)}); err == nil {
		t.Fatal("Greet accepted an object for a string argument")
	}
}

func TestDispatchListsMethods(t *testing.T) {
	names := newDispatcher(&sample{}).names()
	wanted := map[string]bool{"Greet": true, "Save": true, "Ping": true, "Pair": true, "Fail": true}
	for _, name := range names {
		delete(wanted, name)
	}
	if len(wanted) != 0 {
		t.Fatalf("methods missing from dispatch table: %v", wanted)
	}
}
