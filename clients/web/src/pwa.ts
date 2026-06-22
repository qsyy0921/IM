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
  if (isDevelopmentBuild()) {
    window.addEventListener("load", () => {
      void unregisterDevelopmentPWA();
    });
    return;
  }
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/nexusim-sw.js", { scope: "/" }).catch(() => undefined);
  });
}

function isDevelopmentBuild(): boolean {
  const meta = import.meta as ImportMeta & { env?: { DEV?: boolean } };
  return meta.env?.DEV === true;
}

async function unregisterDevelopmentPWA(): Promise<void> {
  const registrations = await navigator.serviceWorker.getRegistrations();
  await Promise.all(registrations.map(registration => registration.unregister()));
  if (!("caches" in globalThis)) {
    return;
  }
  const names = await caches.keys();
  await Promise.all(names.filter(name => name.startsWith("nexusim-")).map(name => caches.delete(name)));
}
