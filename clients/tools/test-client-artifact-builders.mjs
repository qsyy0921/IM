import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const toolsDir = dirname(fileURLToPath(import.meta.url));

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function dryRun(scriptName) {
  const output = execFileSync(process.execPath, [join(toolsDir, scriptName), "--dry-run"], {
    encoding: "utf8"
  });
  return JSON.parse(output);
}

function dryRunCollect(scriptName) {
  const output = execFileSync(process.execPath, [
    join(toolsDir, scriptName),
    "--dry-run",
    "--collect",
    "--artifact-output-dir",
    "C:\\local\\should-not-leak",
    "--run-id",
    "builder-test"
  ], {
    encoding: "utf8"
  });
  return JSON.parse(output);
}

const desktop = dryRun("build-desktop-artifact.mjs");
assert(desktop.target === "windows-desktop", "desktop target mismatch");
assertBuildDryRunExecutionPolicy(desktop.executionPolicy, "desktop");
assert(desktop.outputHint.includes("clients/desktop"), "desktop output hint mismatch");
assert(Array.isArray(desktop.args) && desktop.args.includes("build"), "desktop build command missing build arg");
assert(desktop.skipShellAssetPrepEnv === "NEXUSIM_SKIP_SHELL_ASSET_PREP", "desktop wrapper skip env missing");
assert(!JSON.stringify(desktop).match(/token|secret|password|credential|private/i), "desktop build plan leaks sensitive names");
assert(desktop.collectArtifacts.enabled === false, "desktop collector should be disabled by default");
assert(desktop.forceFreshTauriAssets === false, "desktop default build should not force app clean");

const desktopCollect = dryRunCollect("build-desktop-artifact.mjs");
assertBuildDryRunExecutionPolicy(desktopCollect.executionPolicy, "desktop collect");
assert(desktopCollect.collectArtifacts.enabled === true, "desktop collector flag not reflected");
assert(desktopCollect.collectArtifacts.outputDir === "custom", "desktop collector custom output dir not reflected");
assert(desktopCollect.collectArtifacts.runId === "builder-test", "desktop collector run id not reflected");
assert(!JSON.stringify(desktopCollect).includes("C:\\local"), "desktop collect plan leaked absolute output dir");

const desktopCustomShell = JSON.parse(execFileSync(process.execPath, [
  join(toolsDir, "build-desktop-artifact.mjs"),
  "--dry-run",
  "--shell-config",
  "C:\\local\\shell-config.json"
], {
  encoding: "utf8"
}));
assert(desktopCustomShell.shellConfig === "custom", "desktop custom shell config not reflected");
assert(desktopCustomShell.forceFreshTauriAssets === true, "desktop custom shell config must force fresh Tauri asset embedding");
assert(!JSON.stringify(desktopCustomShell).includes("C:\\local"), "desktop custom shell plan leaked absolute config path");

const android = dryRun("build-android-apk.mjs");
assert(android.target === "android", "android target mismatch");
assertBuildDryRunExecutionPolicy(android.executionPolicy, "android");
assert(android.executionPolicy.startsActivity === false, "android dry-run should not start activities");
assert(android.outputHint.endsWith("app-debug.apk"), "android output hint mismatch");
assert(android.args.join(" ").includes("assembleDebug"), "android build command missing assembleDebug");
assert(android.args.includes("-Pnexusim.skipWebAssetPrep=true"), "android wrapper must skip duplicate Gradle asset prep after manifest verification");
assert(!JSON.stringify(android).match(/token|secret|password|credential|private/i), "android build plan leaks sensitive names");
assert(android.collectArtifacts.enabled === false, "android collector should be disabled by default");
assert(android.shellConfig === "default", "android default shell config marker missing");

const androidCollect = dryRunCollect("build-android-apk.mjs");
assertBuildDryRunExecutionPolicy(androidCollect.executionPolicy, "android collect");
assert(androidCollect.executionPolicy.startsActivity === false, "android collect dry-run should not start activities");
assert(androidCollect.collectArtifacts.enabled === true, "android collector flag not reflected");
assert(androidCollect.collectArtifacts.outputDir === "custom", "android collector custom output dir not reflected");
assert(androidCollect.collectArtifacts.runId === "builder-test", "android collector run id not reflected");
assert(!JSON.stringify(androidCollect).includes("C:\\local"), "android collect plan leaked absolute output dir");

const androidCustomShell = JSON.parse(execFileSync(process.execPath, [
  join(toolsDir, "build-android-apk.mjs"),
  "--dry-run",
  "--shell-config",
  "C:\\local\\android-shell-config.json"
], {
  encoding: "utf8"
}));
assert(androidCustomShell.shellConfig === "custom", "android custom shell config not reflected");
assert(!JSON.stringify(androidCustomShell).includes("C:\\local"), "android custom shell plan leaked absolute config path");

console.log("client artifact builders ok");

function assertBuildDryRunExecutionPolicy(policy, label) {
  assert(policy?.planOnly === true, `${label} dry-run should be marked plan-only`);
  assert(policy.executesBuildCommand === false, `${label} dry-run should not execute build command`);
  assert(policy.preparesShellAssets === false, `${label} dry-run should not prepare shell assets`);
  assert(policy.verifiesShellAssets === false, `${label} dry-run should not verify shell assets`);
  assert(policy.collectsArtifacts === false, `${label} dry-run should not collect artifacts`);
  assert(policy.writesBuildOutput === false, `${label} dry-run should not write build output`);
  assert(policy.startsDocker === false, `${label} dry-run should not start Docker`);
  assert(policy.installsArtifacts === false, `${label} dry-run should not install artifacts`);
  assert(policy.contactsDevice === false, `${label} dry-run should not contact devices`);
  assert(policy.downloadsToolchain === false, `${label} dry-run should not download toolchains`);
}
