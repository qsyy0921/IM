import { createHash } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname } from "node:path";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const collector = join(toolsDir, "collect-client-artifacts.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function runCollector(args) {
  const output = execFileSync(process.execPath, [collector, ...args], {
    encoding: "utf8"
  });
  return JSON.parse(output);
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-client-artifact-"));
try {
  const sourceDir = join(tempRoot, "source");
  const outputDir = join(tempRoot, "out");
  const fakeAPK = join(sourceDir, "fake-app-debug.apk");
  const fakeMSI = join(sourceDir, "fake-desktop.msi");
  const fakeEXE = join(sourceDir, "fake-desktop.exe");
  const apkBody = "fake apk bytes";
  const msiBody = "fake desktop installer bytes";
  const exeBody = "fake desktop executable bytes";

  mkdirSync(sourceDir, { recursive: true });
  writeFileSync(fakeAPK, apkBody);
  writeFileSync(fakeMSI, msiBody);
  writeFileSync(fakeEXE, exeBody);

  const dryRun = runCollector([
    "--target",
    "android",
    "--source",
    fakeAPK,
    "--output-dir",
    outputDir,
    "--run-id",
    "dry-run",
    "--dry-run"
  ]);
  assert(dryRun.dryRun === true, "dry-run flag not reflected");
  assertCollectorDryRunExecutionPolicy(dryRun.executionPolicy);
  assert(dryRun.sources.length === 1, "dry-run should find one source");
  assert(!existsSync(join(outputDir, "dry-run")), "dry-run must not write output");
  assert(!JSON.stringify(dryRun).includes(tempRoot), "dry-run leaked absolute temp path");

  const android = runCollector([
    "--target",
    "android",
    "--source",
    fakeAPK,
    "--output-dir",
    outputDir,
    "--run-id",
    "android-test"
  ]);
  const androidDir = join(outputDir, "android-test");
  const androidManifest = JSON.parse(readFileSync(join(androidDir, "manifest.json"), "utf8"));
  const androidCopy = readFileSync(join(androidDir, "nexusim-android-debug.apk"), "utf8");
  assert(android.artifacts.length === 1, "android collector result should include one artifact");
  assert(androidCopy === apkBody, "android artifact copy mismatch");
  assert(androidManifest.artifacts[0].sha256 === sha256(apkBody), "android artifact hash mismatch");
  assert(androidManifest.artifacts[0].filename === "nexusim-android-debug.apk", "android filename mismatch");
  assert(!JSON.stringify(androidManifest).includes(tempRoot), "android manifest leaked absolute temp path");

  const desktop = runCollector([
    "--target",
    "windows-desktop",
    "--source",
    fakeMSI,
    "--output-dir",
    outputDir,
    "--run-id",
    "desktop-test"
  ]);
  const desktopDir = join(outputDir, "desktop-test");
  const desktopManifest = JSON.parse(readFileSync(join(desktopDir, "manifest.json"), "utf8"));
  const desktopCopy = readFileSync(join(desktopDir, "nexusim-windows-desktop.msi"), "utf8");
  assert(desktop.artifacts.length === 1, "desktop collector result should include one artifact");
  assert(desktopCopy === msiBody, "desktop artifact copy mismatch");
  assert(desktopManifest.artifacts[0].sha256 === sha256(msiBody), "desktop artifact hash mismatch");
  assert(desktopManifest.artifacts[0].filename === "nexusim-windows-desktop.msi", "desktop filename mismatch");
  assert(Array.isArray(desktopManifest.supportFiles), "desktop manifest supportFiles missing");
  assert(desktopManifest.supportFiles.length === 1, "desktop msi package should include one support file");
  assert(desktopManifest.supportFiles[0].filename === "README-windows-desktop.txt", "desktop readme support file missing");
  assert(existsSync(join(desktopDir, "README-windows-desktop.txt")), "desktop readme file missing");
  assert(!JSON.stringify(desktopManifest).includes(tempRoot), "desktop manifest leaked absolute temp path");

  const desktopExe = runCollector([
    "--target",
    "windows-desktop",
    "--source",
    fakeEXE,
    "--output-dir",
    outputDir,
    "--run-id",
    "desktop-exe-test"
  ]);
  const desktopExeDir = join(outputDir, "desktop-exe-test");
  const desktopExeManifest = JSON.parse(readFileSync(join(desktopExeDir, "manifest.json"), "utf8"));
  const desktopExeCopy = readFileSync(join(desktopExeDir, "nexusim-windows-desktop.exe"), "utf8");
  const launcher = readFileSync(join(desktopExeDir, "launch-nexusim-windows.ps1"), "utf8");
  assert(desktopExe.artifacts.length === 1, "desktop exe collector result should include one artifact");
  assert(desktopExeCopy === exeBody, "desktop exe artifact copy mismatch");
  assert(desktopExeManifest.supportFiles.length === 2, "desktop exe package should include readme and launcher");
  assert(desktopExeManifest.supportFiles.some(file => file.filename === "README-windows-desktop.txt"), "desktop exe readme missing");
  assert(desktopExeManifest.supportFiles.some(file => file.filename === "launch-nexusim-windows.ps1"), "desktop exe launcher missing");
  assert(launcher.includes("$PSScriptRoot"), "desktop launcher should use package-relative path");
  assert(launcher.includes("nexusim-windows-desktop.exe"), "desktop launcher should start collected exe");
  assert(!launcher.includes(tempRoot), "desktop launcher leaked absolute temp path");
  assert(!JSON.stringify(desktopExeManifest).includes(tempRoot), "desktop exe manifest leaked absolute temp path");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("client artifact collector ok");

function assertCollectorDryRunExecutionPolicy(policy) {
  assert(policy?.planOnly === true, "collector dry-run should be marked plan-only");
  assert(policy.discoversArtifactSources === true, "collector dry-run should discover artifact sources");
  assert(policy.readsArtifactMetadata === true, "collector dry-run should only read artifact metadata");
  assert(policy.readsArtifactBytes === false, "collector dry-run should not read artifact bytes");
  assert(policy.copiesArtifacts === false, "collector dry-run should not copy artifacts");
  assert(policy.createsOutputDirectory === false, "collector dry-run should not create output directories");
  assert(policy.writesManifest === false, "collector dry-run should not write manifests");
  assert(policy.executesGit === false, "collector dry-run should not execute git");
  assert(policy.installsArtifacts === false, "collector dry-run should not install artifacts");
  assert(policy.contactsDevice === false, "collector dry-run should not contact devices");
  assert(policy.downloadsToolchain === false, "collector dry-run should not download toolchains");
}
