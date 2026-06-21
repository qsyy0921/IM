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

  const { BrowserSessionStore, createBrowserPlatformAdapter } = await import(
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
