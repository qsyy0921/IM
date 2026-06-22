import { buildDryRunPlan } from "./check-client-no-toolchain.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const plan = buildDryRunPlan();
const serialized = JSON.stringify(plan);
const commands = plan.checks.map(check => check.command);

assert(plan.schemaVersion === "nexusim.client-no-toolchain-check.v1", "schema version mismatch");
assertDryRunExecutionPolicy(plan.executionPolicy);
assert(plan.downloadsToolchain === false, "no-toolchain plan must not download toolchains");
assert(plan.readsDeviceReadiness === true, "no-toolchain plan should report read-only device readiness");
assert(plan.installsArtifacts === false, "no-toolchain plan must not install artifacts");
assert(plan.startsDeviceActivities === false, "no-toolchain plan must not start device activities");
assert(plan.opensAdbReverse === false, "no-toolchain plan must not open adb reverse");
assert(plan.startsServices === false, "no-toolchain plan must not start services");
assert(commands.includes("npm --prefix clients run test:no-toolchain-check"), "no-toolchain self-check missing");
assert(commands.includes("npm --prefix clients run validate"), "client workspace validation missing");
assert(commands.includes("npm --prefix clients run test:shell-smoke-plan"), "shell smoke plan check missing");
assert(commands.includes("npm --prefix clients run test:build-prereqs"), "build prereqs report contract check missing");
assert(commands.includes("npm --prefix clients run test:artifact-readiness"), "artifact readiness contract check missing");
assert(commands.includes("npm --prefix clients run test:artifact-install-plan"), "artifact install plan contract check missing");
assert(commands.includes("npm --prefix clients run test:artifact-builders"), "artifact builder contract check missing");
assert(commands.includes("npm --prefix clients run test:artifact-collector"), "artifact collector contract check missing");
assert(commands.includes("npm --prefix clients run validate:builder-profile"), "client builder profile contract check missing");
assert(commands.includes("npm --prefix clients run test:android-docker-builder"), "android builder wrapper contract check missing");
assert(commands.includes("npm --prefix clients run test:clientweb-smoke-hooks"), "clientweb smoke hook check missing");
assert(commands.includes("npm --prefix clients run test:web-pwa"), "web PWA check missing");
assert(commands.includes("npm --prefix clients run test:shell-config"), "shell config contract check missing");
assert(commands.includes("npm --prefix clients run typecheck"), "client workspace TypeScript contract check missing");
assert(commands.includes("npm --prefix clients run validate:desktop-tauri"), "desktop native skeleton contract check missing");
assert(commands.includes("npm --prefix clients run validate:android-native"), "android native skeleton contract check missing");
assert(commands.includes("npm --prefix clients run test:web-platform"), "web platform contract check missing");
assert(commands.includes("npm --prefix clients run test:local-message-store"), "local message store contract check missing");
assert(commands.includes("npm --prefix clients run test:indexeddb-store"), "indexeddb message store contract check missing");
assert(commands.includes("npm --prefix clients run test:key-value-store"), "key-value message store contract check missing");
assert(commands.includes("npm --prefix clients run test:http-bff-client"), "http bff client contract check missing");
assert(commands.includes("npm --prefix clients run test:native-store-readiness"), "native store readiness contract check missing");
assert(commands.includes("npm --prefix clients run test:runtime-lifecycle"), "runtime lifecycle contract check missing");
assert(commands.includes("npm --prefix clients run test:web-shell-actions"), "web shell lifecycle contract check missing");
assert(commands.includes("npm --prefix clients run test:web-shell-automation"), "web shell automation contract check missing");
assert(commands.includes("npm --prefix clients run test:web-shell-smoke-report"), "web shell smoke report contract check missing");
assert(commands.includes("npm --prefix clients run test:shell-web-assets"), "shell asset check missing");
assert(commands.includes("npm --prefix clients run test:shell-asset-prep-wrapper"), "shell asset prep wrapper check missing");
assert(commands.includes("npm --prefix clients run test:desktop-shell-action-assets"), "desktop action asset check missing");
assert(commands.includes("npm --prefix clients run test:desktop-artifact-launch-smoke"), "desktop artifact launch smoke contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-bundle"), "desktop bundle contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-installer-plan"), "desktop installer plan contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-installer-builder"), "desktop installer builder contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-signing-plan"), "desktop signing plan contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-signing-executor"), "desktop signing executor contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-signature-verifier"), "desktop signature verifier contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-composed-smoke"), "desktop composed smoke contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-webview-metadata-smoke"), "desktop WebView metadata runner contract check missing");
assert(commands.includes("npm --prefix clients run test:desktop-webview-login-smoke"), "desktop WebView login runner contract check missing");
assert(commands.includes("npm --prefix clients run test:android-shell-action-assets"), "android action asset check missing");
assert(commands.includes("npm --prefix clients run test:android-webview-metadata-smoke"), "android WebView metadata runner contract check missing");
assert(commands.includes("npm --prefix clients run test:android-webview-login-smoke-plan"), "android WebView login plan check missing");
assert(commands.includes("npm --prefix clients run test:android-webview-login-smoke"), "android WebView login runner contract check missing");
assert(commands.includes("npm --prefix clients run test:android-platform-readiness"), "android platform readiness schema check missing");
assert(commands.includes("npm --prefix clients run test:android-device-readiness"), "android device readiness parser check missing");
assert(commands.includes("npm --prefix clients run test:android-webview-devtools-readiness"), "android WebView devtools readiness parser check missing");
assert(commands.includes("npm --prefix clients run test:android-webview-devtools-parser"), "android WebView devtools socket parser check missing");
assert(commands.includes("npm --prefix clients run report:android-platform-readiness"), "android platform readiness report missing");

for (const forbidden of [
  "build:android-apk",
  "build:android-apk:docker",
  "bootstrap",
  "docker build",
  "gradle",
  "assembleDebug",
  "adb install",
  "adb reverse",
  "smoke:android-webview"
]) {
  assert(!serialized.includes(forbidden), `no-toolchain plan contains forbidden operation ${forbidden}`);
}

assert(!serialized.match(/[A-Z]:\\\\/), "no-toolchain plan leaked a Windows absolute path");
assert(!serialized.includes("\\\\?"), "no-toolchain plan leaked an extended Windows path");
assert(!serialized.match(/token|secret|password|credential|private/i), "no-toolchain plan leaked sensitive names");

console.log("client no-toolchain check plan ok");

function assertDryRunExecutionPolicy(policy) {
  assert(policy?.planOnly === true, "no-toolchain dry-run should be marked plan-only");
  assert(policy.describesFocusedGate === true, "no-toolchain dry-run should describe the focused gate");
  assert(policy.executesChecks === false, "no-toolchain dry-run should not execute checks");
  assert(policy.runsNpmScripts === false, "no-toolchain dry-run should not run npm scripts");
  assert(policy.readsDeviceReadiness === false, "no-toolchain dry-run should not read device readiness");
  assert(policy.installsArtifacts === false, "no-toolchain dry-run should not install artifacts");
  assert(policy.startsDeviceActivities === false, "no-toolchain dry-run should not start device activities");
  assert(policy.opensAdbReverse === false, "no-toolchain dry-run should not open adb reverse");
  assert(policy.startsServices === false, "no-toolchain dry-run should not start services");
  assert(policy.startsDocker === false, "no-toolchain dry-run should not start Docker");
  assert(policy.downloadsToolchain === false, "no-toolchain dry-run should not download toolchains");
}
