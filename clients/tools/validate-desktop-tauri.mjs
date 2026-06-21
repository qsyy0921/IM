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
  "desktop/src-tauri/src/main.rs",
  "desktop/src-tauri/tauri.conf.json",
  "desktop/shell-config.example.json",
  "tools/prepare-shell-web-assets.mjs",
  "web/public/nexusim-shell-config.js"
];

for (const relativePath of requiredPaths) {
  assert(existsSync(join(root, relativePath)), `missing ${relativePath}`);
}

const cargo = read("desktop/src-tauri/Cargo.toml");
assert(cargo.includes('name = "nexusim-desktop"'), "Cargo package name mismatch");
assert(cargo.includes('tauri = { version = "2"'), "Tauri v2 dependency missing");
assert(cargo.includes('tauri-build = { version = "2"'), "Tauri v2 build dependency missing");

const build = read("desktop/src-tauri/build.rs");
assert(build.includes("tauri_build::build()"), "Tauri build hook missing");

const main = read("desktop/src-tauri/src/main.rs");
assert(main.includes("tauri::Builder::default()"), "Tauri Builder entrypoint missing");
assert(main.includes("#[tauri::command]"), "desktop shell must expose only audited Tauri commands");
assert(countMatches(main, /#\[tauri::command\]/g) === 1, "desktop shell must expose only runtime_metadata");
assert(main.includes("fn runtime_metadata() -> String"), "desktop runtime metadata command missing");
assert(main.includes("tauri::generate_handler![runtime_metadata]"), "desktop invoke handler must only register runtime_metadata");
assert(main.includes('RUNTIME_TARGET: &str = "windows-desktop"'), "desktop runtime target marker missing");
assert(main.includes('NATIVE_BRIDGE_VERSION: &str = "0.1.0"'), "desktop native bridge version marker missing");
assert(!main.match(/std::fs|File::|Command::|process::|token|secret|password|credential|message_id/i), "desktop metadata bridge must not expose sensitive or broad native capability");

const config = readJSON("desktop/src-tauri/tauri.conf.json");
assert(config.productName === "NexusIM", "desktop product name mismatch");
assert(config.identifier === "com.nexusim.desktop", "desktop identifier mismatch");
assert(config.build?.frontendDist === "../../web/dist", "desktop frontendDist mismatch");
const frontendDist = resolve(root, "desktop", "src-tauri", config.build.frontendDist);
assert(frontendDist === resolve(root, "web", "dist"), "desktop frontendDist must resolve to shared web/dist");
assert(config.build?.beforeBuildCommand?.includes("build:shell-assets:desktop"), "desktop build must prepare shell web assets");
assert(config.bundle?.active === false, "desktop bundle must stay inactive until artifact build slice");

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
