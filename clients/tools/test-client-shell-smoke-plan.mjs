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
assert(Array.isArray(plan.targets.browser.checklist) && plan.targets.browser.checklist.length >= 3, "browser checklist missing");
assert(plan.targets.browser.checklist.some(item => item.step === "verify-client-flow"), "browser flow verification missing");
assert(plan.targets["windows-desktop"].commands.prepareAssets.includes("build:shell-assets:desktop"), "desktop prep command missing");
assert(plan.targets["windows-desktop"].commands.verifyAssets.includes("windows-desktop"), "desktop verify command missing");
assert(plan.targets["windows-desktop"].commands.installPlan.includes("plan:artifact-install"), "desktop install plan command missing");
assert(plan.targets["windows-desktop"].install, "desktop install status missing");
assert(typeof plan.targets["windows-desktop"].install.readyForInstall === "boolean", "desktop install readiness missing");
assert(Array.isArray(plan.targets["windows-desktop"].install.missing), "desktop install missing list missing");
assert(Array.isArray(plan.targets["windows-desktop"].checklist), "desktop checklist missing");
assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "prepare-shell-assets"), "desktop asset prep checklist missing");
assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "verify-shell-assets"), "desktop asset verification checklist missing");
assert(plan.targets.android.commands.prepareAssets.includes("build:shell-assets:android"), "android prep command missing");
assert(plan.targets.android.commands.verifyAssets.includes("android"), "android verify command missing");
assert(plan.targets.android.commands.installPlan.includes("plan:artifact-install"), "android install plan command missing");
assert(plan.targets.android.install, "android install status missing");
assert(typeof plan.targets.android.install.readyForInstall === "boolean", "android install readiness missing");
assert(typeof plan.targets.android.install.installPrereqs.adbAvailable === "boolean", "android adb prereq status missing");
assert(Array.isArray(plan.targets.android.install.missing), "android install missing list missing");
assert(Array.isArray(plan.targets.android.checklist), "android checklist missing");
assert(plan.targets.android.checklist.some(item => item.step === "prepare-shell-assets"), "android asset prep checklist missing");
assert(plan.targets.android.checklist.some(item => item.step === "verify-shell-assets"), "android asset verification checklist missing");
assert(Array.isArray(plan.targets.android.notes), "android notes missing");
assert(plan.sharedSmoke.backendCommand.includes("run-local-smoke.ps1"), "shared backend smoke command missing");
assert(plan.sharedSmoke.wiredLanExample.includes("172.31.50.1"), "wired LAN smoke example missing");
assert(!serialized.match(/token|secret|password|credential|private/i), "shell smoke plan leaked sensitive names");
assert(!serialized.match(/[A-Z]:\\\\/), "shell smoke plan leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "shell smoke plan leaked extended Windows path");

console.log("client shell smoke plan ok");
