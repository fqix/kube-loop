# ADR 0025: Electron shell with a Go sidecar backend

- Status: Accepted
- Date: 2026-09-11

## Context

The desktop application shipped as a single Wails v2 binary: the Go backend in
`internal/app` was linked into the same process as the platform WebView, Go
methods were bound directly onto `window.go.app.App`, and the Wails runtime
provided events, native dialogs, the system browser, window control and the
custom `kubeloop://` protocol. The system tray came from a separate Go library
that had to share the UI thread with Wails.

That coupling has costs the project no longer wants to pay: the renderer
depends on whichever WebKit/WebView2 the host offers, packaging is bound to the
Wails CLI and its NSIS/plist templates, native UI code and Go business logic run
in one address space, and every UI-thread constraint (tray, single instance,
protocol activation) has to be reconciled inside Go.

## Decision

The desktop application is split into two processes.

### Electron owns native UI

The repository root is the Electron package in the electron-vite layout:
`src/main` (main process), `src/preload` and `src/renderer` (the React
application) are bundled by `electron-vite` into `out/`, which
`electron-builder.yml` packages.
The main process owns the window, the tray, single-instance locking, the
`kubeloop://` protocol registration, native file dialogs and opening the system
browser. The renderer is the existing React application, loaded from the Vite
build (or the Vite dev server) with `contextIsolation` and `sandbox` enabled;
the preload exposes exactly one bridge, `window.kubeloop`, with `call`, `on`,
window controls and theme sync. A strict Content-Security-Policy is applied to
packaged (`file://`) loads.

### Go runs as a sidecar

The root `main` package still builds the Go backend, now named
`kubeloop-backend`. Electron spawns it as a child process and speaks
newline-delimited JSON-RPC 2.0 over its stdin/stdout. No socket or port is
opened, so nothing else on the machine can reach the backend, and the shell's
death closes the pipe and triggers the same cleanup as an explicit shutdown.

`internal/desktopipc` implements the protocol:

- `Dispatcher` binds the exported methods of `*app.App` by reflection with the
  same conventions the previous binding layer used: positional JSON arguments
  and `()`, `(error)`, `(T)` or `(T, error)` results. Methods whose parameters
  are not JSON-decodable (for example `SetHost(app.Host)`) are never bound.
- `Server` runs requests concurrently, pushes `host.event` notifications, and
  issues `host.*` requests back to the shell for dialogs and the browser.
- `app.Host` is the only abstraction `internal/app` has for the shell; the
  Wails runtime import is gone. Events, `OpenURL`, `ShowWindow`, `Quit` and
  the three dialog calls are all that cross the boundary.

### Packaging

`go run ./build/desktop-backend.go` builds the privileged helper (embedded into
the backend), the patched sing-box and the backend itself into `build/bin`.
`electron-builder.yml` bundles them as extra resources next to the Electron
binary, so the existing lookup rules in `internal/helper` (`<exe dir>/sing-box`,
`resources\kubeloop-helper.exe` on Windows) keep working without change.
DMG/tar.gz, NSIS/zip and deb/rpm/tar.gz replace the Wails, `package-desktop.sh`
and nfpm outputs.

## Consequences

- The renderer no longer depends on the host WebView; Chromium ships with the
  app and the UI is identical across platforms.
- `internal/app` is testable without any UI framework: tests inject a fake
  `Host` to capture events and dialog calls.
- The backend can be restarted or debugged independently of the shell
  (`KUBELOOP_BACKEND_PATH` points Electron at any build; a Go debugger attaches
  to `kubeloop-backend`).
- Installers grow by the Electron runtime, and the app now runs three processes
  (Electron main, renderer, Go backend) instead of one.
- The `kubeloop://` OAuth callback is received by Electron and forwarded to the
  backend through `HandleAuthCallbackURL`; in development the script patches
  the Electron bundle's `Info.plist` on macOS so the scheme is delivered.
