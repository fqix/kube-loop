import { spawn, type ChildProcessByStdio } from "node:child_process";
import { createInterface } from "node:readline";
import type { Readable, Writable } from "node:stream";

/**
 * JSON-RPC 2.0 client for the Go backend sidecar (internal/desktopipc).
 * Frames are newline-delimited JSON over the child's stdin/stdout; stderr is
 * the backend's structured log and is forwarded to our own stderr.
 */

export interface RpcError {
  code: number;
  message: string;
  data?: unknown;
}

export class BackendError extends Error {
  readonly code: number;
  readonly data: unknown;
  constructor(error: RpcError) {
    super(error.message);
    this.name = "BackendError";
    this.code = error.code;
    this.data = error.data;
  }
}

interface RpcMessage {
  jsonrpc: "2.0";
  id?: string | number | null;
  method?: string;
  params?: unknown;
  result?: unknown;
  error?: RpcError;
}

export type HostHandler = (params: unknown) => Promise<unknown> | unknown;

export interface BackendOptions {
  binary: string;
  env?: NodeJS.ProcessEnv;
  cwd?: string;
  onEvent: (name: string, payload: unknown) => void;
  onExit: (code: number | null, signal: NodeJS.Signals | null) => void;
  onLog?: (line: string) => void;
  hostHandlers: Record<string, HostHandler>;
}

type Pending = {
  resolve: (value: unknown) => void;
  reject: (reason: Error) => void;
};

export class Backend {
  private readonly options: BackendOptions;
  private child: ChildProcessByStdio<Writable, Readable, Readable> | null = null;
  private nextId = 0;
  private readonly pending = new Map<string, Pending>();
  private readyResolve: ((methods: string[]) => void) | null = null;
  private readyReject: ((reason: Error) => void) | null = null;
  private readonly ready: Promise<string[]>;
  private methods = new Set<string>();
  private stopped = false;

  constructor(options: BackendOptions) {
    this.options = options;
    this.ready = new Promise<string[]>((resolve, reject) => {
      this.readyResolve = resolve;
      this.readyReject = reject;
    });
  }

  /** Spawns the sidecar and resolves once it announces its bound methods. */
  start(): Promise<string[]> {
    const child = spawn(this.options.binary, [], {
      cwd: this.options.cwd,
      env: this.options.env ?? process.env,
      stdio: ["pipe", "pipe", "pipe"],
      windowsHide: true,
    });
    this.child = child;
    child.on("error", (error) => {
      this.readyReject?.(error);
      this.failPending(error);
    });
    child.on("exit", (code, signal) => {
      const reason = new Error(`backend exited (code=${code}, signal=${signal})`);
      this.readyReject?.(reason);
      this.failPending(reason);
      this.child = null;
      if (!this.stopped) this.options.onExit(code, signal);
    });
    createInterface({ input: child.stdout, crlfDelay: Infinity }).on("line", (line) => {
      void this.handleLine(line);
    });
    createInterface({ input: child.stderr, crlfDelay: Infinity }).on("line", (line) => {
      this.options.onLog?.(line);
    });
    return this.ready;
  }

  hasMethod(name: string): boolean {
    return this.methods.has(name);
  }

  /** Calls a bound application method with positional arguments. */
  async call<T = unknown>(method: string, args: unknown[] = []): Promise<T> {
    await this.ready;
    return this.request<T>(method, args);
  }

  /** Asks the backend to run its shutdown hooks, then waits for it to exit. */
  async shutdown(timeoutMs = 8000): Promise<void> {
    const child = this.child;
    if (!child) return;
    this.stopped = true;
    const exited = new Promise<void>((resolve) => child.once("exit", () => resolve()));
    try {
      await Promise.race([
        this.request("backend.shutdown", undefined),
        new Promise<void>((_, reject) => setTimeout(() => reject(new Error("shutdown timeout")), timeoutMs)),
      ]);
    } catch {
      // Fall through to a hard stop below.
    }
    child.stdin.end();
    await Promise.race([exited, new Promise<void>((resolve) => setTimeout(resolve, timeoutMs))]);
    if (this.child) {
      child.kill("SIGKILL");
    }
  }

  private request<T>(method: string, params: unknown): Promise<T> {
    const child = this.child;
    if (!child) return Promise.reject(new Error("backend is not running"));
    const id = String(++this.nextId);
    const frame: RpcMessage = { jsonrpc: "2.0", id, method, params };
    return new Promise<T>((resolve, reject) => {
      this.pending.set(id, { resolve: resolve as (value: unknown) => void, reject });
      child.stdin.write(JSON.stringify(frame) + "\n", (error) => {
        if (error) {
          this.pending.delete(id);
          reject(error);
        }
      });
    });
  }

  private write(frame: RpcMessage) {
    this.child?.stdin.write(JSON.stringify(frame) + "\n");
  }

  private async handleLine(line: string) {
    let message: RpcMessage;
    try {
      message = JSON.parse(line) as RpcMessage;
    } catch {
      this.options.onLog?.(`backend emitted a non-JSON frame: ${line}`);
      return;
    }
    if (message.method === undefined && message.id !== undefined && message.id !== null) {
      this.settle(message);
      return;
    }
    if (message.method === "host.ready") {
      const params = message.params as { methods?: string[] } | undefined;
      this.methods = new Set(params?.methods ?? []);
      this.readyResolve?.(params?.methods ?? []);
      return;
    }
    if (message.method === "host.event") {
      const params = message.params as { name: string; payload: unknown };
      this.options.onEvent(params.name, params.payload);
      return;
    }
    if (message.method) {
      await this.handleHostRequest(message);
    }
  }

  private settle(message: RpcMessage) {
    const key = String(message.id);
    const waiter = this.pending.get(key);
    if (!waiter) return;
    this.pending.delete(key);
    if (message.error) {
      waiter.reject(new BackendError(message.error));
    } else {
      waiter.resolve(message.result);
    }
  }

  private async handleHostRequest(message: RpcMessage) {
    const handler = this.options.hostHandlers[message.method!];
    const isRequest = message.id !== undefined && message.id !== null;
    if (!handler) {
      if (isRequest) {
        this.write({
          jsonrpc: "2.0",
          id: message.id,
          error: { code: -32601, message: `host method ${message.method} is not supported` },
        });
      }
      return;
    }
    try {
      const result = await handler(message.params);
      if (isRequest) this.write({ jsonrpc: "2.0", id: message.id, result: result ?? null });
    } catch (error) {
      if (isRequest) {
        this.write({
          jsonrpc: "2.0",
          id: message.id,
          error: { code: -32000, message: error instanceof Error ? error.message : String(error) },
        });
      }
    }
  }

  private failPending(reason: Error) {
    for (const waiter of this.pending.values()) waiter.reject(reason);
    this.pending.clear();
  }
}
