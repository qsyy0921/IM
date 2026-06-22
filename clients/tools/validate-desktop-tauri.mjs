import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join, resolve } from "node:path";

const root = fileURLToPath(new URL("..", import.meta.url));

function read(path) {
  return readFileSync(join(root, path), "utf8");
}

function readJSON(path) {
  return JSON.parse(read(path));
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const requiredPaths = [
  "desktop/src-tauri/Cargo.toml",
  "desktop/src-tauri/build.rs",
  "desktop/src-tauri/capabilities/default.json",
  "desktop/src-tauri/permissions/local_store.toml",
  "desktop/src-tauri/permissions/runtime_metadata.toml",
  "desktop/src-tauri/src/main.rs",
  "desktop/src-tauri/tauri.conf.json",
  "desktop/shell-config.example.json",
  "tools/prepare-shell-web-assets-if-needed.mjs",
  "tools/prepare-shell-web-assets.mjs",
  "web/public/nexusim-shell-config.js"
];

for (const relativePath of requiredPaths) {
  assert(existsSync(join(root, relativePath)), `missing ${relativePath}`);
}

const cargo = read("desktop/src-tauri/Cargo.toml");
assert(cargo.includes('name = "nexusim-desktop"'), "Cargo package name mismatch");
assert(cargo.includes('tauri = { version = "2"'), "Tauri v2 dependency missing");
assert(cargo.includes('rusqlite = { version = "0.32", features = ["bundled"] }'), "desktop SQLite key-value dependency missing");
assert(cargo.includes('tauri-build = { version = "2"'), "Tauri v2 build dependency missing");

const build = read("desktop/src-tauri/build.rs");
assert(build.includes("tauri_build::build()"), "Tauri build hook missing");

const main = read("desktop/src-tauri/src/main.rs");
assert(main.includes("tauri::Builder::default()"), "Tauri Builder entrypoint missing");
assert(main.includes("#[tauri::command]"), "desktop shell must expose audited Tauri commands");
assert(countMatches(main, /#\[tauri::command\]/g) === 4, "desktop shell must expose only runtime_metadata and fixed local_store commands");
assert(main.includes("fn runtime_metadata() -> String"), "desktop runtime metadata command missing");
assert(main.includes("fn local_store_get_item("), "desktop local_store_get_item command missing");
assert(main.includes("fn local_store_set_item("), "desktop local_store_set_item command missing");
assert(main.includes("fn local_store_remove_item("), "desktop local_store_remove_item command missing");
assert(main.includes("tauri::generate_handler!["), "desktop invoke handler missing");
assert(main.includes("runtime_metadata,"), "desktop invoke handler must register runtime_metadata");
assert(main.includes("local_store_get_item,"), "desktop invoke handler must register local_store_get_item");
assert(main.includes("local_store_set_item,"), "desktop invoke handler must register local_store_set_item");
assert(main.includes("local_store_remove_item"), "desktop invoke handler must register local_store_remove_item");
assert(main.includes('RUNTIME_TARGET: &str = "windows-desktop"'), "desktop runtime target marker missing");
assert(main.includes('NATIVE_BRIDGE_VERSION: &str = "0.2.0"'), "desktop native bridge version marker missing");
assert(main.includes('LOCAL_STORE_CURRENT: &str = "local-storage"'), "desktop local store current marker missing");
assert(main.includes('LOCAL_STORE_TARGET: &str = "sqlite"'), "desktop local store target marker missing");
assert(main.includes('NATIVE_STORE_READY: &str = "true"'), "desktop native store readiness marker missing");
assert(main.includes('NATIVE_STORE_REASON: &str = ""'), "desktop native store reason marker missing");
assert(main.includes('NATIVE_STORE_BRIDGE: &str = "tauri-sqlite"'), "desktop native store bridge marker missing");
assert(main.includes('LOCAL_STORE_KEY_PREFIX: &str = "nexusim:client-message-store:v1:"'), "desktop local store key prefix guard missing");
assert(main.includes("app_local_data_dir()"), "desktop local store must use app-local data directory");
assert(!main.match(/Command::|process::|token|secret|password|credential|message_id/i), "desktop native bridge must not expose sensitive or broad process capability");

const config = readJSON("desktop/src-tauri/tauri.conf.json");
assert(config.productName === "NexusIM", "desktop product name mismatch");
assert(config.identifier === "com.nexusim.desktop", "desktop identifier mismatch");
assert(config.build?.frontendDist === "../../web/dist", "desktop frontendDist mismatch");
assert(config.app?.withGlobalTauri === true, "desktop shell must expose the audited Tauri global bridge for WebView metadata smoke");
const frontendDist = resolve(root, "desktop", "src-tauri", config.build.frontendDist);
assert(frontendDist === resolve(root, "web", "dist"), "desktop frontendDist must resolve to shared web/dist");
assert(config.build?.beforeBuildCommand === "npm --prefix .. run prepare:shell-assets:desktop", "desktop build must use stable npm shell asset prep entrypoint");
assert(config.bundle?.active === false, "desktop bundle must stay inactive until artifact build slice");

const capability = readJSON("desktop/src-tauri/capabilities/default.json");
assert(capability.identifier === "main-shell-metadata", "desktop capability identifier mismatch");
assert(Array.isArray(capability.windows) && capability.windows.length === 1 && capability.windows[0] === "main", "desktop capability must target only the main window");
assert(Array.isArray(capability.permissions), "desktop capability permissions missing");
assert(capability.permissions.includes("core:default"), "desktop capability must include core default IPC permission set");
assert(capability.permissions.includes("allow-runtime-metadata"), "desktop capability must allow runtime_metadata app command");
assert(capability.permissions.includes("allow-local-store"), "desktop capability must allow fixed local-store commands");
assert(capability.permissions.length === 3, "desktop capability must not grant additional native commands");

const runtimeMetadataPermission = read("desktop/src-tauri/permissions/runtime_metadata.toml");
assert(runtimeMetadataPermission.includes('identifier = "allow-runtime-metadata"'), "desktop runtime metadata allow permission missing");
assert(runtimeMetadataPermission.includes('commands.allow = ["runtime_metadata"]'), "desktop runtime metadata command allow missing");
assert(runtimeMetadataPermission.includes('identifier = "deny-runtime-metadata"'), "desktop runtime metadata deny permission missing");
assert(!runtimeMetadataPermission.match(/File::|Command::|process::|token|secret|password|credential|message_id/i), "desktop runtime metadata permission must not mention broad native capability");

const localStorePermission = read("desktop/src-tauri/permissions/local_store.toml");
assert(localStorePermission.includes('identifier = "allow-local-store"'), "desktop local store allow permission missing");
assert(
  localStorePermission.includes('commands.allow = ["local_store_get_item", "local_store_set_item", "local_store_remove_item"]'),
  "desktop local store command allow missing"
);
assert(localStorePermission.includes('identifier = "deny-local-store"'), "desktop local store deny permission missing");
assert(!localStorePermission.match(/File::|Command::|process::|token|secret|password|credential|message_id/i), "desktop local store permission must not mention broad native capability");

const clientsPackage = readJSON("package.json");
assert(clientsPackage.scripts?.["prepare:shell-assets:desktop"]?.includes("prepare-shell-web-assets-if-needed.mjs"), "clients package must expose desktop shell asset prep entrypoint");
const desktopPackage = readJSON("desktop/package.json");
assert(desktopPackage.scripts?.["prepare:shell-assets:desktop"]?.includes("prepare-shell-web-assets-if-needed.mjs"), "desktop package must expose shell asset prep entrypoint");
assert(desktopPackage.scripts?.["tauri:build"] === "tauri build", "desktop tauri build script must rely on Tauri beforeBuildCommand");

const shellConfig = readJSON("desktop/shell-config.example.json");
assert(shellConfig.target === "windows-desktop", "desktop shell config target mismatch");
assert(typeof shellConfig.apiBaseURL === "string", "desktop shell config apiBaseURL missing");
assert(typeof shellConfig.pushWebSocketURL === "string", "desktop shell config pushWebSocketURL missing");
assert(!JSON.stringify(shellConfig).match(/token|secret|password|credential|private/i), "desktop shell config must not contain secrets");

const shellConfigPlaceholder = read("web/public/nexusim-shell-config.js");
assert(shellConfigPlaceholder.includes("__NEXUSIM_CLIENT_SHELL__"), "web shell config placeholder missing");

console.log("desktop tauri runner skeleton ok");

function countMatches(text, pattern) {
  return Array.from(text.matchAll(pattern)).length;
}
