package app

import (
	"errors"
	"path/filepath"
	"testing"
)

type fakeHost struct {
	events   []string
	opened   []string
	shown    int
	filePath string
	dirPath  string
	savePath string
	err      error
}

func (h *fakeHost) Emit(name string, _ any) { h.events = append(h.events, name) }
func (h *fakeHost) OpenURL(target string) error {
	h.opened = append(h.opened, target)
	return h.err
}
func (h *fakeHost) ShowWindow() { h.shown++ }
func (h *fakeHost) Quit()       {}
func (h *fakeHost) OpenFileDialog(string) (string, error) {
	return h.filePath, h.err
}
func (h *fakeHost) OpenDirectoryDialog(string) (string, error) {
	return h.dirPath, h.err
}
func (h *fakeHost) SaveFileDialog(string, string) (string, error) {
	return h.savePath, h.err
}

func TestHostCallsAreNoOpsWithoutShell(t *testing.T) {
	application := &App{}
	application.emit("session:state", nil)
	application.showWindow()
	if err := application.openURL("https://example.com"); !errors.Is(err, errApplicationNotReady) {
		t.Fatalf("openURL without host = %v, want not-ready error", err)
	}
	if _, err := application.PickServerUploadPath("file"); !errors.Is(err, errApplicationNotReady) {
		t.Fatalf("PickServerUploadPath without host = %v, want not-ready error", err)
	}
}

func TestHostReceivesShellCalls(t *testing.T) {
	host := &fakeHost{filePath: "/tmp/upload.txt", dirPath: "/tmp/parent", savePath: "/tmp/out.bin"}
	application := &App{}
	application.SetHost(host)

	application.emit("update:state", struct{}{})
	application.showWindow()
	if err := application.openURL("https://example.com/login"); err != nil {
		t.Fatal(err)
	}
	if got, err := application.PickServerUploadPath("file"); err != nil || got != "/tmp/upload.txt" {
		t.Fatalf("PickServerUploadPath(file) = %q, %v", got, err)
	}
	if got, err := application.PickServerUploadPath("directory"); err != nil || got != "/tmp/parent" {
		t.Fatalf("PickServerUploadPath(directory) = %q, %v", got, err)
	}
	if got, err := application.PickServerDownloadPath("file", "out.bin"); err != nil || got != "/tmp/out.bin" {
		t.Fatalf("PickServerDownloadPath(file) = %q, %v", got, err)
	}
	got, err := application.PickServerDownloadPath("directory", "bundle")
	if err != nil || got != filepath.Join("/tmp/parent", "bundle") {
		t.Fatalf("PickServerDownloadPath(directory) = %q, %v", got, err)
	}
	if len(host.events) != 1 || host.events[0] != "update:state" {
		t.Fatalf("events = %v", host.events)
	}
	if host.shown != 1 || len(host.opened) != 1 {
		t.Fatalf("host calls: shown=%d opened=%v", host.shown, host.opened)
	}
}
