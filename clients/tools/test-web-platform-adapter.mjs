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
      export {
        loadRuntimeConfig,
        readAndroidNativeBridgeMetadata,
        readAndroidNativeStorageBridge,
        readClientShellConfig,
        readDesktopNativeBridgeMetadata,
        readDesktopNativeStorageBridge
      } from "../web/src/runtime-config";
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
    readAndroidNativeStorageBridge,
    readClientShellConfig,
    readDesktopNativeBridgeMetadata,
    readDesktopNativeStorageBridge
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
        runtimeLabel: "NexusIM Android shell",
        capabilities: {
          localStore: {
            currentDefault: "local-storage",
            productionTarget: "sqlite",
            nativeStoreReady: false,
            nativeStoreReason: "sqlite-native-bridge-unavailable",
            nativeStoreBridge: "android-sqlite"
          }
        }
      });
    }
  };
  const nativeMetadata = readAndroidNativeBridgeMetadata();
  assertEqual(nativeMetadata?.target, "android", "android native bridge metadata target");
  assertEqual(nativeMetadata?.nativeBridgeVersion, "0.1.0", "android native bridge metadata version");
  assertEqual(nativeMetadata?.runtimeLabel, "NexusIM Android shell", "android native bridge metadata label");
  assertEqual(nativeMetadata?.capabilities?.localStore?.ready, false, "android local store readiness");
  assertEqual(nativeMetadata?.capabilities?.localStore?.reason, "sqlite-native-bridge-unavailable", "android local store reason");
  assertEqual(nativeMetadata?.capabilities?.localStore?.bridge, "android-sqlite", "android local store bridge");
  assertEqual(
    nativeMetadata?.capabilities?.localStore?.nextAction,
    "android-sqlite is required before android can use sqlite local store",
    "android not-ready local store next action"
  );
  assertEqual(readAndroidNativeStorageBridge(nativeMetadata), undefined, "android native storage bridge rejects not-ready metadata");

  const androidNativeItems = new Map();
  globalThis.NexusIMNative = {
    runtimeMetadata() {
      return JSON.stringify({
        target: "android",
        nativeBridgeVersion: "0.2.0",
        runtimeLabel: "NexusIM Android shell",
        capabilities: {
          localStore: {
            currentDefault: "local-storage",
            productionTarget: "sqlite",
            nativeStoreReady: true,
            nativeStoreReason: "",
            nativeStoreBridge: "android-sqlite"
          }
        }
      });
    },
    localStoreGetItem(key) {
      return androidNativeItems.get(key) ?? null;
    },
    localStoreSetItem(key, value) {
      androidNativeItems.set(key, value);
    },
    localStoreRemoveItem(key) {
      androidNativeItems.delete(key);
    }
  };
  const readyAndroidMetadata = readAndroidNativeBridgeMetadata();
  assertEqual(readyAndroidMetadata?.capabilities?.localStore?.ready, true, "android ready native store metadata");
  assertEqual(
    readyAndroidMetadata?.capabilities?.localStore?.nextAction,
    "",
    "android ready native store has no next action"
  );
  const androidNativeStorage = readAndroidNativeStorageBridge(readyAndroidMetadata);
  assertEqual(typeof androidNativeStorage?.setItem, "function", "android ready native storage bridge");
  androidNativeStorage?.setItem("android-key", "android-value");
  assertEqual(androidNativeStorage?.getItem("android-key"), "android-value", "android native storage get item");
  androidNativeStorage?.removeItem("android-key");
  assertEqual(androidNativeStorage?.getItem("android-key"), null, "android native storage remove item");
  const nativeStoreAdapter = createBrowserPlatformAdapter({
    config: {
      apiBaseURL: "http://127.0.0.1:8080",
      pushWebSocketURL: "ws://127.0.0.1:8088/ws",
      deviceID: "android-native-store"
    },
    target: "android",
    nativeStorageBridge: androidNativeStorage
  });
  await nativeStoreAdapter.messageStore.upsertMessages([
    message("conv-native", 9, "android-native-message")
  ]);
  assertEqual(
    await nativeStoreAdapter.messageStore.getLastReceivedSeq("conv-native"),
    9,
    "webview adapter uses native storage bridge for cursor"
  );
  const reopenedNativeStoreAdapter = createBrowserPlatformAdapter({
    config: {
      apiBaseURL: "http://127.0.0.1:8080",
      pushWebSocketURL: "ws://127.0.0.1:8088/ws",
      deviceID: "android-native-store"
    },
    target: "android",
    nativeStorageBridge: androidNativeStorage
  });
  const cachedMessages = await reopenedNativeStoreAdapter.messageStore.listMessages("conv-native");
  assertEqual(cachedMessages.length, 1, "webview adapter reopens native storage bridge cache");
  assertEqual(cachedMessages[0]?.messageID, "android-native-message", "webview adapter native bridge message id");

  globalThis.NexusIMNative = {
    runtimeMetadata() {
      return JSON.stringify({
        target: "android",
        nativeBridgeVersion: "0.2.0",
        runtimeLabel: "NexusIM Android shell",
        capabilities: {
          localStore: {
            currentDefault: "local-storage",
            productionTarget: "sqlite",
            nativeStoreReady: true,
            nativeStoreReason: "",
            nativeStoreBridge: "android-sqlite"
          }
        }
      });
    },
    localStoreGetItem() {
      return null;
    }
  };
  assertEqual(
    readAndroidNativeStorageBridge(readAndroidNativeBridgeMetadata()),
    undefined,
    "android native storage bridge requires all storage methods"
  );

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
          runtimeLabel: "NexusIM desktop shell",
          capabilities: {
            localStore: {
              currentDefault: "local-storage",
              productionTarget: "sqlite",
              nativeStoreReady: false,
              nativeStoreReason: "sqlite-native-bridge-unavailable",
              nativeStoreBridge: "tauri-sqlite"
            }
          }
        });
      }
    }
  };
  const desktopMetadata = await readDesktopNativeBridgeMetadata();
  assertEqual(desktopMetadata?.target, "windows-desktop", "desktop native bridge metadata target");
  assertEqual(desktopMetadata?.nativeBridgeVersion, "0.1.0", "desktop native bridge metadata version");
  assertEqual(desktopMetadata?.runtimeLabel, "NexusIM desktop shell", "desktop native bridge metadata label");
  assertEqual(desktopMetadata?.capabilities?.localStore?.ready, false, "desktop local store readiness");
  assertEqual(desktopMetadata?.capabilities?.localStore?.reason, "sqlite-native-bridge-unavailable", "desktop local store reason");
  assertEqual(desktopMetadata?.capabilities?.localStore?.bridge, "tauri-sqlite", "desktop local store bridge");
  assertEqual(
    desktopMetadata?.capabilities?.localStore?.nextAction,
    "tauri-sqlite is required before windows-desktop can use sqlite local store",
    "desktop not-ready local store next action"
  );
  assertEqual(readDesktopNativeStorageBridge(desktopMetadata), undefined, "desktop native storage bridge rejects not-ready metadata");

  const desktopNativeItems = new Map();
  const desktopInvocations = [];
  globalThis.__TAURI__ = {
    core: {
      async invoke(command, args = {}) {
        desktopInvocations.push(command);
        if (command === "runtime_metadata") {
          return JSON.stringify({
            target: "windows-desktop",
            nativeBridgeVersion: "0.2.0",
            runtimeLabel: "NexusIM desktop shell",
            capabilities: {
              localStore: {
                currentDefault: "local-storage",
                productionTarget: "sqlite",
                nativeStoreReady: true,
                nativeStoreReason: "",
                nativeStoreBridge: "tauri-sqlite"
              }
            }
          });
        }
        if (command === "local_store_get_item") {
          return desktopNativeItems.get(args.key) ?? null;
        }
        if (command === "local_store_set_item") {
          desktopNativeItems.set(args.key, args.value);
          return null;
        }
        if (command === "local_store_remove_item") {
          desktopNativeItems.delete(args.key);
          return null;
        }
        throw new Error(`unexpected command ${command}`);
      }
    }
  };
  const readyDesktopMetadata = await readDesktopNativeBridgeMetadata();
  assertEqual(readyDesktopMetadata?.capabilities?.localStore?.ready, true, "desktop ready native store metadata");
  assertEqual(
    readyDesktopMetadata?.capabilities?.localStore?.nextAction,
    "",
    "desktop ready native store has no next action"
  );
  const desktopNativeStorage = readDesktopNativeStorageBridge(readyDesktopMetadata);
  assertEqual(typeof desktopNativeStorage?.setItem, "function", "desktop ready native storage bridge");
  await desktopNativeStorage?.setItem("desktop-key", "desktop-value");
  assertEqual(await desktopNativeStorage?.getItem("desktop-key"), "desktop-value", "desktop native storage get item");
  await desktopNativeStorage?.removeItem("desktop-key");
  assertEqual(await desktopNativeStorage?.getItem("desktop-key"), null, "desktop native storage remove item");
  assertEqual(desktopInvocations.includes("local_store_get_item"), true, "desktop bridge get command invoked");
  assertEqual(desktopInvocations.includes("local_store_set_item"), true, "desktop bridge set command invoked");
  assertEqual(desktopInvocations.includes("local_store_remove_item"), true, "desktop bridge remove command invoked");

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

function message(conversationID, conversationSeq, messageID) {
  return {
    tenantID: "tenant-1",
    conversationID,
    messageID,
    senderUserID: "user-1",
    conversationSeq,
    contentType: "TEXT",
    text: "cached",
    status: "DELIVERED",
    createdAtMs: conversationSeq
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
