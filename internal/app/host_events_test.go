package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	clientremote "github.com/fengqi-dev/kube-loop/internal/client/remote"
	"github.com/fengqi-dev/kube-loop/internal/update"
)

// The frontend subscribes to these exact event names, so a rename here is a
// breaking change to the desktop shell contract.
func TestCheckForUpdatesEmitsUpdateState(t *testing.T) {
	releases := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://example.test/release"}`))
	}))
	t.Cleanup(releases.Close)

	host := &fakeHost{}
	application := &App{
		updater: &update.Checker{
			CurrentVersion: "1.0.0",
			HTTPClient:     releases.Client(),
			LatestURL:      releases.URL,
		},
	}
	SetHost(application, host)

	state := application.CheckForUpdates()

	events := host.emitted()
	if len(events) != 1 || events[0].name != updateStateEvent {
		t.Fatalf("emitted = %v, want a single %q event", host.eventNames(), updateStateEvent)
	}
	emitted, ok := events[0].payload.(update.Info)
	if !ok {
		t.Fatalf("payload type = %T, want update.Info", events[0].payload)
	}
	if emitted != state {
		t.Fatalf("emitted payload = %#v, want the returned state %#v", emitted, state)
	}
	if emitted.LatestVersion != "v9.9.9" {
		t.Fatalf("LatestVersion = %q, want v9.9.9", emitted.LatestVersion)
	}
}

func TestOpenUpdatePageUsesHostBrowser(t *testing.T) {
	host := &fakeHost{}
	application := &App{}
	SetHost(application, host)
	application.updateState = update.Info{URL: "https://example.test/release"}

	if err := application.OpenUpdatePage(); err != nil {
		t.Fatalf("OpenUpdatePage() error = %v", err)
	}
	if len(host.openedURL) != 1 || host.openedURL[0] != "https://example.test/release" {
		t.Fatalf("openedURL = %v, want the recorded release URL", host.openedURL)
	}

	// An empty state falls back to the project releases page.
	application.updateState = update.Info{}
	if err := application.OpenUpdatePage(); err != nil {
		t.Fatalf("OpenUpdatePage() error = %v", err)
	}
	if host.openedURL[1] != releaseURL {
		t.Fatalf("fallback URL = %q, want %q", host.openedURL[1], releaseURL)
	}
}

func TestOpenUpdatePageReportsMissingShell(t *testing.T) {
	application := &App{}

	if err := application.OpenUpdatePage(); err == nil {
		t.Fatal("OpenUpdatePage() succeeded without a desktop shell")
	}
}

func TestServerInventoryEventEmitsSnapshot(t *testing.T) {
	host := &fakeHost{}
	application := &App{}
	SetHost(application, host)

	event := ServerInventoryEvent{
		ProfileID: "profile-1",
		Namespace: "default",
		Resource:  clientremote.InventoryResource("pods"),
		Error:     "watch closed",
	}
	application.emitServerInventoryEvent(event)

	events := host.emitted()
	if len(events) != 1 || events[0].name != serverInventorySnapshotEvent {
		t.Fatalf("emitted = %v, want a single %q event", host.eventNames(), serverInventorySnapshotEvent)
	}
	if events[0].payload != any(event) {
		t.Fatalf("payload = %#v, want %#v", events[0].payload, event)
	}
}

func TestPickServerPathsUseHostDialogs(t *testing.T) {
	host := &fakeHost{
		fileDialog:      func(title string) (string, error) { return "file:" + title, nil },
		directoryDialog: func(_ string) (string, error) { return "/parent", nil },
		saveDialog:      func(_, name string) (string, error) { return "save:" + name, nil },
	}
	application := &App{}
	SetHost(application, host)

	got, err := application.PickServerUploadPath("file")
	if err != nil || got != "file:Select file to upload" {
		t.Fatalf("PickServerUploadPath(file) = %q, %v", got, err)
	}
	got, err = application.PickServerUploadPath("directory")
	if err != nil || got != "/parent" {
		t.Fatalf("PickServerUploadPath(directory) = %q, %v", got, err)
	}
	if _, err = application.PickServerUploadPath("other"); err == nil {
		t.Fatal("PickServerUploadPath(other) succeeded, want a kind error")
	}

	got, err = application.PickServerDownloadPath("file", "report.txt")
	if err != nil || got != "save:report.txt" {
		t.Fatalf("PickServerDownloadPath(file) = %q, %v", got, err)
	}
	got, err = application.PickServerDownloadPath("directory", "bundle")
	if err != nil || got != "/parent/bundle" {
		t.Fatalf("PickServerDownloadPath(directory) = %q, %v", got, err)
	}
}
