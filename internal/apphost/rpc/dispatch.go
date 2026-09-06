// Package rpc exposes the desktop application to an out-of-process shell.
//
// The Electron main process spawns the sidecar this package serves, then the
// renderer reaches every application method through a loopback JSON-RPC
// endpoint. Methods are dispatched by name off *app.App, so the frontend calls
// a binding exactly as it would a local method.
package rpc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// dispatcher resolves method names to the bound receiver's exported methods.
type dispatcher struct {
	receiver reflect.Value
	methods  map[string]reflect.Method
}

var errorInterface = reflect.TypeFor[error]()

func newDispatcher(receiver any) *dispatcher {
	value := reflect.ValueOf(receiver)
	receiverType := value.Type()
	methods := make(map[string]reflect.Method, receiverType.NumMethod())
	for method := range receiverType.Methods() {
		methods[method.Name] = method
	}
	return &dispatcher{receiver: value, methods: methods}
}

// names lists every dispatchable method, sorted by the map's own iteration is
// not stable, so callers that need order must sort the result themselves.
func (d *dispatcher) names() []string {
	names := make([]string, 0, len(d.methods))
	for name := range d.methods {
		names = append(names, name)
	}
	return names
}

// call invokes method with JSON-encoded arguments and returns the JSON-encodable
// result. A method's trailing error return becomes the returned error rather
// than part of the result, matching how the frontend's promises reject today.
func (d *dispatcher) call(name string, args []json.RawMessage) (any, error) {
	method, ok := d.methods[name]
	if !ok {
		return nil, fmt.Errorf("unknown method %q", name)
	}
	methodType := method.Type
	// Index 0 is the receiver.
	wanted := methodType.NumIn() - 1
	if methodType.IsVariadic() {
		return nil, fmt.Errorf("method %q is variadic and cannot be dispatched", name)
	}
	if len(args) != wanted {
		return nil, fmt.Errorf("method %q expects %d argument(s), got %d", name, wanted, len(args))
	}

	in := make([]reflect.Value, 0, wanted+1)
	in = append(in, d.receiver)
	for index, encoded := range args {
		argument := reflect.New(methodType.In(index + 1))
		if err := json.Unmarshal(encoded, argument.Interface()); err != nil {
			return nil, fmt.Errorf("decode argument %d of %q: %w", index, name, err)
		}
		in = append(in, argument.Elem())
	}

	out := method.Func.Call(in)
	return splitResults(out, methodType)
}

func splitResults(out []reflect.Value, methodType reflect.Type) (any, error) {
	var err error
	count := len(out)
	if count > 0 && methodType.Out(count-1).Implements(errorInterface) {
		if failure, ok := reflect.TypeAssert[error](out[count-1]); ok && failure != nil {
			err = failure
		}
		count--
	}
	switch count {
	case 0:
		return nil, err
	case 1:
		return out[0].Interface(), err
	default:
		results := make([]any, 0, count)
		for _, value := range out[:count] {
			results = append(results, value.Interface())
		}
		return results, err
	}
}

// pending tracks in-flight calls the sidecar made into the shell.
type pending struct {
	mu      sync.Mutex
	next    uint64
	waiting map[uint64]chan shellResult
}

type shellResult struct {
	value json.RawMessage
	err   error
}

func newPending() *pending {
	return &pending{waiting: make(map[uint64]chan shellResult)}
}

func (p *pending) begin() (uint64, chan shellResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.next++
	id := p.next
	result := make(chan shellResult, 1)
	p.waiting[id] = result
	return id, result
}

func (p *pending) complete(id uint64, result shellResult) {
	p.mu.Lock()
	waiter, ok := p.waiting[id]
	delete(p.waiting, id)
	p.mu.Unlock()
	if ok {
		waiter <- result
	}
}

func (p *pending) cancel(id uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.waiting, id)
}

// failAll releases every waiter, used when the shell connection drops.
func (p *pending) failAll(err error) {
	p.mu.Lock()
	waiting := p.waiting
	p.waiting = make(map[uint64]chan shellResult)
	p.mu.Unlock()
	for _, waiter := range waiting {
		waiter <- shellResult{err: err}
	}
}
