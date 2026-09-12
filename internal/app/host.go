package app

import (
	"errors"
	"sync"
)

var errApplicationNotReady = errors.New("application is not ready")

// Host is the desktop shell that owns the window, tray, native dialogs and the
// system browser. The Go backend runs as a sidecar process and never touches
// native UI itself; every call here crosses the IPC boundary to the shell.
type Host interface {
	// Emit publishes a named event with a JSON-serializable payload to the
	// renderer. It must never block the caller.
	Emit(name string, payload any)
	// OpenURL opens a target in the user's default browser.
	OpenURL(target string) error
	// ShowWindow restores and focuses the main window.
	ShowWindow()
	// Quit asks the shell to exit the whole application.
	Quit()
	// OpenFileDialog returns the selected file path or "" when cancelled.
	OpenFileDialog(title string) (string, error)
	// OpenDirectoryDialog returns the selected directory or "" when cancelled.
	OpenDirectoryDialog(title string) (string, error)
	// SaveFileDialog returns the chosen destination or "" when cancelled.
	SaveFileDialog(title, defaultName string) (string, error)
}

// hostRef is a swappable Host so the application can be constructed before the
// IPC transport is connected and so tests can capture emitted events.
type hostRef struct {
	mu   sync.RWMutex
	host Host
}

func (ref *hostRef) set(host Host) {
	ref.mu.Lock()
	defer ref.mu.Unlock()
	ref.host = host
}

func (ref *hostRef) get() Host {
	ref.mu.RLock()
	defer ref.mu.RUnlock()
	return ref.host
}

// SetHost attaches the desktop shell. Events emitted before a Host is attached
// are dropped, matching the previous behaviour when the window was not ready.
func (a *App) SetHost(host Host) {
	a.host.set(host)
}

func (a *App) emit(name string, payload any) {
	if host := a.host.get(); host != nil {
		host.Emit(name, payload)
	}
}

func (a *App) openURL(target string) error {
	host := a.host.get()
	if host == nil {
		return errApplicationNotReady
	}
	return host.OpenURL(target)
}

func (a *App) showWindow() {
	if host := a.host.get(); host != nil {
		host.ShowWindow()
	}
}

func (a *App) hostOrError() (Host, error) {
	host := a.host.get()
	if host == nil {
		return nil, errApplicationNotReady
	}
	return host, nil
}
