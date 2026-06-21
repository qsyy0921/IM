import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import * as esbuild from "esbuild";

const tempDir = mkdtempSync(join(tmpdir(), "nexusim-web-platform-"));
const bundlePath = join(tempDir, "web-platform-entry.mjs");
const toolsDir = fileURLToPath(new URL(".", import.meta.url));
const entryPath = join(toolsDir, `.web-platform-entry-${process.pid}.ts`);

async function main() {
  installBrowserGlobals();
  writeFileSync(
    entryPath,
    `
      export { BrowserSessionStore, createBrowserPlatformAdapter } from "../web/src/platform-adapter";
      export { loadRuntimeConfig, readAndroidNativeBridgeMetadata, readClientShellConfig, readDesktopNativeBridgeMetadata } from "../web/src/runtime-config";
    `,
    "utf8"
  );

  await esbuild.build({
    entryPoints: [entryPath],
    bundle: true,
    platform: "browser",
    format: "esm",
    outfile: bundlePath,
    logLevel: "silent"
  });

  const {
    BrowserSessionStore,
    createBrowserPlatformAdapter,
    loadRuntimeConfig,
    readAndroidNativeBridgeMetadata,
    readClientShellConfig,
    readDesktopNativeBridgeMetadata
  } = await import(
    pathToFileURL(bundlePath).href
  );

  const store = new BrowserSessionStore("nexusim:web-platform-test");
  assertEqual(await store.loadSession(), null, "new browser session store is empty");
  await store.saveSession(session("token-1"));
  assertEqual((await store.loadSession())?.accessToken, "token-1", "session store loads saved token");

  const reopened = new BrowserSessionStore("nexusim:web-platform-test");
  assertEqual((await reopened.loadSession())?.sessionID, "session-1", "session survives store reopen");
  await reopened.clearSession();
  assertEqual(await store.loadSession(), null, "clear removes browser session");

  const adapter = createBrowserPlatformAdapter({
    config: {
      apiBaseURL: "http://127.0.0.1:8080",
      pushWebSocketURL: "ws://127.0.0.1:8088/ws",
      deviceID: "web-device"
    },
    sessionKey: "nexusim:web-platform-adapter"
  });
  assertEqual(adapter.identity.target, "browser", "browser adapter target");
  assertEqual(adapter.identity.deviceID, "web-device", "browser adapter device id");
  assertEqual(await adapter.networkState.current(), "ONLINE", "browser adapter reports online");
  assertEqual(adapter.lifecycle.current(), "ACTIVE", "browser adapter reports active lifecycle");
  assertEqual(await adapter.wakeupNotifications.requestPermission(), "UNSUPPORTED", "browser wakeup is unsupported");

  globalThis.__NEXUSIM_CLIENT_SHELL__ = {
    target: "windows-desktop",
    apiBaseURL: "http://172.31.50.1:8080",
    pushWebSocketURL: "ws://172.31.50.1:8088/ws",
    deviceID: "desktop-webview",
    installationID: "desktop-installation",
    appVersion: "0.2.0",
    sessionKey: "nexusim:desktop-webview",
    smokeCallbackURL: "http://127.0.0.1:49152/shell-smoke",
    smokeRunID: "desktop-smoke-1",
    smokeMode: "metadata"
  };
  const shellConfig = readClientShellConfig();
  const shellRuntimeConfig = loadRuntimeConfig();
  assertEqual(shellConfig.target, "windows-desktop", "shell config preserves desktop target");
  assertEqual(shellConfig.smokeCallbackURL, "http://127.0.0.1:49152/shell-smoke", "shell config preserves loopback smoke callback");
  assertEqual(shellConfig.smokeRunID, "desktop-smoke-1", "shell config preserves smoke run id");
  assertEqual(shellConfig.smokeMode, "metadata", "shell config preserves smoke mode");
  assertEqual(shellRuntimeConfig.apiBaseURL, "http://172.31.50.1:8080", "shell config overrides api base");
  assertEqual(shellRuntimeConfig.pushWebSocketURL, "ws://172.31.50.1:8088/ws", "shell config overrides push url");
  assertEqual(shellRuntimeConfig.deviceID, "desktop-webview", "shell config overrides device id");

  const shellAdapter = createBrowserPlatformAdapter({
    config: shellRuntimeConfig,
    target: shellConfig.target,
    installationID: shellConfig.installationID,
    appVersion: shellConfig.appVersion,
    sessionKey: shellConfig.sessionKey
  });
  assertEqual(shellAdapter.identity.target, "windows-desktop", "webview shell uses desktop target");
  assertEqual(shellAdapter.identity.installationID, "desktop-installation", "webview shell uses injected installation id");
  assertEqual(shellAdapter.identity.appVersion, "0.2.0", "webview shell uses injected app version");

  globalThis.__NEXUSIM_CLIENT_SHELL__ = {
    target: "android",
    deviceID: "android-webview",
    sessionKey: "nexusim:android-webview"
  };
  const androidShellConfig = readClientShellConfig();
  const androidRuntimeConfig = loadRuntimeConfig();
  const androidAdapter = createBrowserPlatformAdapter({
    config: androidRuntimeConfig,
    target: androidShellConfig.target,
    sessionKey: androidShellConfig.sessionKey
  });
  assertEqual(androidRuntimeConfig.deviceID, "android-webview", "android shell config overrides device id");
  assertEqual(androidAdapter.identity.target, "android", "webview shell uses android target");

  globalThis.NexusIMNative = {
    runtimeMetadata() {
      return JSON.stringify({
        target: "android",
        nativeBridgeVersion: "0.1.0",
        runtimeLabel: "NexusIM Android shell"
      });
    }
  };
  const nativeMetadata = readAndroidNativeBridgeMetadata();
  assertEqual(nativeMetadata?.target, "android", "android native bridge metadata target");
  assertEqual(nativeMetadata?.nativeBridgeVersion, "0.1.0", "android native bridge metadata version");
  assertEqual(nativeMetadata?.runtimeLabel, "NexusIM Android shell", "android native bridge metadata label");

  globalThis.NexusIMNative = {
    runtimeMetadata() {
      return JSON.stringify({ target: "windows-desktop" });
    }
  };
  assertEqual(readAndroidNativeBridgeMetadata(), undefined, "android native bridge rejects non-android target");

  globalThis.NexusIMNative = {
    runtimeMetadata() {
      return "not-json";
    }
  };
  assertEqual(readAndroidNativeBridgeMetadata(), undefined, "android native bridge rejects malformed json");

  globalThis.__TAURI__ = {
    core: {
      async invoke(command) {
        assertEqual(command, "runtime_metadata", "desktop native bridge command");
        return JSON.stringify({
          target: "windows-desktop",
          nativeBridgeVersion: "0.1.0",
          runtimeLabel: "NexusIM desktop shell"
        });
      }
    }
  };
  const desktopMetadata = await readDesktopNativeBridgeMetadata();
  assertEqual(desktopMetadata?.target, "windows-desktop", "desktop native bridge metadata target");
  assertEqual(desktopMetadata?.nativeBridgeVersion, "0.1.0", "desktop native bridge metadata version");
  assertEqual(desktopMetadata?.runtimeLabel, "NexusIM desktop shell", "desktop native bridge metadata label");

  globalThis.__TAURI__ = {
    core: {
      async invoke() {
        return JSON.stringify({ target: "android" });
      }
    }
  };
  assertEqual(await readDesktopNativeBridgeMetadata(), undefined, "desktop native bridge rejects non-desktop target");

  globalThis.__TAURI__ = {
    core: {
      async invoke() {
        return "not-json";
      }
    }
  };
  assertEqual(await readDesktopNativeBridgeMetadata(), undefined, "desktop native bridge rejects malformed json");

  globalThis.__TAURI__ = {
    core: {
      async invoke() {
        return { target: "windows-desktop" };
      }
    }
  };
  assertEqual(await readDesktopNativeBridgeMetadata(), undefined, "desktop native bridge rejects non-string response");

  console.log("web platform adapter ok");
}

