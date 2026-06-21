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
assert(report.targets.android.buildCommand.includes("build:android-apk:collect"), "android collect command missing");
assert(report.targets.android.dockerBuilder.profile === "client-builders", "android builder profile mismatch");
assert(report.targets.android.dockerBuilder.outputHint.endsWith("manifest.json"), "android builder manifest hint missing");
assert(Array.isArray(report.checks), "checks must be an array");
assert(!serialized.match(/token|secret|password|credential|private/i), "readiness report leaked sensitive names");
assert(!serialized.match(/[A-Z]:\\\\/), "readiness report leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "readiness report leaked extended Windows path");

console.log("client artifact readiness report ok");
