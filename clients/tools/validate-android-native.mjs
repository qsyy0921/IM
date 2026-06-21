import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const root = fileURLToPath(new URL("..", import.meta.url));

function read(path) {
  return readFileSync(join(root, path), "utf8");
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const requiredPaths = [
  "android/native/settings.gradle.kts",
  "android/native/build.gradle.kts",
  "android/native/app/build.gradle.kts",
  "android/native/app/src/main/AndroidManifest.xml",
  "android/native/app/src/main/java/com/nexusim/android/MainActivity.kt",
  "android/native/app/src/main/java/com/nexusim/android/NexusIMBridge.kt",
  "android/shell-config.example.json",
  "web/public/nexusim-shell-config.js"
];

for (const relativePath of requiredPaths) {
  assert(existsSync(join(root, relativePath)), `missing ${relativePath}`);
}

const settings = read("android/native/settings.gradle.kts");
assert(settings.includes('rootProject.name = "NexusIMAndroid"'), "Android root project name mismatch");
assert(settings.includes('include(":app")'), "Android app module missing");

const rootBuild = read("android/native/build.gradle.kts");
assert(rootBuild.includes('id("com.android.application")'), "Android application plugin missing");
assert(rootBuild.includes('id("org.jetbrains.kotlin.android")'), "Kotlin Android plugin missing");

const appBuild = read("android/native/app/build.gradle.kts");
assert(appBuild.includes('namespace = "com.nexusim.android"'), "Android namespace mismatch");
assert(appBuild.includes('applicationId = "com.nexusim.android"'), "Android applicationId mismatch");
assert(appBuild.includes("minSdk = 26"), "Android minSdk mismatch");
assert(appBuild.includes("targetSdk = 35"), "Android targetSdk mismatch");

const manifest = read("android/native/app/src/main/AndroidManifest.xml");
assert(manifest.includes("android.permission.INTERNET"), "Android INTERNET permission missing");
assert(manifest.includes("android.permission.ACCESS_NETWORK_STATE"), "Android network state permission missing");
assert(manifest.includes('android:name=".MainActivity"'), "Android MainActivity missing");

const bridge = read("android/native/app/src/main/java/com/nexusim/android/NexusIMBridge.kt");
assert(bridge.includes('runtimeTarget: String = "android"'), "Android runtime target marker missing");
assert(!bridge.includes("SharedPreferences"), "native bridge must not own session storage yet");
assert(!bridge.includes("SQLite"), "native bridge must not own message store yet");

const shellConfig = JSON.parse(read("android/shell-config.example.json"));
assert(shellConfig.target === "android", "Android shell config target mismatch");
assert(typeof shellConfig.apiBaseURL === "string", "Android shell config apiBaseURL missing");
assert(typeof shellConfig.pushWebSocketURL === "string", "Android shell config pushWebSocketURL missing");
assert(!JSON.stringify(shellConfig).match(/token|secret|password|credential|private/i), "Android shell config must not contain secrets");

const shellConfigPlaceholder = read("web/public/nexusim-shell-config.js");
assert(shellConfigPlaceholder.includes("__NEXUSIM_CLIENT_SHELL__"), "web shell config placeholder missing");

console.log("android native bridge skeleton ok");
