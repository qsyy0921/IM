import { buildClientBuildPrereqsReport } from "./check-client-build-prereqs.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const report = buildClientBuildPrereqsReport({
  desktopArtifactReady: true,
  androidApkReady: false,
  checks: [
    {
      name: "local:tauri",
      target: "desktop",
      label: "local npm Tauri CLI",
      ok: true,
      command: "E:\\development\\IM\\clients\\node_modules\\.bin\\tauri.cmd --version",
      detail: "tauri-cli 2.11.3"
    },
    {
      name: "java>=17",
      target: "android",
      label: "JDK 17+",
      ok: false,
      command: "java -version",
      detail: "openjdk version \"1.8.0_482\"",
      detectedMajorVersion: 8
    },
    {
      name: "ANDROID_HOME",
      target: "android",
      label: "Android SDK",
      ok: false,
      command: "env:ANDROID_HOME",
      detail: "C:\\Android\\Sdk"
    }
  ]
});
const serialized = JSON.stringify(report);

assert(report.schemaVersion === "nexusim.client-build-prereqs.v1", "schema mismatch");
assertBuildPrereqsPolicy(report.executionPolicy);
assert(report.desktopArtifactReady === true, "desktop readiness mismatch");
assert(report.androidApkReady === false, "android readiness mismatch");
assert(report.checks.length === 3, "checks length mismatch");
assert(report.checks[0].name === "local:tauri", "check name mismatch");
assert(report.checks[1].detectedMajorVersion === 8, "detected Java major version missing");
assert(!serialized.includes("command"), "build prereqs report leaked command field");
assert(!serialized.includes("detail"), "build prereqs report leaked detail field");
assert(!serialized.includes("E:\\development"), "build prereqs report leaked local node bin path");
assert(!serialized.includes("C:\\Android"), "build prereqs report leaked Android SDK path");
assert(!serialized.match(/token|secret|password|credential|private/i), "build prereqs report leaked sensitive field name");

console.log("client build prereqs report ok");

function assertBuildPrereqsPolicy(policy) {
  assert(policy?.reportOnly === true, "build prereqs should be report-only");
  assert(policy.planOnly === false, "build prereqs is an actual local readiness probe");
  assert(policy.runsReadinessCommands === true, "build prereqs should run local readiness commands");
  assert(policy.readsLocalToolchainState === true, "build prereqs should read local toolchain state");
  assert(policy.readsEnvironmentVariables === true, "build prereqs should read environment variables");
  assert(policy.readsLocalNodeBinState === true, "build prereqs should read local node bin state");
  assert(policy.buildsNativeArtifacts === false, "build prereqs must not build artifacts");
  assert(policy.preparesShellAssets === false, "build prereqs must not prepare shell assets");
  assert(policy.startsServices === false, "build prereqs must not start services");
  assert(policy.startsDocker === false, "build prereqs must not start Docker");
  assert(policy.buildsDockerImages === false, "build prereqs must not build Docker images");
  assert(policy.installsArtifacts === false, "build prereqs must not install artifacts");
  assert(policy.contactsDevice === false, "build prereqs must not contact devices");
  assert(policy.startsDeviceActivities === false, "build prereqs must not start device activities");
  assert(policy.opensAdbReverse === false, "build prereqs must not open adb reverse");
  assert(policy.opensAdbForward === false, "build prereqs must not open adb forward");
  assert(policy.downloadsToolchain === false, "build prereqs must not download toolchains");
  assert(policy.exposesLocalAbsolutePaths === false, "build prereqs must not expose local absolute paths");
  assert(policy.exposesCommandOutput === false, "build prereqs must not expose command output");
}
