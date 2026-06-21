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
assert(desktop.outputHint.includes("clients/desktop"), "desktop output hint mismatch");
assert(Array.isArray(desktop.args) && desktop.args.includes("build"), "desktop build command missing build arg");
assert(desktop.skipShellAssetPrepEnv === "NEXUSIM_SKIP_SHELL_ASSET_PREP", "desktop wrapper skip env missing");
assert(!JSON.stringify(desktop).match(/token|secret|password|credential|private/i), "desktop build plan leaks sensitive names");
assert(desktop.collectArtifacts.enabled === false, "desktop collector should be disabled by default");
assert(desktop.forceFreshTauriAssets === false, "desktop default build should not force app clean");

const desktopCollect = dryRunCollect("build-desktop-artifact.mjs");
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
assert(android.outputHint.endsWith("app-debug.apk"), "android output hint mismatch");
assert(android.args.join(" ").includes("assembleDebug"), "android build command missing assembleDebug");
assert(android.args.includes("-Pnexusim.skipWebAssetPrep=true"), "android wrapper must skip duplicate Gradle asset prep after manifest verification");
assert(!JSON.stringify(android).match(/token|secret|password|credential|private/i), "android build plan leaks sensitive names");
assert(android.collectArtifacts.enabled === false, "android collector should be disabled by default");
assert(android.shellConfig === "default", "android default shell config marker missing");

const androidCollect = dryRunCollect("build-android-apk.mjs");
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
