import { mkdtempSync, rmSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import * as esbuild from "esbuild";

const tempDir = mkdtempSync(join(tmpdir(), "nexusim-native-store-readiness-"));
const bundlePath = join(tempDir, "native-store-readiness-entry.mjs");
const toolsDir = fileURLToPath(new URL(".", import.meta.url));
const entryPath = join(toolsDir, `.native-store-readiness-entry-${process.pid}.ts`);

async function main() {
  writeFileSync(
    entryPath,
    `
      export {
        NativeStoreUnavailableError,
        assertNativeStoreReady,
        nativeStoreReadiness
      } from "../packages/client-core/src/native-store-readiness";
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
    NativeStoreUnavailableError,
    assertNativeStoreReady,
    nativeStoreReadiness
  } = await import(pathToFileURL(bundlePath).href);

  assertDeepEqual(
    nativeStoreReadiness({
      target: "windows-desktop",
      requestedStore: "local-storage"
    }),
    {
      target: "windows-desktop",
      requestedStore: "local-storage",
      ready: true,
      reason: "",
      bridge: "none",
      nextAction: ""
    },
    "local-storage does not need native sqlite bridge"
  );

  const desktop = nativeStoreReadiness({
    target: "windows-desktop",
    requestedStore: "sqlite"
  });
  assertEqual(desktop.ready, false, "desktop sqlite is not ready without bridge");
  assertEqual(desktop.reason, "sqlite-native-bridge-unavailable", "desktop sqlite reason");
  assertEqual(desktop.bridge, "tauri-sqlite", "desktop sqlite bridge");
  assertContains(desktop.nextAction, "tauri-sqlite", "desktop sqlite next action");

  const android = nativeStoreReadiness({
    target: "android",
    requestedStore: "sqlite"
  });
  assertEqual(android.ready, false, "android sqlite is not ready without bridge");
  assertEqual(android.reason, "sqlite-native-bridge-unavailable", "android sqlite reason");
  assertEqual(android.bridge, "android-sqlite", "android sqlite bridge");
  assertContains(android.nextAction, "android-sqlite", "android sqlite next action");

  const readyAndroid = assertNativeStoreReady({
    target: "android",
    requestedStore: "sqlite",
    nativeBridgeAvailable: true
  });
  assertEqual(readyAndroid.ready, true, "android sqlite can be marked ready by native bridge");
  assertEqual(readyAndroid.reason, "", "ready android sqlite has no failure reason");

  assertThrows(
    () =>
      assertNativeStoreReady({
        target: "windows-desktop",
        requestedStore: "sqlite"
      }),
    NativeStoreUnavailableError,
    "reason=sqlite-native-bridge-unavailable",
    "missing desktop sqlite bridge throws typed error"
  );

  console.log("native store readiness ok");
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assertContains(actual, expectedFragment, message) {
  if (!String(actual).includes(expectedFragment)) {
    throw new Error(`${message}: expected ${String(actual)} to contain ${expectedFragment}`);
  }
}

function assertDeepEqual(actual, expected, message) {
  const actualJSON = JSON.stringify(actual);
  const expectedJSON = JSON.stringify(expected);
  if (actualJSON !== expectedJSON) {
    throw new Error(`${message}: expected ${expectedJSON}, got ${actualJSON}`);
  }
}

function assertThrows(task, expectedConstructor, expectedMessageFragment, message) {
  try {
    task();
  } catch (error) {
    if (!(error instanceof expectedConstructor)) {
      const actualName = error instanceof Error ? error.name : typeof error;
      throw new Error(`${message}: unexpected error type ${actualName}`);
    }
    if (!error.message.includes(expectedMessageFragment)) {
      throw new Error(`${message}: unexpected error message ${error.message}`);
    }
    return;
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
