import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));
const repoRoot = dirname(clientsRoot);

const requiredPaths = [
  "deploy/docker/client-android-builder.Dockerfile",
  "deploy/local/docker-compose.client-builders.yml",
  "clients/tools/build-android-apk.mjs",
  "clients/tools/build-desktop-artifact.mjs"
];

for (const relativePath of requiredPaths) {
  assert(existsSync(join(repoRoot, relativePath)), `missing ${relativePath}`);
}

const dockerfile = read("deploy/docker/client-android-builder.Dockerfile");
assert(dockerfile.includes("eclipse-temurin:17-jdk"), "Android builder must use JDK 17+ base image");
assert(dockerfile.includes("ANDROID_HOME"), "Android builder must set ANDROID_HOME");
assert(dockerfile.includes("sdkmanager"), "Android builder must install SDK packages with sdkmanager");
assert(dockerfile.includes("android-${ANDROID_COMPILE_SDK}"), "Android builder must use compile SDK arg");
assert(dockerfile.includes("npm --prefix clients ci"), "Android builder must use reproducible npm ci");
assert(dockerfile.includes("npm --prefix clients run build:android-apk"), "Android builder must call APK build wrapper");

const compose = read("deploy/local/docker-compose.client-builders.yml");
assert(compose.includes("client-android-apk-builder"), "compose must define Android APK builder service");
assert(compose.includes("client-builders"), "compose must hide builder behind client-builders profile");
assert(compose.includes("deploy/docker/client-android-builder.Dockerfile"), "compose must reference Android builder Dockerfile");
assert(compose.includes("NEXUSIM_CLIENT_ARTIFACTS_DIR"), "compose must expose artifact output directory override");
assert(compose.includes("nexusim-debug.apk"), "compose must copy APK to a stable output filename");
assert(!compose.match(/token|secret|password|credential|private/i), "builder compose must not contain sensitive fields");
assert(!dockerfile.match(/token|secret|password|credential|private/i), "builder Dockerfile must not contain sensitive fields");

console.log("client builder profile ok");

function read(relativePath) {
  return readFileSync(join(repoRoot, relativePath), "utf8");
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}
