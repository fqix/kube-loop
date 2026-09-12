import {
  app,
  BrowserWindow,
  dialog,
  ipcMain,
  Menu,
  nativeImage,
  nativeTheme,
  session,
  shell,
  Tray,
  type IpcMainInvokeEvent,
} from "electron";
import path from "node:path";
import { Backend, BackendError } from "./backend";
import {
  backendBinaryPath,
  backendWorkingDirectory,
  devServerURL,
  preloadPath,
  PROTOCOL_SCHEME,
  rendererIndexPath,
  trayIconPath,
} from "./paths";

const APP_ID = "dev.fengqi.kube-loop";
const BACKGROUND = "#0f172a";

let mainWindow: BrowserWindow | null = null;
let tray: Tray | null = null;
let backend: Backend | null = null;
let quitting = false;
let backendStopped = false;
// Callback URLs that arrive before the backend is ready are replayed later.
const pendingCallbacks: string[] = [];

app.setAppUserModelId(APP_ID);

// ---------------------------------------------------------------------------
// Single instance and kubeloop:// callbacks

function callbackFromArgs(args: string[]): string | undefined {
  return args.find((argument) => argument.toLowerCase().startsWith(`${PROTOCOL_SCHEME}://`));
}

function deliverAuthCallback(rawURL: string) {
  if (!backend || !backend.hasMethod("HandleAuthCallbackURL")) {
    pendingCallbacks.push(rawURL);
    return;
  }
  backend.call("HandleAuthCallbackURL", [rawURL]).catch((error: unknown) => {
    console.error(`OAuth callback rejected: ${error instanceof Error ? error.message : String(error)}`);
  });
  showWindow();
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", (_event, argv) => {
    const callback = callbackFromArgs(argv);
    if (callback) deliverAuthCallback(callback);
    showWindow();
  });
  app.on("open-url", (event, url) => {
    event.preventDefault();
    deliverAuthCallback(url);
  });
  registerProtocolClient();
}

function registerProtocolClient() {
  if (process.defaultApp && process.argv.length >= 2) {
    // Running from source: register the electron binary plus our entry point.
    app.setAsDefaultProtocolClient(PROTOCOL_SCHEME, process.execPath, [path.resolve(process.argv[1])]);
    return;
  }
  app.setAsDefaultProtocolClient(PROTOCOL_SCHEME);
}

// ---------------------------------------------------------------------------
// Window

function createWindow(): BrowserWindow {
  const window = new BrowserWindow({
    title: "KubeLoop",
    width: 1080,
    height: 720,
    minWidth: 760,
    minHeight: 480,
    show: false,
    backgroundColor: BACKGROUND,
    frame: process.platform === "darwin",
    titleBarStyle: process.platform === "darwin" ? "hidden" : undefined,
    trafficLightPosition: process.platform === "darwin" ? { x: 14, y: 14 } : undefined,
    webPreferences: {
      preload: preloadPath(),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      spellcheck: false,
    },
  });
  window.setMenuBarVisibility(false);
  window.once("ready-to-show", () => window.show());
  window.on("close", (event) => {
    // Match the previous HideWindowOnClose behaviour: the app keeps running
    // in the tray until the user quits explicitly.
    if (!quitting) {
      event.preventDefault();
      window.hide();
    }
  });
  window.on("closed", () => {
    mainWindow = null;
  });
  window.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url);
    return { action: "deny" };
  });
  window.webContents.on("will-navigate", (event, url) => {
    const allowed = devServerURL();
    if (allowed && url.startsWith(allowed)) return;
    if (url.startsWith("file://")) return;
    event.preventDefault();
    void shell.openExternal(url);
  });

  const dev = devServerURL();
  if (dev) {
    void window.loadURL(dev);
  } else {
    void window.loadFile(rendererIndexPath());
  }
  return window;
}

function showWindow() {
  if (!mainWindow) {
    mainWindow = createWindow();
    return;
  }
  if (mainWindow.isMinimized()) mainWindow.restore();
  mainWindow.show();
  mainWindow.focus();
}

function createTray() {
  let icon = nativeImage.createFromPath(trayIconPath());
  if (!icon.isEmpty()) icon = icon.resize({ width: 18, height: 18 });
  tray = new Tray(icon);
  tray.setToolTip("KubeLoop");
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: "Open KubeLoop", click: () => showWindow() },
      { type: "separator" },
      { label: "Quit KubeLoop", click: () => app.quit() },
    ]),
  );
  tray.on("click", () => showWindow());
}

// ---------------------------------------------------------------------------
// Backend

function trustedSender(event: IpcMainInvokeEvent): boolean {
  return mainWindow !== null && event.sender === mainWindow.webContents;
}

