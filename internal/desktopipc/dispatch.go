package desktopipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

var errorType = reflect.TypeFor[error]()

// boundMethod is one exported method exposed to the shell.
type boundMethod struct {
	receiver reflect.Value
	method   reflect.Method
	params   []reflect.Type
	// returnsValue and returnsError describe the result shape:
	//   ()            → null
	//   (error)       → null or error
	//   (T)           → T
	//   (T, error)    → T or error
	returnsValue bool
	returnsError bool
}

// Dispatcher routes named requests to exported methods on bound values, using
// the same conventions the previous in-process binding layer relied on so the
// renderer keeps calling the same method names with positional arguments.
type Dispatcher struct {
	methods map[string]*boundMethod
}

// NewDispatcher returns an empty dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{methods: make(map[string]*boundMethod)}
}

// Bind exposes every exported method of target whose parameters and results
// can cross a JSON boundary. Methods named in exclude are skipped even when
// their signature is compatible.
func (d *Dispatcher) Bind(target any, exclude ...string) error {
	value := reflect.ValueOf(target)
	if !value.IsValid() || (value.Kind() == reflect.Pointer && value.IsNil()) {
		return errors.New("bind target must be a non-nil value")
	}
	valueType := value.Type()
	for method := range valueType.Methods() {
		if !method.IsExported() || slices.Contains(exclude, method.Name) {
			continue
		}
		bound, ok := describeMethod(value, method)
		if !ok {
			continue
		}
		if _, exists := d.methods[method.Name]; exists {
			return fmt.Errorf("method %q is already bound", method.Name)
		}
		d.methods[method.Name] = bound
	}
	return nil
}

// Methods lists bound method names in a stable order.
func (d *Dispatcher) Methods() []string {
	names := make([]string, 0, len(d.methods))
	for name := range d.methods {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func describeMethod(receiver reflect.Value, method reflect.Method) (*boundMethod, bool) {
	methodType := method.Type
	// Index 0 is the receiver.
	params := make([]reflect.Type, 0, methodType.NumIn()-1)
	for index := 1; index < methodType.NumIn(); index++ {
		param := methodType.In(index)
		if !jsonDecodable(param) {
			return nil, false
		}
		params = append(params, param)
	}
	bound := &boundMethod{receiver: receiver, method: method, params: params}
	switch methodType.NumOut() {
	case 0:
	case 1:
		if methodType.Out(0) == errorType {
			bound.returnsError = true
		} else {
			bound.returnsValue = true
		}
	case 2:
		if methodType.Out(1) != errorType {
			return nil, false
		}
		bound.returnsValue = true
		bound.returnsError = true
	default:
		return nil, false
	}
	return bound, true
}

func jsonDecodable(t reflect.Type) bool {
	//nolint:exhaustive // Every remaining kind is a JSON-compatible scalar or struct.
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Complex64, reflect.Complex128:
		return false
	case reflect.Interface:
		// Only the empty interface can be decoded; a named interface such as
		// the host abstraction must never be reachable from the renderer.
		return t.NumMethod() == 0
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return jsonDecodable(t.Elem())
	case reflect.Map:
		return jsonDecodable(t.Key()) && jsonDecodable(t.Elem())
	default:
		return true
	}
}

// Call invokes a bound method with JSON positional parameters and returns the
// JSON-encoded result. Errors from the method itself are reported with
// CodeApplication so the shell can distinguish them from protocol faults.
func (d *Dispatcher) Call(name string, params json.RawMessage) (json.RawMessage, *Error) {
	bound, ok := d.methods[name]
	if !ok {
		return nil, newError(CodeMethodNotFound, "method %q is not bound", name)
	}
	args, rpcErr := decodeArguments(bound.params, params)
	if rpcErr != nil {
		return nil, rpcErr
	}
	results := bound.method.Func.Call(append([]reflect.Value{bound.receiver}, args...))
	if bound.returnsError {
		if errValue := results[len(results)-1]; !errValue.IsNil() {
			callErr, _ := reflect.TypeAssert[error](errValue)
			return nil, &Error{Code: CodeApplication, Message: callErr.Error()}
		}
	}
	if !bound.returnsValue {
		return json.RawMessage("null"), nil
	}
	encoded, err := json.Marshal(results[0].Interface())
	if err != nil {
		return nil, newError(CodeInternal, "encode %s result: %v", name, err)
	}
	return encoded, nil
}

func decodeArguments(params []reflect.Type, raw json.RawMessage) ([]reflect.Value, *Error) {
	var positional []json.RawMessage
	if len(strings.TrimSpace(string(raw))) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &positional); err != nil {
			return nil, newError(CodeInvalidParams, "params must be a JSON array: %v", err)
		}
	}
	if len(positional) != len(params) {
		return nil, newError(CodeInvalidParams, "expected %d argument(s), got %d", len(params), len(positional))
	}
	args := make([]reflect.Value, 0, len(params))
	for index, paramType := range params {
		target := reflect.New(paramType)
		if err := json.Unmarshal(positional[index], target.Interface()); err != nil {
			return nil, newError(CodeInvalidParams, "argument %d: %v", index, err)
		}
		args = append(args, target.Elem())
	}
	return args, nil
}
