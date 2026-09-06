package app

import (
	"errors"
	"sync"
	"testing"
)

// fakeHost records everything the application asks of the desktop shell.
type fakeHost struct {
	mu        sync.Mutex
	events    []hostEvent
	shown     int
	quit      int
	openedURL []string

	fileDialog      func(title string) (string, error)
	directoryDialog func(title string) (string, error)
	saveDialog      func(title, defaultFilename string) (string, error)
}

type hostEvent struct {
	name    string
	payload any
}

func (h *fakeHost) Emit(event string, payload any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, hostEvent{name: event, payload: payload})
}

func (h *fakeHost) ShowWindow() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.shown++
}

func (h *fakeHost) Quit() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.quit++
}

func (h *fakeHost) OpenURL(target string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.openedURL = append(h.openedURL, target)
	return nil
}

func (h *fakeHost) OpenFileDialog(title string) (string, error) {
	if h.fileDialog == nil {
		return "", errors.New("file dialog not configured")
	}
	return h.fileDialog(title)
}

func (h *fakeHost) OpenDirectoryDialog(title string) (string, error) {
	if h.directoryDialog == nil {
		return "", errors.New("directory dialog not configured")
	}
	return h.directoryDialog(title)
}

func (h *fakeHost) SaveFileDialog(title, defaultFilename string) (string, error) {
	if h.saveDialog == nil {
		return "", errors.New("save dialog not configured")
	}
	return h.saveDialog(title, defaultFilename)
}

func (h *fakeHost) emitted() []hostEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]hostEvent(nil), h.events...)
}

func (h *fakeHost) eventNames() []string {
	names := make([]string, 0)
	for _, event := range h.emitted() {
		names = append(names, event.name)
	}
	return names
}

func TestApplicationWithoutHostIsUsable(t *testing.T) {
	application := &App{}

	application.emit("update:state", nil)
	application.hostRuntime().ShowWindow()
	application.hostRuntime().Quit()
	if err := application.hostRuntime().OpenURL(releaseURL); !errors.Is(err, errNoHost) {
		t.Fatalf("OpenURL error = %v, want errNoHost", err)
	}

	if _, err := application.hostRuntime().OpenFileDialog("Select"); !errors.Is(err, errNoHost) {
		t.Fatalf("OpenFileDialog error = %v, want errNoHost", err)
	}
	if _, err := application.hostRuntime().OpenDirectoryDialog("Select"); !errors.Is(err, errNoHost) {
		t.Fatalf("OpenDirectoryDialog error = %v, want errNoHost", err)
	}
	if _, err := application.hostRuntime().SaveFileDialog("Save", "file"); !errors.Is(err, errNoHost) {
		t.Fatalf("SaveFileDialog error = %v, want errNoHost", err)
	}
}

func TestSetHostRoutesShellCalls(t *testing.T) {
	host := &fakeHost{}
	application := &App{}
	SetHost(application, host)

	application.emit("update:state", "payload")
	application.hostRuntime().ShowWindow()
	application.hostRuntime().Quit()
	if err := application.hostRuntime().OpenURL(releaseURL); err != nil {
		t.Fatalf("OpenURL error = %v", err)
	}

	events := host.emitted()
	if len(events) != 1 || events[0].name != "update:state" || events[0].payload != "payload" {
		t.Fatalf("emitted events = %#v, want one update:state event", events)
	}
	if host.shown != 1 || host.quit != 1 {
		t.Fatalf("shown = %d, quit = %d, want 1 and 1", host.shown, host.quit)
	}
	if len(host.openedURL) != 1 || host.openedURL[0] != releaseURL {
		t.Fatalf("openedURL = %v, want [%s]", host.openedURL, releaseURL)
	}

	SetHost(nil, host) // must not panic
}
