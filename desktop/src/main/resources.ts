import { existsSync } from "node:fs";
import { join, resolve } from "node:path";
import { app } from "electron";

/**
 * Resolve a file that ships beside the application.
 *
 * A packaged app finds it under Electron's resources path, which is where
 * Forge copies `extraResource` entries. A development run has no such bundle,
 * so the same files are read from the repository instead.
 */
export function resourcePath(name: string, developmentPath: string): string {
  const packaged = join(process.resourcesPath, name);
  if (app.isPackaged || existsSync(packaged)) {
    return packaged;
  }
  return resolve(repositoryRoot(), developmentPath);
}

/** The repository root, relative to the app directory Forge builds into. */
export function repositoryRoot(): string {
  return resolve(app.getAppPath(), "..");
}
