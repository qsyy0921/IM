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
assert(plan.downloadsToolchain === false, "no-toolchain plan must not download toolchains");
assert(plan.touchesDevice === false, "no-toolchain plan must not touch devices");
assert(plan.startsServices === false, "no-toolchain plan must not start services");
assert(commands.includes("npm --prefix clients run test:shell-smoke-plan"), "shell smoke plan check missing");
assert(commands.includes("npm --prefix clients run test:web-pwa"), "web PWA check missing");
assert(commands.includes("npm --prefix clients run test:shell-web-assets"), "shell asset check missing");
assert(commands.includes("npm --prefix clients run test:desktop-shell-action-assets"), "desktop action asset check missing");
assert(commands.includes("npm --prefix clients run test:android-shell-action-assets"), "android action asset check missing");
assert(commands.includes("npm --prefix clients run test:android-platform-readiness"), "android platform readiness schema check missing");
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