function startBackend(): Backend {
  const process_ = new Backend({
    binary: backendBinaryPath(),
    cwd: backendWorkingDirectory(),
    onEvent: (name, payload) => {
      mainWindow?.webContents.send("backend:event", name, payload);
    },
    onLog: (line) => process.stderr.write(line + "\n"),
    onExit: (code, signal) => {
      if (quitting) return;
      console.error(`backend exited unexpectedly (code=${code}, signal=${signal})`);
      dialog.showErrorBox("KubeLoop", "The KubeLoop backend stopped unexpectedly. The application will close.");
      backendStopped = true;
      app.quit();
    },
    hostHandlers: {
      "host.openURL": async (params) => {
        const { url } = params as { url: string };
        if (!/^https?:\/\//i.test(url)) throw new Error("only http(s) URLs may be opened");
        await shell.openExternal(url);
        return null;
      },
      "host.showWindow": () => {
        showWindow();
      },
      "host.quit": () => {
        app.quit();
      },
      "host.openFileDialog": async (params) => {
        const { title } = params as { title: string };
        const result = await dialog.showOpenDialog(dialogParent(), { title, properties: ["openFile"] });
        return { path: result.canceled ? "" : (result.filePaths[0] ?? "") };
      },
      "host.openDirectoryDialog": async (params) => {
        const { title } = params as { title: string };
        const result = await dialog.showOpenDialog(dialogParent(), {
          title,
          properties: ["openDirectory", "createDirectory"],
        });
        return { path: result.canceled ? "" : (result.filePaths[0] ?? "") };
      },
      "host.saveFileDialog": async (params) => {
        const { title, defaultName } = params as { title: string; defaultName?: string };
        const result = await dialog.showSaveDialog(dialogParent(), { title, defaultPath: defaultName });
        return { path: result.canceled ? "" : (result.filePath ?? "") };
      },
    },
  });
  return process_;
}

function dialogParent(): BrowserWindow {
  if (!mainWindow) mainWindow = createWindow();
  return mainWindow;
}

function registerIpc() {
  ipcMain.handle("backend:call", async (event, method: unknown, args: unknown) => {
    if (!trustedSender(event)) throw new Error("untrusted sender");
    if (typeof method !== "string" || !Array.isArray(args)) throw new Error("invalid backend call");
    if (!backend) throw new Error("backend is not running");
    try {
      return await backend.call(method, args);
    } catch (error) {
      // Electron serialises thrown errors to their message only; keep the
      // application error text intact for the renderer.
      if (error instanceof BackendError) throw new Error(error.message);
      throw error;
    }
  });
  ipcMain.handle("window:control", (event, action: unknown) => {
    if (!trustedSender(event) || !mainWindow) return false;
    switch (action) {
      case "minimize":
        mainWindow.minimize();
        return true;
      case "toggleMaximize":
        if (mainWindow.isMaximized()) mainWindow.unmaximize();
        else mainWindow.maximize();
        return mainWindow.isMaximized();
      case "hide":
        mainWindow.hide();
        return true;
      case "isMaximized":
        return mainWindow.isMaximized();
      default:
        return false;
    }
  });
  ipcMain.handle("window:setTheme", (event, theme: unknown) => {
    if (!trustedSender(event)) return;
    nativeTheme.themeSource = theme === "dark" ? "dark" : theme === "light" ? "light" : "system";
  });
  ipcMain.handle("app:platform", (event) => (trustedSender(event) ? process.platform : ""));
}

// ---------------------------------------------------------------------------
// Lifecycle

// The packaged renderer is fully self-contained: no remote scripts, styles or
// connections. Vite's dev server needs inline scripts and a websocket for HMR,
// so the policy only applies to file:// loads.
function applyContentSecurityPolicy() {
  if (devServerURL()) return;
  const policy = [
    "default-src 'self'",
    "script-src 'self'",
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    "font-src 'self' data:",
    "connect-src 'self'",
    "object-src 'none'",
    "base-uri 'none'",
    "form-action 'none'",
  ].join("; ");
  session.defaultSession.webRequest.onHeadersReceived((details, callback) => {
    callback({
      responseHeaders: { ...details.responseHeaders, "Content-Security-Policy": [policy] },
    });
  });
}

app.whenReady().then(async () => {
  applyContentSecurityPolicy();
  registerIpc();
  backend = startBackend();
  createTray();
  mainWindow = createWindow();
  try {
    await backend.start();
  } catch (error) {
    dialog.showErrorBox(
      "KubeLoop",
      `The KubeLoop backend could not start: ${error instanceof Error ? error.message : String(error)}`,
    );
    backendStopped = true;
    app.quit();
    return;
  }
  const initial = callbackFromArgs(process.argv);
  if (initial) pendingCallbacks.push(initial);
  for (const url of pendingCallbacks.splice(0)) deliverAuthCallback(url);
});

app.on("activate", () => showWindow());

app.on("window-all-closed", () => {
  // Keep running in the tray on every platform; quitting is explicit.
});

app.on("before-quit", (event) => {
  quitting = true;
  if (backendStopped || !backend) return;
  event.preventDefault();
  const current = backend;
  backendStopped = true;
  void current.shutdown().finally(() => {
    tray?.destroy();
    app.quit();
  });
});
