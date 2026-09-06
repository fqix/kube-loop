/**
 * Window and theme controls provided by the desktop shell.
 *
 * The shell injects these on `window.runtime`. Reading them lazily — rather
 * than importing a shell-specific module — keeps the components usable in the
 * browser-only dev server and in the node test runner, where no shell exists.
 */

type ShellRuntime = NonNullable<Window["runtime"]>;

function shell(): Partial<ShellRuntime> {
  return typeof window === "undefined" ? {} : (window.runtime ?? {});
}

/** Whether a desktop shell is present to service window controls. */
export function shellAvailable(): boolean {
  return typeof window !== "undefined" && Boolean(window.runtime);
}

export function WindowMinimise() {
  shell().WindowMinimise?.();
}

export function WindowHide() {
  shell().WindowHide?.();
}

export function WindowToggleMaximise() {
  shell().WindowToggleMaximise?.();
}

export function WindowIsMaximised(): Promise<boolean> {
  return shell().WindowIsMaximised?.() ?? Promise.resolve(false);
}

export function WindowSetDarkTheme() {
  shell().WindowSetDarkTheme?.();
}

export function WindowSetLightTheme() {
  shell().WindowSetLightTheme?.();
}
