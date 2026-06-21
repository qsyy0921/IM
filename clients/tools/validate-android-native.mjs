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
  "tools/prepare-shell-web-assets.mjs",
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
assert(appBuild.includes("prepareNexusIMWebAssets"), "Android web asset prep task missing");
assert(appBuild.includes("build:shell-assets:android"), "Android build must prepare shell web assets");
assert(appBuild.includes('gradleProperty("nexusim.skipWebAssetPrep")'), "Android Gradle task must support wrapper skip property");
assert(appBuild.includes("onlyIf { !skipNexusIMWebAssetPrep.get() }"), "Android Gradle task must skip duplicate asset prep when wrapper already verified assets");
assert(appBuild.includes('workingDir = file("../../..")'), "Android web asset prep must run from clients workspace root");

const manifest = read("android/native/app/src/main/AndroidManifest.xml");
assert(manifest.includes("android.permission.INTERNET"), "Android INTERNET permission missing");
assert(manifest.includes("android.permission.ACCESS_NETWORK_STATE"), "Android network state permission missing");
assert(manifest.includes('android:name=".MainActivity"'), "Android MainActivity missing");

const mainActivity = read("android/native/app/src/main/java/com/nexusim/android/MainActivity.kt");
assert(mainActivity.includes("WebViewAssetLoader"), "Android shell must use WebViewAssetLoader");
assert(mainActivity.includes("appassets.androidplatform.net"), "Android shell must load appassets URL");
assert(mainActivity.includes("allowFileAccess = false"), "Android shell must disable file access");
assert(mainActivity.includes('addJavascriptInterface(NexusIMBridge(), "NexusIMNative")'), "Android shell must register low-permission native metadata bridge");
assert(mainActivity.includes("WebView.setWebContentsDebuggingEnabled"), "Android shell must explicitly gate WebView inspection");
assert(mainActivity.includes("ApplicationInfo.FLAG_DEBUGGABLE"), "Android WebView inspection must follow application debuggable flag");
assert(!/setWebContentsDebuggingEnabled\s*\(\s*true\s*\)/.test(mainActivity), "Android WebView inspection must not be enabled unconditionally");

const bridge = read("android/native/app/src/main/java/com/nexusim/android/NexusIMBridge.kt");
assert(bridge.includes("@JavascriptInterface"), "Android native bridge must expose only annotated methods");
assert(countMatches(bridge, /@JavascriptInterface/g) === 1, "Android native bridge must expose only runtimeMetadata");
assert(bridge.includes("fun runtimeMetadata(): String"), "Android runtime metadata method missing");
assert(bridge.includes('RUNTIME_TARGET: String = "android"'), "Android runtime target marker missing");
assert(bridge.includes("JSONObject()"), "Android native bridge must return structured metadata JSON");
assert(!bridge.includes("SharedPreferences"), "native bridge must not own session storage yet");
assert(!bridge.includes("SQLite"), "native bridge must not own message store yet");
assert(!bridge.match(/token|secret|password|credential|private/i), "native bridge must not contain sensitive fields");

const shellConfig = JSON.parse(read("android/shell-config.example.json"));
assert(shellConfig.target === "android", "Android shell config target mismatch");
assert(typeof shellConfig.apiBaseURL === "string", "Android shell config apiBaseURL missing");
assert(typeof shellConfig.pushWebSocketURL === "string", "Android shell config pushWebSocketURL missing");
assert(!JSON.stringify(shellConfig).match(/token|secret|password|credential|private/i), "Android shell config must not contain secrets");

const shellConfigPlaceholder = read("web/public/nexusim-shell-config.js");
assert(shellConfigPlaceholder.includes("__NEXUSIM_CLIENT_SHELL__"), "web shell config placeholder missing");

console.log("android native bridge skeleton ok");

function countMatches(text, pattern) {
  return Array.from(text.matchAll(pattern)).length;
}
