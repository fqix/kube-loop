import { contextBridge, ipcRenderer } from "electron";

/**
 * Rebuild the globals the previous shell injected. The frontend reaches the Go
 * application through `window.go.app.App` and subscribes to backend events
 * through `window.runtime.EventsOn`, so keeping both shapes means no call site
 * in the React application has to know which shell it is running under.
 */

type EventCallback = (payload: never) => void;

const subscribers = new Map<string, Set<EventCallback>>();

ipcRenderer.on("backend:event", (_event, name: string, payload: unknown) => {
  for (const callback of subscribers.get(name) ?? []) {
    callback(payload as never);
  }
});

/**
 * The primitives handed to the main world. Each is a plain function, which is
 * all the context bridge can carry across the isolation boundary.
 */
const shell = {
  call: async (method: string, args: unknown[]): Promise<unknown> => {
    const reply = (await ipcRenderer.invoke("backend:call", method, args)) as {
      value?: unknown;
      error?: string;
    };
    if (reply.error !== undefined) {
      // Reject with the backend's own message, not Electron's IPC wrapper.
      throw new Error(reply.error);
    }
    return reply.value ?? null;
  },
  subscribe: (event: string, callback: EventCallback): (() => void) => {
    let listeners = subscribers.get(event);
    if (!listeners) {
      listeners = new Set();
      subscribers.set(event, listeners);
    }
    listeners.add(callback);
    return () => {
      listeners?.delete(callback);
      if (listeners?.size === 0) subscribers.delete(event);
    };
  },
  minimise: () => void ipcRenderer.invoke("window:minimise"),
  hide: () => void ipcRenderer.invoke("window:hide"),
  toggleMaximise: () => void ipcRenderer.invoke("window:toggle-maximise"),
  isMaximised: (): Promise<boolean> => ipcRenderer.invoke("window:is-maximised"),
  setTheme: (theme: string) => void ipcRenderer.invoke("window:set-theme", theme),
};

// A Proxy cannot be cloned across the context bridge, so it is built in the
// main world instead, over the plain functions passed in as arguments. The
// function below is serialized: it can only use what it receives.
contextBridge.executeInMainWorld({
  func: (api: typeof shell) => {
    const application = new Proxy(
      {},
      {
        get(_target, property) {
          if (typeof property !== "string") return undefined;
          // Every Go binding is reachable by name, so a new binding needs no
          // change here.
          return (...args: unknown[]) => api.call(property, args);
        },
        has: () => true,
      },
    );
    Object.defineProperty(window, "go", {
      value: { app: { App: application } },
      configurable: false,
      writable: false,
    });
    Object.defineProperty(window, "runtime", {
      value: {
        EventsOn: api.subscribe,
        WindowMinimise: api.minimise,
        WindowHide: api.hide,
        WindowToggleMaximise: api.toggleMaximise,
        WindowIsMaximised: api.isMaximised,
        WindowSetDarkTheme: () => api.setTheme("dark"),
        WindowSetLightTheme: () => api.setTheme("light"),
      },
      configurable: false,
      writable: false,
    });
  },
  args: [shell],
});
