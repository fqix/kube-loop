import { join } from "node:path";
import {
  app,
  BrowserWindow,
  ipcMain,
  Menu,
  nativeTheme,
  Tray,
  type IpcMainInvokeEvent,
} from "electron";
import { connectBridge, type Bridge } from "./bridge";
import { resourcePath } from "./resources";
import { startSidecar, type Sidecar } from "./sidecar";

const protocolScheme = "kubeloop";
const backgroundColour = "#0f172a";

let mainWindow: BrowserWindow | null = null;
let tray: Tray | null = null;
let bridge: Bridge | null = null;
let sidecar: Sidecar | null = null;
/** Set on the real quit path so closing the window only hides it. */
let quitting = false;

/**
 * The renderer mounts and calls the backend before the sidecar has finished
 * starting, so calls wait here rather than failing. A deep link that arrives
 * during launch waits on the same promise.
 */
let announceBridge: (ready: Bridge) => void;
let failBridge: (reason: Error) => void;
const bridgeReady = new Promise<Bridge>((fulfil, reject) => {
  announceBridge = fulfil;
  failBridge = reject;
});

const window = () => mainWindow;

function authCallbackFrom(values: string[]): string | undefined {
  return values.find(value => value.toLowerCase().startsWith(`${protocolScheme}://`));
}

async function deliverAuthCallback(url: string) {
  showWindow();
  try {
    const ready = await bridgeReady;
    await ready.call("HandleAuthCallbackURL", [url]);
  } catch (reason) {
    console.error("OAuth callback rejected:", reason);
  }
}

function showWindow() {
  if (!mainWindow) return;
  if (mainWindow.isMinimized()) mainWindow.restore();
  mainWindow.show();
  mainWindow.focus();
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1080,
    height: 720,
    backgroundColor: backgroundColour,
    show: false,
    // macOS keeps its native traffic lights over the app's own title bar;
    // every other platform draws the controls in the renderer.
    ...(process.platform === "darwin"
      ? { titleBarStyle: "hiddenInset" as const }
      : { frame: false }),
    webPreferences: {
      preload: join(__dirname, "preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  mainWindow.once("ready-to-show", () => mainWindow?.show());
  mainWindow.on("close", event => {
    // The app keeps running in the tray, matching the previous shell.
    if (!quitting) {
      event.preventDefault();
      mainWindow?.hide();
    }
  });
  mainWindow.on("closed", () => {
    mainWindow = null;
  });

  if (MAIN_WINDOW_VITE_DEV_SERVER_URL) {
    void mainWindow.loadURL(MAIN_WINDOW_VITE_DEV_SERVER_URL);
  } else {
    void mainWindow.loadFile(join(__dirname, `../renderer/${MAIN_WINDOW_VITE_NAME}/index.html`));
  }
}

function createTray() {
  const icon = resourcePath("appicon.png", "packaging/icons/appicon.png");
  try {
    tray = new Tray(icon);
  } catch (reason) {
    console.error("System tray unavailable:", reason);
    return;
  }
  tray.setToolTip("KubeLoop");
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: "Open KubeLoop", click: showWindow },
      { type: "separator" },
      { label: "Quit KubeLoop", click: () => app.quit() },
    ]),
  );
  tray.on("click", showWindow);
}

function registerIpc() {
  // Errors are returned rather than thrown: Electron prefixes a thrown IPC
  // error with "Error invoking remote method ...", and the frontend shows these
  // messages to the user verbatim.
  ipcMain.handle("backend:call", async (_event: IpcMainInvokeEvent, method: string, args: unknown[]) => {
    try {
      const ready = await bridgeReady;
      return { value: await ready.call(method, Array.isArray(args) ? args : []) };
    } catch (reason) {
      return { error: reason instanceof Error ? reason.message : String(reason) };
    }
  });

  ipcMain.handle("window:minimise", () => mainWindow?.minimize());
  ipcMain.handle("window:hide", () => mainWindow?.hide());
  ipcMain.handle("window:toggle-maximise", () => {
    if (!mainWindow) return;
    if (mainWindow.isMaximized()) mainWindow.unmaximize();
    else mainWindow.maximize();
  });
  ipcMain.handle("window:is-maximised", () => mainWindow?.isMaximized() ?? false);
  ipcMain.handle("window:set-theme", (_event: IpcMainInvokeEvent, theme: string) => {
    nativeTheme.themeSource = theme === "dark" ? "dark" : "light";
  });
}

function registerProtocol() {
  if (process.defaultApp && process.argv.length >= 2) {
    // A development run is launched through the Electron binary, so the
    // registration has to name the script being run.
    app.setAsDefaultProtocolClient(protocolScheme, process.execPath, [process.argv[1]]);
  } else {
    app.setAsDefaultProtocolClient(protocolScheme);
  }
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", (_event, argv) => {
    const callback = authCallbackFrom(argv);
    if (callback) void deliverAuthCallback(callback);
    else showWindow();
  });

  // macOS delivers the callback through open-url, which can fire before ready.
  app.on("open-url", (event, url) => {
    event.preventDefault();
    void deliverAuthCallback(url);
  });

  app.on("before-quit", () => {
    quitting = true;
  });

  app.on("window-all-closed", () => {
    // The tray keeps the app alive on every platform, as the previous shell did.
  });

  app.on("activate", () => {
    if (mainWindow) showWindow();
    else createWindow();
  });

  void app.whenReady().then(async () => {
    registerProtocol();
    registerIpc();
    createWindow();
    createTray();

    try {
      sidecar = await startSidecar();
    } catch (reason) {
      const failure = reason instanceof Error ? reason : new Error(String(reason));
      console.error(failure);
      failBridge(failure);
      app.quit();
      return;
    }
    sidecar.process.on("exit", code => {
      if (!quitting) {
        console.error(`KubeLoop backend exited unexpectedly with code ${code ?? "unknown"}`);
        app.quit();
      }
    });

    bridge = connectBridge(sidecar.handshake, window);
    announceBridge(bridge);

    const launchCallback = authCallbackFrom(process.argv);
    if (launchCallback) void deliverAuthCallback(launchCallback);
  });

  app.on("will-quit", () => {
    bridge?.close();
  });
}
