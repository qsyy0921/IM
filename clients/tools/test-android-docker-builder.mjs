import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const script = join(toolsDir, "run-android-docker-builder.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const dryRun = JSON.parse(execFileSync(process.execPath, [script, "--dry-run"], {
  encoding: "utf8"
}));
const bootstrapDryRun = JSON.parse(execFileSync(process.execPath, [script, "--dry-run", "--allow-image-build"], {
  encoding: "utf8"
}));
const imageOnlyDryRun = JSON.parse(execFileSync(process.execPath, [script, "--dry-run", "--build-image-only"], {
  encoding: "utf8"
}));

assert(dryRun.schemaVersion === "nexusim.android-docker-builder-plan.v1", "schema mismatch");
assert(dryRun.safeDefaultNoImageBuild === true, "default must not allow image build");
assert(dryRun.allowImageBuild === false, "default allowImageBuild mismatch");
assert(dryRun.commands.buildImage.includes("client-android-apk-builder"), "build command missing service");
assert(dryRun.commands.runBuilder.includes("run --rm client-android-apk-builder"), "run command missing service");
assert(dryRun.outputHint.endsWith("manifest.json"), "manifest output hint missing");
assert(bootstrapDryRun.allowImageBuild === true, "bootstrap dry-run should allow image build");
assert(bootstrapDryRun.safeDefaultNoImageBuild === false, "bootstrap safe default flag mismatch");
assert(imageOnlyDryRun.buildImageOnly === true, "build-image-only flag mismatch");

const serialized = JSON.stringify([dryRun, bootstrapDryRun, imageOnlyDryRun]);
assert(!serialized.match(/token|secret|password|credential|private/i), "builder plan leaked sensitive names");
assert(!serialized.match(/[A-Z]:\\\\/), "builder plan leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "builder plan leaked extended Windows path");

console.log("android docker builder wrapper ok");

