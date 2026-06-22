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
assertDryRunPolicy(dryRun.executionPolicy);
assert(dryRun.safeDefaultNoImageBuild === true, "default must not allow image build");
assert(dryRun.allowImageBuild === false, "default allowImageBuild mismatch");
assert(dryRun.commands.buildImage.includes("client-android-apk-builder"), "build command missing service");
assert(dryRun.commands.runBuilder.includes("run --rm client-android-apk-builder"), "run command missing service");
assert(dryRun.outputHint.endsWith("manifest.json"), "manifest output hint missing");
assertDryRunPolicy(bootstrapDryRun.executionPolicy);
assert(bootstrapDryRun.allowImageBuild === true, "bootstrap dry-run should allow image build");
assert(bootstrapDryRun.safeDefaultNoImageBuild === false, "bootstrap safe default flag mismatch");
assert(bootstrapDryRun.executionPolicy.plannedDownloadsToolchain === !bootstrapDryRun.imagePresent, "bootstrap dry-run planned download flag mismatch");
assert(bootstrapDryRun.executionPolicy.downloadsToolchain === false, "bootstrap dry-run must not actually download toolchains");
assertDryRunPolicy(imageOnlyDryRun.executionPolicy);
assert(imageOnlyDryRun.buildImageOnly === true, "build-image-only flag mismatch");

const serialized = JSON.stringify([dryRun, bootstrapDryRun, imageOnlyDryRun]);
assert(!serialized.match(/token|secret|password|credential|private/i), "builder plan leaked sensitive names");
assert(!serialized.match(/[A-Z]:\\\\/), "builder plan leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "builder plan leaked extended Windows path");

console.log("android docker builder wrapper ok");

function assertDryRunPolicy(policy) {
  assert(policy?.planOnly === true, "builder dry-run should be plan-only");
  assert(policy.reportOnly === true, "builder dry-run should be report-only");
  assert(policy.readsDockerBuilderState === true, "builder dry-run should read Docker builder state");
  assert(policy.runsDockerCommands === false, "builder dry-run must not run Docker commands");
  assert(policy.startsDocker === false, "builder dry-run must not start Docker");
  assert(policy.buildsDockerImages === false, "builder dry-run must not build Docker images");
  assert(policy.startsBuilderContainer === false, "builder dry-run must not start builder containers");
  assert(policy.buildsAndroidApk === false, "builder dry-run must not build Android APKs");
  assert(policy.collectsArtifacts === false, "builder dry-run must not collect artifacts");
  assert(policy.writesArtifactManifest === false, "builder dry-run must not write artifact manifests");
  assert(policy.installsArtifacts === false, "builder dry-run must not install artifacts");
  assert(policy.contactsDevice === false, "builder dry-run must not contact devices");
  assert(policy.startsDeviceActivities === false, "builder dry-run must not start device activities");
  assert(policy.opensAdbReverse === false, "builder dry-run must not open adb reverse");
  assert(policy.opensAdbForward === false, "builder dry-run must not open adb forward");
  assert(policy.downloadsToolchain === false, "builder dry-run must not actually download toolchains");
  assert(policy.exposesLocalAbsolutePaths === false, "builder dry-run must not expose local absolute paths");
}
