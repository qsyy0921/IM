import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const reportScript = join(toolsDir, "report-client-artifact-readiness.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const output = execFileSync(process.execPath, [reportScript], {
  encoding: "utf8"
});
const report = JSON.parse(output);
const serialized = JSON.stringify(report);

assert(report.schemaVersion === "nexusim.client-artifact-readiness.v1", "schema version mismatch");
assert(report.targets["windows-desktop"].buildCommand.includes("build:desktop-artifact:collect"), "desktop collect command missing");
assert(typeof report.targets["windows-desktop"].shellAssets?.verified === "boolean", "desktop shell asset status missing");
if (report.targets["windows-desktop"].ready) {
  assert(
    !report.targets["windows-desktop"].missing.some(item => item.name === "cargo tauri" || item.name === "local:tauri"),
    "desktop missing list should not include a satisfied alternative Tauri CLI"
  );
}
assert(report.targets.android.buildCommand.includes("build:android-apk:collect"), "android collect command missing");
assert(typeof report.targets.android.shellAssets?.verified === "boolean", "android shell asset status missing");
assert(report.targets.android.dockerBuilder.profile === "client-builders", "android builder profile mismatch");
assert(report.targets.android.dockerBuilder.outputHint.endsWith("manifest.json"), "android builder manifest hint missing");
assert(
  report.targets.android.dockerBuilder.imageBuildCommand.includes("build:android-apk:docker:bootstrap"),
  "android builder image build command missing"
);
assert(
  report.targets.android.dockerBuilder.buildCommand.includes("build:android-apk:docker"),
  "android builder run command missing"
);
assert(
  report.targets.android.dockerBuilder.safeDryRunCommand.includes("run-android-docker-builder.mjs --dry-run"),
  "android builder safe dry-run command missing"
);
assert(Array.isArray(report.checks), "checks must be an array");
assert(Array.isArray(report.nextActions), "nextActions must be an array");
assert(
  report.nextActions.some(action => action.target === "windows-desktop" && typeof action.command === "string"),
  "desktop next action command missing"
);
assert(
  report.nextActions.some(action => action.target === "android" && typeof action.command === "string"),
  "android next action command missing"
);
assert(!serialized.match(/token|secret|password|credential|private/i), "readiness report leaked sensitive names");
assert(!serialized.match(/[A-Z]:\\\\/), "readiness report leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "readiness report leaked extended Windows path");

console.log("client artifact readiness report ok");
