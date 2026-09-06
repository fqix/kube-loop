package app

import (
	"context"
	"errors"
)

// errNoHost reports a shell capability requested before a shell was attached.
var errNoHost = errors.New("desktop shell is unavailable")

// Host is the desktop shell the application runs inside. It covers every
// capability the application needs from the shell: pushing events to the user
// interface, driving the window, and opening native dialogs.
//
// Keeping them behind an interface lets the application logic stay independent
// of the shell, which runs in a separate process.
type Host interface {
	Emit(event string, payload any)
	ShowWindow()
	Quit()
	OpenURL(target string) error
	OpenFileDialog(title string) (string, error)
	OpenDirectoryDialog(title string) (string, error)
	SaveFileDialog(title, defaultFilename string) (string, error)
}

// SetHost installs the shell an application reports to. It is called by the
// desktop entry point before startup.
func SetHost(a *App, host Host) {
	if a == nil {
		return
	}
	a.host = host
}

// host resolves the installed shell, falling back to a no-op so an application
// built without a shell — unit tests, and the bindings generator — stays usable.
func (a *App) hostRuntime() Host {
	if a.host != nil {
		return a.host
	}
	return noopHost{}
}

// emit publishes an event to the user interface when a shell is attached.
func (a *App) emit(event string, payload any) {
	a.hostRuntime().Emit(event, payload)
}

type noopHost struct{}

func (noopHost) Emit(string, any)     {}
func (noopHost) ShowWindow()          {}
func (noopHost) Quit()                {}
func (noopHost) OpenURL(string) error { return errNoHost }

func (noopHost) OpenFileDialog(string) (string, error)      { return "", errNoHost }
func (noopHost) OpenDirectoryDialog(string) (string, error) { return "", errNoHost }
func (noopHost) SaveFileDialog(string, string) (string, error) {
	return "", errNoHost
}

// RuntimeContext exposes the shell context an application was started with, so
// a shell adapter outside this package can bind to it. It is nil until startup.
func RuntimeContext(a *App) context.Context {
	if a == nil {
		return nil
	}
	return a.ctx
}
