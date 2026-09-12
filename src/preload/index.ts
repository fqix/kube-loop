import { contextBridge, ipcRenderer } from "electron";

/**
 * The only surface the renderer sees. `call` reaches a bound method on the Go
 * backend by name with positional arguments; `on` subscribes to events the
 * backend pushes (session:state, update:state, …).
 */
// ipcRenderer.invoke wraps a rejection as
// "Error invoking remote method 'backend:call': Error: <message>"; the
// renderer shows backend errors verbatim, so hand it only the message.
const remotePrefix = /^Error invoking remote method '[^']+': (?:Error: )?/;

function cleanError(error: unknown): Error {
  const message = error instanceof Error ? error.message : String(error);
  return new Error(message.replace(remotePrefix, ""));
}

const kubeloop = {
  call: (method: string, ...args: unknown[]): Promise<unknown> =>
    ipcRenderer.invoke("backend:call", method, args).catch((error: unknown) => {
      throw cleanError(error);
    }),
  on: (name: string, callback: (payload: unknown) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, eventName: string, payload: unknown) => {
      if (eventName === name) callback(payload);
    };
    ipcRenderer.on("backend:event", listener);
    return () => ipcRenderer.removeListener("backend:event", listener);
  },
  window: {
    minimize: (): Promise<boolean> => ipcRenderer.invoke("window:control", "minimize"),
    toggleMaximize: (): Promise<boolean> => ipcRenderer.invoke("window:control", "toggleMaximize"),
    hide: (): Promise<boolean> => ipcRenderer.invoke("window:control", "hide"),
    isMaximized: (): Promise<boolean> => ipcRenderer.invoke("window:control", "isMaximized"),
    setTheme: (theme: "light" | "dark" | "system"): Promise<void> =>
      ipcRenderer.invoke("window:setTheme", theme),
  },
  platform: (): Promise<string> => ipcRenderer.invoke("app:platform"),
};

export type KubeLoopBridge = typeof kubeloop;

contextBridge.exposeInMainWorld("kubeloop", kubeloop);
