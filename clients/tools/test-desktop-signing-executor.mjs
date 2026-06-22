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
import { execFileSync, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname } from "node:path";
import { buildDesktopSigningPlan } from "./plan-desktop-signing.mjs";
import { buildSigningOutput } from "./sign-desktop-artifact.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const signer = join(toolsDir, "sign-desktop-artifact.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function runSigner(args, env = {}) {
  const output = execFileSync(process.execPath, [signer, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      ...env
    }
  });
  return JSON.parse(output);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-signing-executor-"));
try {
  const collectedDir = join(tempRoot, "collected");
  mkdirSync(collectedDir, { recursive: true });
  const exe = "fake desktop executable bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exe);
  const manifest = {
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-23T00:00:00.000Z",
    gitCommit: "testcommit",
    runId: "desktop-signing-executor-test",
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
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  const fakeSignTool = join(tempRoot, process.platform === "win32" ? "signtool.cmd" : "signtool");
  const fakePfx = join(tempRoot, "nexusim-signing.pfx");
  writeFileSync(fakePfx, "fake pfx");
  writeFileSync(
    fakeSignTool,
    process.platform === "win32"
      ? "@echo off\r\necho signed> \"%~dp0signed.txt\"\r\nexit /b 0\r\n"
      : "#!/bin/sh\nprintf signed > \"$(dirname \"$0\")/signed.txt\"\n"
  );

  const missingPlan = buildDesktopSigningPlan({ manifest: manifestPath });
  const missingOutput = buildSigningOutput(missingPlan, { execute: false });
  assert(missingOutput.readyToSign === false, "missing signing output should not be ready");
  assert(missingOutput.readyToExecuteSigning === false, "missing signing output should not be executable");
  assert(missingOutput.executionPolicy.planOnly === true, "missing output should be plan-only");
  assert(missingOutput.executionPolicy.signsArtifacts === false, "missing output must not sign artifacts");

  const readyPlan = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true
  });
  const dryRun = buildSigningOutput(readyPlan, { execute: false });
  const dryRunJSON = JSON.stringify(dryRun);
  assert(dryRun.readyToSign === true, "dry-run output should preserve signing readiness");
  assert(dryRun.readyToExecuteSigning === true, "dry-run output should be executable when readiness is true");
  assert(dryRun.executionPolicy.planOnly === true, "default signing output should be plan-only");
  assert(dryRun.executionPolicy.executesSignCommand === false, "default signing output must not execute signtool");
  assert(dryRun.commandTemplate.includes("<signtool>"), "dry-run output should expose low-sensitive signtool placeholder");
  assert(!dryRunJSON.includes(tempRoot), "dry-run signing output leaked absolute temp path");
  assert(!dryRunJSON.match(/token|secret|password|credential|private/i), "dry-run signing output leaked sensitive names");
  assert(!existsSync(join(tempRoot, "signed.txt")), "dry-run should not run the fake signing tool");

  const cliPlan = runSigner([
    "--manifest",
    manifestPath,
    "--signtool",
    fakeSignTool,
    "--cert-file",
    fakePfx,
    "--timestamp-url",
    "https://timestamp.example.test"
  ], {
    NEXUSIM_DESKTOP_SIGN_PFX_PASS: "present"
  });
  assert(cliPlan.readyToSign === true, "CLI signing dry-run should be ready");
  assert(cliPlan.executionPolicy.executesSignCommand === false, "CLI dry-run should not execute signing");
  assert(!JSON.stringify(cliPlan).includes(tempRoot), "CLI signing dry-run leaked absolute temp path");

  const notReady = spawnSync(process.execPath, [
    signer,
    "--execute",
    "--manifest",
    manifestPath
  ], {
    encoding: "utf8",
    env: {
      ...process.env
    }
  });
  assert(notReady.status === 2, "execute without signing config should fail closed");
  const notReadyOutput = JSON.parse(notReady.stdout);
  assert(notReadyOutput.executionPolicy.executeRequested === true, "not-ready execute should report execute requested");
  assert(notReadyOutput.executionPolicy.executesSignCommand === false, "not-ready execute must not sign");
  assert(notReadyOutput.missing.includes("signtool-path"), "not-ready execute should report missing signtool");
  assert(!existsSync(join(tempRoot, "signed.txt")), "not-ready execute should not run the fake signing tool");

  if (process.platform === "win32") {
    const readyExecute = spawnSync(process.execPath, [
      signer,
      "--execute",
      "--manifest",
      manifestPath,
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
    assert(readyExecute.status === 0, `ready execute should run fake signing tool: ${readyExecute.stderr}`);
    assert(readFileSync(join(tempRoot, "signed.txt"), "utf8").trim() === "signed", "ready execute should invoke the signing tool");
  }
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signing executor ok");
