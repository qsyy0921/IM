interface BrowserShellConfig {
  readonly target?: "browser" | "windows-desktop" | "android";
}

export function registerBrowserPWA(): void {
  if (typeof window === "undefined" || !("serviceWorker" in navigator)) {
    return;
  }
  const shell = (globalThis as { __NEXUSIM_CLIENT_SHELL__?: BrowserShellConfig }).__NEXUSIM_CLIENT_SHELL__;
  if (shell?.target && shell.target !== "browser") {
    return;
  }
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/nexusim-sw.js", { scope: "/" }).catch(() => undefined);
  });
}
