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
      export { createAndroidClientRuntime } from "../android/src/runtime";
      export { createAndroidShellActions } from "../android/src/shell-actions";
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
    createDesktopShellActions,
    createAndroidClientRuntime,
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

  console.log("client runtime lifecycle ok");
}

async function exerciseRuntime(label, createRuntime, config, createShellActions) {
  const calls = installFetchStub(label);
  const runtime = createRuntime({ config });
  const session = await runtime.login({
    tenantID: "tenant-1",
    userID: "user-1",
    password: "password",
    deviceID: config.deviceID
  });
  assertEqual(session.accessToken, `${label}-gateway-token-1`, `${label} login returns gateway token`);
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
  const shellActions = createShellActions(restoredRuntime);
  const restoredState = await shellActions.restoreSession();
  assertEqual(restoredState.authenticated, true, `${label} shell restore reports authenticated`);
  assertEqual(restoredState.sessionID, `${label}-session-1`, `${label} shell restore returns session id`);
  assertEqual(
    restoredRuntime.auth.current()?.accessToken,
    `${label}-gateway-token-1`,
    `${label} restore hydrates auth manager`
  );

  const refreshed = await restoredRuntime.refresh();
  assertEqual(refreshed.accessToken, `${label}-gateway-token-2`, `${label} refresh returns new token`);
  assertEqual(
    (await runtime.platform.secureSessionStore.loadSession())?.refreshToken,
    `${label}-refresh-token-2`,
    `${label} refresh persists new refresh token`
  );

  const loggedOutState = await shellActions.logout();
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
