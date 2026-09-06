import { BrowserWindow, dialog, shell } from "electron";
import type { Handshake } from "./sidecar";

/**
 * Frames exchanged with the Go sidecar over its shell socket. The sidecar
 * pushes application events and, for anything only the shell can do — native
 * dialogs, opening a browser — calls back and waits for a result.
 */
type ShellFrame = {
  type: "event" | "call" | "result";
  id?: number;
  event?: string;
  method?: string;
  params?: Record<string, unknown>;
  data?: unknown;
  result?: unknown;
  error?: string;
};

const reconnectDelayMs = 500;

export type Bridge = {
  /** Invoke an application binding, rejecting with the Go-side error. */
  call: (method: string, args: unknown[]) => Promise<unknown>;
  close: () => void;
};

/**
 * Connect the shell to the sidecar. Application events are forwarded to the
 * renderer as `backend:event`, and shell requests are served here in the main
 * process so the renderer never handles the loopback token.
 */
export function connectBridge(handshake: Handshake, window: () => BrowserWindow | null): Bridge {
  const endpoint = `http://127.0.0.1:${handshake.port}`;
  let socket: WebSocket | null = null;
  let closed = false;

  const open = () => {
    if (closed) return;
    // The token rides as the negotiated subprotocol: a WebSocket handshake
    // cannot carry an Authorization header.
    socket = new WebSocket(`ws://127.0.0.1:${handshake.port}/shell`, [handshake.token]);
    socket.addEventListener("message", event => {
      void handleFrame(JSON.parse(String(event.data)) as ShellFrame);
    });
    socket.addEventListener("close", () => {
      socket = null;
      if (!closed) setTimeout(open, reconnectDelayMs);
    });
    socket.addEventListener("error", () => socket?.close());
  };

  const reply = (id: number, result: unknown, error?: string) => {
    socket?.send(JSON.stringify({ type: "result", id, result, error } satisfies ShellFrame));
  };

  const handleFrame = async (frame: ShellFrame) => {
    if (frame.type === "event" && frame.event) {
      window()?.webContents.send("backend:event", frame.event, frame.data);
      return;
    }
    if (frame.type !== "call" || !frame.method) return;
    try {
      const result = await runShellMethod(frame.method, frame.params ?? {}, window);
      if (frame.id) reply(frame.id, result);
    } catch (reason) {
      if (frame.id) reply(frame.id, null, reason instanceof Error ? reason.message : String(reason));
    }
  };

  open();

  return {
    async call(method, args) {
      const response = await fetch(`${endpoint}/rpc`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${handshake.token}`,
        },
        body: JSON.stringify({ method, args }),
      });
      if (!response.ok) {
        throw new Error(`KubeLoop backend rejected ${method}: HTTP ${response.status}`);
      }
      const payload = (await response.json()) as { result?: unknown; error?: string };
      if (payload.error) {
        throw new Error(payload.error);
      }
      return payload.result ?? null;
    },
    close() {
      closed = true;
      socket?.close();
    },
  };
}

/** Serve the capabilities the application can only get from the shell. */
async function runShellMethod(
  method: string,
  params: Record<string, unknown>,
  window: () => BrowserWindow | null,
): Promise<unknown> {
  const parent = window();
  const title = typeof params.title === "string" ? params.title : undefined;

  switch (method) {
    case "window.show": {
      if (parent) {
        if (parent.isMinimized()) parent.restore();
        parent.show();
        parent.focus();
      }
      return null;
    }
    case "app.quit": {
      // Let the will-quit handler stop the sidecar rather than killing it here.
      const { app } = await import("electron");
      app.quit();
      return null;
    }
    case "shell.openExternal": {
      const url = String(params.url ?? "");
      await shell.openExternal(url);
      return null;
    }
    case "dialog.openFile":
    case "dialog.openDirectory": {
      const properties: Array<"openFile" | "openDirectory"> =
        method === "dialog.openFile" ? ["openFile"] : ["openDirectory"];
      const chosen = parent
        ? await dialog.showOpenDialog(parent, { title, properties })
        : await dialog.showOpenDialog({ title, properties });
      // An empty string is how the application reads "the user cancelled".
      return chosen.canceled ? "" : (chosen.filePaths[0] ?? "");
    }
    case "dialog.saveFile": {
      const defaultPath =
        typeof params.defaultFilename === "string" ? params.defaultFilename : undefined;
      const chosen = parent
        ? await dialog.showSaveDialog(parent, { title, defaultPath })
        : await dialog.showSaveDialog({ title, defaultPath });
      return chosen.canceled ? "" : (chosen.filePath ?? "");
    }
    default:
      throw new Error(`unsupported shell method ${method}`);
  }
}
