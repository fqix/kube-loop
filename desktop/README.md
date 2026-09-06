# KubeLoop desktop application

An Electron shell around a Go sidecar.

- `src/main` — the Electron main process: the window, tray, `kubeloop://` deep
  links, and the bridge to the backend. `preload.ts` republishes the backend on
  `window.go.app.App` and `window.runtime` for the renderer.
- `src/renderer` — the React interface.
- `forge.config.ts` — packaging: the installers, the bundled resources, and the
  platform signing steps.

The shell spawns [`cmd/kubeloop-desktop-host`](../cmd/kubeloop-desktop-host),
which serves every application binding over a loopback JSON-RPC endpoint and
pushes backend events over a WebSocket.

Run everything from the repository root:

```bash
npm ci
npm run build:host      # build the Go sidecar into build/bin
npm run dev             # run the app
npm run package:desktop # build installers for this platform
```

`build:host` writes the sidecar next to the `sing-box` core that
`build/stage-package-assets.go` stages, so run `make desktop-assets` once if
`build/bin` is empty.

## Checking the bridge

The smoke test drives a running app over the DevTools protocol and exercises
the whole stack — renderer globals, the preload bridge, the main process, and
the Go sidecar:

```bash
npm run start:debug --workspace=kubeloop-desktop   # in one shell
npm run test:shell --workspace=kubeloop-desktop    # in another
```
