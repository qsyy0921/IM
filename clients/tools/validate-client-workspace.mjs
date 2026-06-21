import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const root = fileURLToPath(new URL("..", import.meta.url));

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const requiredPaths = [
  "package.json",
  "tsconfig.base.json",
  "packages/protocol/package.json",
  "packages/protocol/src/index.ts",
  "packages/client-core/package.json",
  "packages/client-core/src/index.ts",
  "packages/client-core/src/development-adapters.ts",
  "packages/client-core/src/key-value-message-store.ts",
  "packages/client-core/src/http-bff-client.ts",
  "packages/client-core/src/websocket-push-transport.ts",
  "packages/client-core/src/runtime.ts",
  "web/package.json",
  "web/.env.example",
  "web/src/App.tsx",
  "web/src/adapters/bff-client.ts",
  "web/src/adapters/browser-push-transport.ts",
  "tools/test-indexeddb-message-store.mjs",
  "tools/test-key-value-message-store.mjs",
  "tools/test-web-shell-lifecycle-contract.mjs",
  "tools/check-client-build-prereqs.mjs",
  "tools/client-build-env.mjs",
  "tools/build-desktop-artifact.mjs",
  "tools/build-android-apk.mjs",
  "tools/verify-shell-assets.mjs",
  "tools/plan-client-shell-smoke.mjs",
  "tools/test-client-artifact-builders.mjs",
  "tools/test-client-shell-smoke-plan.mjs",
  "desktop/package.json",
  "desktop/src/index.ts",
  "desktop/src/platform-contract.ts",
  "desktop/src/platform-adapter.ts",
  "desktop/src/persistent-message-store.ts",
  "desktop/src/runtime-config.ts",
  "desktop/src/runtime.ts",
  "desktop/src-tauri/Cargo.toml",
  "desktop/src-tauri/build.rs",
  "desktop/src-tauri/src/main.rs",
  "desktop/src-tauri/tauri.conf.json",
  "android/package.json",
  "android/src/index.ts",
  "android/src/platform-contract.ts",
  "android/src/platform-adapter.ts",
  "android/src/persistent-message-store.ts",
  "android/src/runtime-config.ts",
  "android/src/runtime.ts",
  "android/native/settings.gradle.kts",
  "android/native/app/build.gradle.kts",
  "android/native/app/src/main/AndroidManifest.xml",
  "android/native/app/src/main/java/com/nexusim/android/MainActivity.kt",
  "android/native/app/src/main/java/com/nexusim/android/NexusIMBridge.kt",
  "android/app.config.json"
];

for (const relativePath of requiredPaths) {
  assert(existsSync(join(root, relativePath)), `missing ${relativePath}`);
}

const workspacePackage = readJSON(join(root, "package.json"));
assert(workspacePackage.private === true, "clients workspace must be private");
assert(Array.isArray(workspacePackage.workspaces), "workspaces must be declared");
assert(workspacePackage.workspaces.includes("packages/*"), "packages/* workspace missing");
assert(workspacePackage.workspaces.includes("web"), "web workspace missing");
assert(workspacePackage.workspaces.includes("desktop"), "desktop workspace missing");
assert(workspacePackage.workspaces.includes("android"), "android workspace missing");

const protocolPackage = readJSON(join(root, "packages/protocol/package.json"));
const corePackage = readJSON(join(root, "packages/client-core/package.json"));
const webPackage = readJSON(join(root, "web/package.json"));
const desktopPackage = readJSON(join(root, "desktop/package.json"));
const androidPackage = readJSON(join(root, "android/package.json"));

assert(protocolPackage.name === "@nexusim/protocol", "protocol package name mismatch");
assert(corePackage.name === "@nexusim/client-core", "client-core package name mismatch");
assert(webPackage.dependencies["@nexusim/protocol"] === "0.1.0", "web must depend on protocol");
assert(webPackage.dependencies["@nexusim/client-core"] === "0.1.0", "web must depend on client-core");
assert(desktopPackage.name === "@nexusim/desktop", "desktop package name mismatch");
assert(androidPackage.name === "@nexusim/android", "android package name mismatch");
assert(desktopPackage.dependencies["@nexusim/protocol"] === "0.1.0", "desktop must depend on protocol");
assert(desktopPackage.dependencies["@nexusim/client-core"] === "0.1.0", "desktop must depend on client-core");
assert(androidPackage.dependencies["@nexusim/protocol"] === "0.1.0", "android must depend on protocol");
assert(androidPackage.dependencies["@nexusim/client-core"] === "0.1.0", "android must depend on client-core");

console.log("client workspace skeleton ok");
