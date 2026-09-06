import { spawn, type ChildProcessByStdio } from "node:child_process";
import type { Readable } from "node:stream";
import { existsSync } from "node:fs";
import { app } from "electron";
import { resourcePath } from "./resources";

/** The one line the Go sidecar prints on stdout once it is listening. */
export type Handshake = { port: number; token: string };

const handshakeTimeoutMs = 30_000;

function binaryName() {
  return process.platform === "win32" ? "kubeloop-desktop-host.exe" : "kubeloop-desktop-host";
}

/**
 * Resolve the sidecar binary. During development it is the binary
 * `npm run build:host` writes into build/bin, which is also where the Go
 * tooling stages sing-box.
 */
function sidecarPath() {
  return resourcePath(binaryName(), `build/bin/${binaryName()}`);
}

/** The sidecar's stdin is closed; stdout carries the handshake and stderr the logs. */
type SidecarProcess = ChildProcessByStdio<null, Readable, Readable>;

export type Sidecar = {
  handshake: Handshake;
  process: SidecarProcess;
  stop: () => void;
};

/**
 * Start the Go application process and wait for its handshake. Its stderr is
 * forwarded so the backend's structured logs reach the same console as the
 * shell's own output.
 */
export async function startSidecar(): Promise<Sidecar> {
  const executable = sidecarPath();
  if (!existsSync(executable)) {
    throw new Error(
      `KubeLoop backend not found at ${executable}. Run "npm run build:host" from the repository root.`,
    );
  }

  const child = spawn(executable, [], { stdio: ["ignore", "pipe", "pipe"] });
  child.stderr.setEncoding("utf8");
  child.stderr.on("data", (chunk: string) => process.stderr.write(chunk));

  const handshake = await readHandshake(child);
  const stop = () => {
    if (!child.killed) {
      // SIGTERM lets the application run its ordered shutdown: data planes,
      // port forwards and the privileged TUN session all need releasing.
      child.kill("SIGTERM");
    }
  };
  app.once("will-quit", stop);
  return { handshake, process: child, stop };
}

function readHandshake(child: SidecarProcess): Promise<Handshake> {
  return new Promise((fulfil, reject) => {
    let buffered = "";
    const timer = setTimeout(() => {
      cleanUp();
      child.kill("SIGKILL");
      reject(new Error("KubeLoop backend did not report a handshake in time"));
    }, handshakeTimeoutMs);

    const onData = (chunk: string) => {
      buffered += chunk;
      const newline = buffered.indexOf("\n");
      if (newline < 0) return;
      cleanUp();
      try {
        fulfil(JSON.parse(buffered.slice(0, newline)) as Handshake);
      } catch (reason) {
        reject(new Error(`KubeLoop backend sent an unreadable handshake: ${String(reason)}`));
      }
    };
    const onExit = (code: number | null) => {
      cleanUp();
      reject(new Error(`KubeLoop backend exited with code ${code ?? "unknown"} before starting`));
    };
    function cleanUp() {
      clearTimeout(timer);
      child.stdout.off("data", onData);
      child.off("exit", onExit);
    }

    child.stdout.setEncoding("utf8");
    child.stdout.on("data", onData);
    child.on("exit", onExit);
  });
}