function session(accessToken) {
  return {
    tenantID: "tenant-1",
    userID: "user-1",
    deviceID: "web-device",
    sessionID: "session-1",
    accessToken,
    refreshToken: "refresh-1"
  };
}

function installBrowserGlobals() {
  const storage = new Map();
  Object.defineProperty(globalThis, "sessionStorage", {
    configurable: true,
    value: {
    getItem(key) {
      return storage.get(key) ?? null;
    },
    setItem(key, value) {
      storage.set(key, String(value));
    },
    removeItem(key) {
      storage.delete(key);
    },
    clear() {
      storage.clear();
    }
    }
  });
  Object.defineProperty(globalThis, "navigator", {
    configurable: true,
    value: { onLine: true }
  });
  const listeners = new Map();
  Object.defineProperty(globalThis, "window", {
    configurable: true,
    value: {
    addEventListener(type, listener) {
      listeners.set(`${type}:${listeners.size}`, listener);
    },
    removeEventListener() {
      return;
    }
    }
  });
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: {
    hidden: false,
    addEventListener(type, listener) {
      listeners.set(`document:${type}:${listeners.size}`, listener);
    },
    removeEventListener() {
      return;
    }
    }
  });
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

try {
  await main();
} finally {
  try {
    rmSync(entryPath, { force: true });
  } catch {
    // The temp entry may not exist if startup failed before it was written.
  }
  rmSync(tempDir, { recursive: true, force: true });
}
