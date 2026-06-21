import { mkdtempSync, rmSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import * as esbuild from "esbuild";

const tempDir = mkdtempSync(join(tmpdir(), "nexusim-client-runtime-"));
const bundlePath = join(tempDir, "runtime-entry.mjs");
const toolsDir = fileURLToPath(new URL(".", import.meta.url));
const entryPath = join(toolsDir, `.runtime-lifecycle-entry-${process.pid}.ts`);

async function main() {
  writeFileSync(
    entryPath,
    `
      export { createDesktopClientRuntime } from "../desktop/src/runtime";
      export { createDesktopShellActions } from "../desktop/src/shell-actions";
      export { createDesktopPlatformAdapter } from "../desktop/src/platform-adapter";
      export { createAndroidClientRuntime } from "../android/src/runtime";
      export { createAndroidShellActions } from "../android/src/shell-actions";
      export { createAndroidPlatformAdapter } from "../android/src/platform-adapter";
    `,
    "utf8"
  );

  await esbuild.build({
    entryPoints: [entryPath],
    bundle: true,
    platform: "node",
    format: "esm",
    outfile: bundlePath,
    logLevel: "silent"
  });

  const {
    createDesktopClientRuntime,
    createDesktopPlatformAdapter,
    createDesktopShellActions,
    createAndroidClientRuntime,
    createAndroidPlatformAdapter,
    createAndroidShellActions
  } = await import(pathToFileURL(bundlePath).href);

  await exerciseRuntime("desktop", createDesktopClientRuntime, {
    apiBaseURL: "http://bff.local",
    pushWebSocketURL: "ws://push.local/ws",
    deviceID: "desktop-device",
    os: "windows",
    secureStorage: "development",
    localStore: "memory",
    shell: "tauri"
  }, createDesktopShellActions);

  await exerciseRuntime("android", createAndroidClientRuntime, {
    apiBaseURL: "http://bff.local",
    pushWebSocketURL: "ws://push.local/ws",
    deviceID: "android-device",
    platform: "android",
    secureStorage: "development",
    localStore: "memory",
    notificationProvider: "none"
  }, createAndroidShellActions);

  assertThrows(
    () => createDesktopPlatformAdapter({
      config: {
        apiBaseURL: "http://bff.local",
        pushWebSocketURL: "ws://push.local/ws",
        deviceID: "desktop-device",
        os: "windows",
        secureStorage: "development",
        localStore: "sqlite",
        shell: "tauri"
      }
    }),
    "reason=sqlite-native-bridge-unavailable; bridge=tauri-sqlite",
    "desktop sqlite store must fail closed until native bridge exists"
  );

  await exerciseNativeBridgeStore("desktop", createDesktopPlatformAdapter, {
    apiBaseURL: "http://bff.local",
    pushWebSocketURL: "ws://push.local/ws",
    deviceID: "desktop-device",
    os: "windows",
    secureStorage: "development",
    localStore: "sqlite",
    shell: "tauri"
  });

  assertThrows(
    () => createAndroidPlatformAdapter({
      config: {
        apiBaseURL: "http://bff.local",
        pushWebSocketURL: "ws://push.local/ws",
        deviceID: "android-device",
        platform: "android",
        secureStorage: "development",
        localStore: "sqlite",
        notificationProvider: "none"
      }
    }),
    "reason=sqlite-native-bridge-unavailable; bridge=android-sqlite",
    "android sqlite store must fail closed until native bridge exists"
  );

  await exerciseNativeBridgeStore("android", createAndroidPlatformAdapter, {
    apiBaseURL: "http://bff.local",
    pushWebSocketURL: "ws://push.local/ws",
    deviceID: "android-device",
    platform: "android",
    secureStorage: "development",
    localStore: "sqlite",
    notificationProvider: "none"
  });

  console.log("client runtime lifecycle ok");
}

async function exerciseRuntime(label, createRuntime, config, createShellActions) {
  const calls = installFetchStub(label);
  const runtime = createRuntime({ config });
  const shellActions = createShellActions(runtime);
  const loginState = await shellActions.login({
    tenantID: "tenant-1",
    userID: "user-1",
    password: "password",
    deviceID: config.deviceID
  });
  assertEqual(loginState.authenticated, true, `${label} shell login reports authenticated`);
  assertEqual(loginState.sessionID, `${label}-session-1`, `${label} shell login returns session id`);
  assertEqual(runtime.auth.current()?.accessToken, `${label}-gateway-token-1`, `${label} login hydrates auth manager`);
  assertEqual(
    (await runtime.platform.secureSessionStore.loadSession())?.accessToken,
    `${label}-gateway-token-1`,
    `${label} login persists session`
  );

  await runtime.platform.messageStore.upsertMessages([
    {
      tenantID: "tenant-1",
      conversationID: "conv-1",
      messageID: `${label}-msg-1`,
      senderUserID: "user-1",
      conversationSeq: 7,
      contentType: "TEXT",
      text: "cached",
      status: "DELIVERED",
      createdAtMs: 7
    }
  ]);

  const restoredRuntime = createRuntime({ config, platform: runtime.platform });
  const restoredShellActions = createShellActions(restoredRuntime);
  const restoredState = await restoredShellActions.restoreSession();
  assertEqual(restoredState.authenticated, true, `${label} shell restore reports authenticated`);
  assertEqual(restoredState.sessionID, `${label}-session-1`, `${label} shell restore returns session id`);
  assertEqual(
    restoredRuntime.auth.current()?.accessToken,
    `${label}-gateway-token-1`,
    `${label} restore hydrates auth manager`
  );

  const refreshedState = await restoredShellActions.refresh();
  assertEqual(refreshedState.authenticated, true, `${label} shell refresh reports authenticated`);
  assertEqual(restoredRuntime.auth.current()?.accessToken, `${label}-gateway-token-2`, `${label} refresh returns new token`);
  assertEqual(
    (await runtime.platform.secureSessionStore.loadSession())?.refreshToken,
    `${label}-refresh-token-2`,
    `${label} refresh persists new refresh token`
  );

  const loggedOutState = await restoredShellActions.logout();
  assertEqual(loggedOutState.authenticated, false, `${label} shell logout reports unauthenticated`);
  assertEqual(restoredRuntime.auth.current(), null, `${label} logout clears auth manager`);
  assertEqual(await runtime.platform.secureSessionStore.loadSession(), null, `${label} logout clears session store`);
  assertEqual(
    await runtime.platform.messageStore.getLastReceivedSeq("conv-1"),
    0,
    `${label} logout clears message cache`
  );

  const logout = calls.find(call => call.path === "/api/auth/logout");
  assertEqual(
    logout?.authorization,
    `Bearer ${label}-gateway-token-2`,
    `${label} logout uses current refreshed token`
  );
}

async function exerciseNativeBridgeStore(label, createPlatformAdapter, config) {
  const bridge = new FakeNativeKeyValueBridge();
  const first = createPlatformAdapter({
    config,
    nativeStorageBridge: bridge
  });
  await first.messageStore.upsertMessages([
    {
      tenantID: "tenant-1",
      conversationID: "conv-native",
      messageID: `${label}-native-msg-1`,
      senderUserID: "user-1",
      conversationSeq: 11,
      contentType: "TEXT",
      text: "native-cache",
      status: "DELIVERED",
      createdAtMs: 11
    }
  ]);
  assertEqual(
    await first.messageStore.getLastReceivedSeq("conv-native"),
    11,
    `${label} native bridge store records cursor`
  );

  const reopened = createPlatformAdapter({
    config,
    nativeStorageBridge: bridge
  });
  assertEqual(
    await reopened.messageStore.getLastReceivedSeq("conv-native"),
    11,
    `${label} native bridge store persists cursor across platform adapter reopen`
  );
  const messages = await reopened.messageStore.listMessages("conv-native");
  assertEqual(messages.length, 1, `${label} native bridge store persists message`);
  assertEqual(messages[0]?.messageID, `${label}-native-msg-1`, `${label} native bridge message id mismatch`);

  await reopened.messageStore.clear();
  assertEqual(
    await first.messageStore.getLastReceivedSeq("conv-native"),
    0,
    `${label} native bridge store clear is visible across adapters`
  );
}

class FakeNativeKeyValueBridge {
  #items = new Map();

  getItem(key) {
    return this.#items.get(key) ?? null;
  }

  setItem(key, value) {
    this.#items.set(key, value);
  }

  removeItem(key) {
    this.#items.delete(key);
  }
}

function installFetchStub(label) {
  const calls = [];
  let refreshCount = 0;
  globalThis.fetch = async (url, init = {}) => {
    const parsed = new URL(String(url));
    const path = parsed.pathname;
    const headers = normalizedHeaders(init.headers);
    calls.push({ path, authorization: headers.authorization ?? "" });

    if (path === "/api/auth/login") {
      return jsonResponse(sessionPayload(label, 1));
    }
    if (path === "/api/auth/refresh") {
      refreshCount += 1;
      return jsonResponse(sessionPayload(label, refreshCount + 1));
    }
    if (path === "/api/auth/logout") {
      return jsonResponse({});
    }
    return jsonResponse(
      { error: { code: "NotFound", message: `unexpected ${path}` } },
      404
    );
  };
  return calls;
}

function sessionPayload(label, generation) {
  return {
    tenant_id: "tenant-1",
    user_id: "user-1",
    device_id: `${label}-device`,
    session_id: `${label}-session-1`,
    gateway_token: `${label}-gateway-token-${generation}`,
    refresh_token: `${label}-refresh-token-${generation}`,
    push_gateway_token: `${label}-push-token-${generation}`,
    gateway_expires_at_unix_ms: String(4_102_444_800_000),
    push_gateway_expires_at_unix_ms: String(4_102_444_800_000)
  };
}

function jsonResponse(payload, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    async text() {
      return JSON.stringify(payload);
    }
  };
}

function normalizedHeaders(headers) {
  if (!headers) {
    return {};
  }
  if (headers instanceof Headers) {
    return Object.fromEntries(
      Array.from(headers.entries()).map(([key, value]) => [key.toLowerCase(), value])
    );
  }
  return Object.fromEntries(
    Object.entries(headers).map(([key, value]) => [key.toLowerCase(), String(value)])
  );
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assertThrows(task, expectedMessage, message) {
  try {
    task();
  } catch (error) {
    if (error instanceof Error && error.message.includes(expectedMessage)) {
      return;
    }
    const actual = error instanceof Error ? error.message : String(error);
    throw new Error(`${message}: unexpected error ${actual}`);
  }
  throw new Error(`${message}: expected error`);
}

try {
  await main();
} finally {
  try {
    unlinkSync(entryPath);
  } catch {
    // The temp entry may not exist if startup failed before it was written.
  }
  rmSync(tempDir, { recursive: true, force: true });
}
