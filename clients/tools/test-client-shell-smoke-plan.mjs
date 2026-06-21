import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const planScript = join(toolsDir, "plan-client-shell-smoke.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const output = execFileSync(process.execPath, [planScript], {
  encoding: "utf8"
});
const plan = JSON.parse(output);
const serialized = JSON.stringify(plan);

assert(plan.schemaVersion === "nexusim.client-shell-smoke-plan.v1", "shell smoke plan schema mismatch");
assert(plan.targets.browser.readyForManualShellSmoke === true, "browser shell smoke should be available");
assert(plan.targets.browser.launchCommand.includes("dev:web"), "browser launch command missing");
assert(plan.targets["windows-desktop"].commands.prepareAssets.includes("build:shell-assets:desktop"), "desktop prep command missing");
assert(plan.targets["windows-desktop"].commands.verifyAssets.includes("windows-desktop"), "desktop verify command missing");
assert(plan.targets.android.commands.prepareAssets.includes("build:shell-assets:android"), "android prep command missing");
assert(plan.targets.android.commands.verifyAssets.includes("android"), "android verify command missing");
assert(Array.isArray(plan.targets.android.notes), "android notes missing");
assert(plan.sharedSmoke.backendCommand.includes("run-local-smoke.ps1"), "shared backend smoke command missing");
assert(plan.sharedSmoke.wiredLanExample.includes("172.31.50.1"), "wired LAN smoke example missing");
assert(!serialized.match(/token|secret|password|credential|private/i), "shell smoke plan leaked sensitive names");
assert(!serialized.match(/[A-Z]:\\\\/), "shell smoke plan leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "shell smoke plan leaked extended Windows path");

console.log("client shell smoke plan ok");
