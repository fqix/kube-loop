import { app } from "electron";
import { existsSync } from "node:fs";
import path from "node:path";

export const PROTOCOL_SCHEME = "kubeloop";

/** Repository root when running from source (out/main → root). */
export function repositoryRoot(): string {
  return path.resolve(__dirname, "..", "..");
}

function backendBinaryName(): string {
  return process.platform === "win32" ? "kubeloop-backend.exe" : "kubeloop-backend";
}

/**
 * The Go backend lives next to the other bundled tools: under resources/ in a
 * packaged app, under build/bin when developing. KUBELOOP_BACKEND_PATH wins so
 * a debugger-built binary can be substituted.
 */
export function backendBinaryPath(): string {
  const override = process.env.KUBELOOP_BACKEND_PATH?.trim();
  if (override) return override;
  if (app.isPackaged) return path.join(process.resourcesPath, backendBinaryName());
  return path.join(repositoryRoot(), "build", "bin", backendBinaryName());
}

/** Working directory for the backend: dev-mode lookups expect the repo root. */
export function backendWorkingDirectory(): string {
  return app.isPackaged ? process.resourcesPath : repositoryRoot();
}

export function trayIconPath(): string {
  if (app.isPackaged) return path.join(process.resourcesPath, "appicon.png");
  return path.join(repositoryRoot(), "build", "appicon.png");
}

export function rendererIndexPath(): string {
  return path.join(__dirname, "..", "renderer", "index.html");
}

/** electron-vite exposes the renderer dev server here during `electron-vite dev`. */
export function devServerURL(): string | undefined {
  const url = process.env.ELECTRON_RENDERER_URL?.trim();
  return url || undefined;
}

export function preloadPath(): string {
  const candidate = path.join(__dirname, "..", "preload", "index.js");
  if (!existsSync(candidate)) throw new Error(`preload script missing at ${candidate}`);
  return candidate;
}
