import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname } from "node:path";
import { buildDesktopInstallerPlan } from "./plan-desktop-installer.mjs";
import { buildInstallerOutput } from "./build-desktop-installer.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const installerBuilder = join(toolsDir, "build-desktop-installer.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function writeJSON(path, value) {
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`, "utf8");
}

function runBuilder(args, env = {}) {
  const output = execFileSync(process.execPath, [installerBuilder, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      ...env
    }
  });
  return JSON.parse(output);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-installer-builder-"));
try {
  const collectedDir = join(tempRoot, "collected");
  mkdirSync(collectedDir, { recursive: true });
  const exe = "fake desktop executable bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exe);
  const manifest = {
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-23T00:00:00.000Z",
    gitCommit: "testcommit",
    runId: "desktop-installer-builder-test",
    artifacts: [
      {
        target: "windows-desktop",
        filename: "nexusim-windows-desktop.exe",
        bytes: Buffer.byteLength(exe),
        sha256: sha256(exe),
        sourcePathHash: sha256("desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/nexusim.exe"
      }
    ]
  };
  const manifestPath = join(collectedDir, "manifest.json");
  writeJSON(manifestPath, manifest);

  const activeConfigPath = join(tempRoot, "tauri-active.json");
  writeJSON(activeConfigPath, {
    productName: "NexusIM",
    version: "0.1.0",
    identifier: "com.nexusim.desktop",
    bundle: {
      active: true,
      targets: ["msi"],
      publisher: "NexusIM"
    }
  });
  const fakeSignTool = join(tempRoot, "signtool.exe");
  const fakePfx = join(tempRoot, "nexusim-signing.pfx");
  writeFileSync(fakeSignTool, "fake signtool");
  writeFileSync(fakePfx, "fake pfx");

  const readyPlan = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "msi",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true
  });
  const dryRun = buildInstallerOutput(readyPlan, { execute: false });
  const dryRunJSON = JSON.stringify(dryRun);
  assert(dryRun.readyToBuildInstaller === true, "dry-run output should preserve readiness");
  assert(dryRun.executionPolicy.planOnly === true, "default installer builder output should be plan-only");
  assert(dryRun.executionPolicy.executesBuildCommand === false, "default installer builder must not execute build");
  assert(dryRun.executionPolicy.executeRequested === false, "default installer builder should not request execution");
  assert(dryRun.executionPolicy.requiresExplicitExecuteFlag === true, "execute flag requirement missing");
  assert(!dryRunJSON.includes(tempRoot), "dry-run installer builder output leaked absolute temp path");
  assert(!dryRunJSON.match(/token|secret|password|credential|private/i), "dry-run installer builder output leaked sensitive names");

  const cliPlan = runBuilder([
    "--manifest",
    manifestPath,
    "--tauri-config",
    activeConfigPath,
    "--target",
    "msi",
    "--signtool",
    fakeSignTool,
    "--cert-file",
    fakePfx,
    "--timestamp-url",
    "https://timestamp.example.test"
  ], {
    NEXUSIM_DESKTOP_SIGN_PFX_PASS: "present"
  });
  assert(cliPlan.readyToBuildInstaller === true, "CLI dry-run should be ready");
  assert(cliPlan.executionPolicy.planOnly === true, "CLI default should be plan-only");
  assert(cliPlan.executionPolicy.executesBuildCommand === false, "CLI default should not execute build");
  assert(cliPlan.executionBlockers.includes("repository-tauri-config-required"), "CLI custom config dry-run should require repository config before execution");

  const notReady = spawnSync(process.execPath, [
    installerBuilder,
    "--execute",
    "--manifest",
    manifestPath,
    "--tauri-config",
    activeConfigPath
  ], {
    encoding: "utf8",
    env: {
      ...process.env
    }
  });
  assert(notReady.status === 2, "execute with missing signing config should fail closed");
  const notReadyOutput = JSON.parse(notReady.stdout);
  assert(notReadyOutput.executionPolicy.executeRequested === true, "execute attempt should report requested execute policy");
  assert(notReadyOutput.executionPolicy.executesBuildCommand === false, "not-ready execute must not run build command");
  assert(notReadyOutput.readyToBuildInstaller === false, "not-ready execute output should not be ready");
  assert(notReadyOutput.readyToExecuteInstallerBuild === false, "not-ready execute should not be executable");
  assert(notReadyOutput.missing.includes("desktop-signing-ready"), "not-ready execute should report signing readiness");

  const customConfigExecute = spawnSync(process.execPath, [
    installerBuilder,
    "--execute",
    "--manifest",
    manifestPath,
    "--tauri-config",
    activeConfigPath,
    "--target",
    "msi",
    "--signtool",
    fakeSignTool,
    "--cert-file",
    fakePfx,
    "--timestamp-url",
    "https://timestamp.example.test"
  ], {
    encoding: "utf8",
    env: {
      ...process.env,
      NEXUSIM_DESKTOP_SIGN_PFX_PASS: "present"
    }
  });
  assert(customConfigExecute.status === 2, "execute with custom tauri config should fail closed");
  const customConfigOutput = JSON.parse(customConfigExecute.stdout);
  assert(customConfigOutput.readyToBuildInstaller === true, "custom config plan should be build-ready");
  assert(customConfigOutput.readyToExecuteInstallerBuild === false, "custom config execute should be blocked");
  assert(customConfigOutput.executionPolicy.executesBuildCommand === false, "custom config execute must not run build command");
  assert(customConfigOutput.executionBlockers.includes("repository-tauri-config-required"), "custom config execute should report repository config blocker");

  assert(readFileSync(fakePfx, "utf8") === "fake pfx", "fixture pfx should still exist");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop installer builder ok");
