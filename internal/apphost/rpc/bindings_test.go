package rpc

import (
	"reflect"
	"slices"
	"testing"

	desktopapp "github.com/fengqi-dev/kube-loop/internal/app"
)

// jsonSafe reports whether a value of this type survives a JSON round trip
// between the sidecar and the shell. The bridge encodes every argument and
// result as JSON, so this test keeps a new binding from silently becoming
// uncallable from the frontend.
func jsonSafe(candidate reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[candidate] {
		return true
	}
	seen[candidate] = true

	switch candidate.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer,
		reflect.Complex64, reflect.Complex128:
		return false
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return jsonSafe(candidate.Elem(), seen)
	case reflect.Map:
		return jsonSafe(candidate.Key(), seen) && jsonSafe(candidate.Elem(), seen)
	case reflect.Interface:
		// An interface carries no static shape; only the empty interface is
		// safe, and only because the concrete value is encoded as-is.
		return candidate.NumMethod() == 0
	case reflect.Struct:
		for field := range candidate.Fields() {
			if field.PkgPath != "" && !field.Anonymous {
				continue // Unexported fields are skipped by encoding/json.
			}
			if field.Tag.Get("json") == "-" {
				continue
			}
			if !jsonSafe(field.Type, seen) {
				return false
			}
		}
		return true
	case reflect.Invalid, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.String:
		return true
	default:
		return true
	}
}

func TestEveryApplicationBindingIsDispatchable(t *testing.T) {
	dispatch := newDispatcher(&desktopapp.App{})
	names := dispatch.names()
	if len(names) == 0 {
		t.Fatal("the application exposes no methods")
	}
	slices.Sort(names)

	for _, name := range names {
		method := dispatch.methods[name]
		signature := method.Type
		if signature.IsVariadic() {
			t.Errorf("%s is variadic and cannot be dispatched over the shell bridge", name)
			continue
		}
		for index := 1; index < signature.NumIn(); index++ {
			if !jsonSafe(signature.In(index), map[reflect.Type]bool{}) {
				t.Errorf("%s: argument %d of type %s is not JSON-encodable", name, index-1, signature.In(index))
			}
		}
		for index := range signature.NumOut() {
			out := signature.Out(index)
			if out.Implements(errorInterface) {
				continue
			}
			if !jsonSafe(out, map[reflect.Type]bool{}) {
				t.Errorf("%s: result %d of type %s is not JSON-encodable", name, index, out)
			}
		}
	}
}

// The frontend calls these by name, so losing one breaks the desktop app.
func TestCoreBindingsArePresent(t *testing.T) {
	dispatch := newDispatcher(&desktopapp.App{})
	for _, name := range []string{
		"Bootstrap",
		"ServerProfiles",
		"LoadServerInventory",
		"ConnectServerDataPlane",
		"DisconnectServerDataPlane",
		"StartServerPortForward",
		"PickServerUploadPath",
		"PickServerDownloadPath",
		"CheckForUpdates",
		"OpenUpdatePage",
		"HandleAuthCallbackURL",
		"GetMCPStatus",
	} {
		if _, ok := dispatch.methods[name]; !ok {
			t.Errorf("binding %q is missing from the shell bridge", name)
		}
	}
}
