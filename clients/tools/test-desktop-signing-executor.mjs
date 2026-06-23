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
import { createTemporaryCodeSigningPfx, testPfxValue } from "./test-desktop-signing-fixtures.mjs";

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
        artifactKind: "desktop-executable",
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
  const pfxFixture = createTemporaryCodeSigningPfx(tempRoot);
  const fakePfx = pfxFixture.pfxPath;
  const readyPfxOptions = {
    certFile: fakePfx,
    pfxPassEnv: pfxFixture.pfxPassEnv,
    pfxPassEnvPresent: true,
    pfxPassEnvValue: testPfxValue,
    pfxCertificateProbe: pfxFixture.pfxCertificateProbe
  };
  const signingProfile = join(tempRoot, "desktop-signing-profile.json");
  writeFileSync(
    fakeSignTool,
    process.platform === "win32"
      ? "@echo off\r\necho signed> \"%~dp0signed.txt\"\r\necho %*> \"%~dp0signed-args.txt\"\r\nexit /b 0\r\n"
      : "#!/bin/sh\nprintf signed > \"$(dirname \"$0\")/signed.txt\"\nprintf '%s\\n' \"$*\" > \"$(dirname \"$0\")/signed-args.txt\"\n"
  );
  writeFileSync(signingProfile, `${JSON.stringify({
    schemaVersion: "nexusim.desktop-signing-profile.v1",
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      pfxPassEnv: pfxFixture.pfxPassEnv
    },
    signature: {
      expectedSignerSubjectContains: "NexusIM"
    }
  }, null, 2)}\n`);

  const missingPlan = buildDesktopSigningPlan({ manifest: manifestPath });
  const missingOutput = buildSigningOutput(missingPlan, { execute: false });
  assert(missingOutput.readyToSign === false, "missing signing output should not be ready");
  assert(missingOutput.readyToExecuteSigning === false, "missing signing output should not be executable");
  assert(missingOutput.executionPolicy.planOnly === true, "missing output should be plan-only");
  assert(missingOutput.executionPolicy.signsArtifacts === false, "missing output must not sign artifacts");

  const readyPlan = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    ...readyPfxOptions
  });
  const dryRun = buildSigningOutput(readyPlan, { execute: false });
  const dryRunJSON = JSON.stringify(dryRun);
  assert(dryRun.readyToSign === true, "dry-run output should preserve signing readiness");
  assert(dryRun.readyToExecuteSigning === true, "dry-run output should be executable when readiness is true");
  assert(dryRun.executionPolicy.planOnly === true, "default signing output should be plan-only");
  assert(dryRun.executionPolicy.executesSignCommand === false, "default signing output must not execute signtool");
  assert(dryRun.executionPolicy.requiresValidSignatureAfterSigning === false, "default signing output should not require post-signature verification");
  assert(dryRun.executionPolicy.verifiesSignatureAfterSigning === false, "default signing dry-run should not verify signatures");
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
    ...pfxFixture.env
  });
  assert(cliPlan.readyToSign === true, "CLI signing dry-run should be ready");
  assert(cliPlan.executionPolicy.executesSignCommand === false, "CLI dry-run should not execute signing");
  assert(!JSON.stringify(cliPlan).includes(tempRoot), "CLI signing dry-run leaked absolute temp path");

  const cliEnvExpectedSignerPlan = runSigner([
    "--manifest",
    manifestPath,
    "--signtool",
    fakeSignTool,
    "--cert-file",
    fakePfx,
    "--timestamp-url",
    "https://timestamp.example.test"
  ], {
    ...pfxFixture.env,
    NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT: "NexusIM"
  });
  assert(cliEnvExpectedSignerPlan.readyToSign === true, "env expected signer dry-run should be signing-ready");
  assert(
    cliEnvExpectedSignerPlan.executionPolicy.expectedSignerSubjectPolicyConfigured === true,
    "env expected signer policy should be declared"
  );
  assert(
    cliEnvExpectedSignerPlan.executionPolicy.requiresExpectedSignerSubjectAfterSigning === false,
    "env expected signer policy should only be enforced with --require-valid"
  );

  const cliProfilePlan = runSigner([
    "--manifest",
    manifestPath,
    "--signing-profile",
    signingProfile
  ], {
    ...pfxFixture.env
  });
  assert(cliProfilePlan.readyToSign === true, "CLI signing profile dry-run should be ready");
  assert(cliProfilePlan.executionPolicy.executesSignCommand === false, "CLI signing profile dry-run should not execute signing");
  assert(cliProfilePlan.executionPolicy.readsSigningProfile === true, "executor should declare profile reads");
  assert(cliProfilePlan.executionPolicy.expectedSignerSubjectPolicyConfigured === true, "executor should declare expected signer policy from profile");
  assert(cliProfilePlan.executionPolicy.requiresExpectedSignerSubjectAfterSigning === false, "executor should only enforce signer policy with --require-valid");
  assert(cliProfilePlan.signingPlan.signing.certificate.pfxPassEnv === pfxFixture.pfxPassEnv, "executor should preserve profile pfx pass env");
  assert(!JSON.stringify(cliProfilePlan).includes(tempRoot), "CLI signing profile dry-run leaked absolute temp path");

  const cliRequireValidPlan = runSigner([
    "--require-valid",
    "--manifest",
    manifestPath,
    "--signtool",
    fakeSignTool,
    "--cert-file",
    fakePfx,
    "--timestamp-url",
    "https://timestamp.example.test"
  ], {
    ...pfxFixture.env
  });
  assert(cliRequireValidPlan.readyToSign === true, "require-valid CLI dry-run should still be ready to sign");
  assert(cliRequireValidPlan.executionPolicy.requiresValidSignatureAfterSigning === true, "require-valid dry-run should declare valid-signature requirement");
  assert(cliRequireValidPlan.executionPolicy.verifiesSignatureAfterSigning === false, "require-valid dry-run must not verify before execute");

  const cliEnvExpectedSignerRequireValidPlan = runSigner([
    "--require-valid",
    "--manifest",
    manifestPath,
    "--signtool",
    fakeSignTool,
    "--cert-file",
    fakePfx,
    "--timestamp-url",
    "https://timestamp.example.test"
  ], {
    ...pfxFixture.env,
    NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT: "NexusIM"
  });
  assert(
    cliEnvExpectedSignerRequireValidPlan.executionPolicy.requiresExpectedSignerSubjectAfterSigning === true,
    "require-valid env expected signer dry-run should enforce signer policy after signing"
  );

  const cliProfileRequireValidPlan = runSigner([
    "--require-valid",
    "--manifest",
    manifestPath,
    "--signing-profile",
    signingProfile
  ], {
    ...pfxFixture.env
  });
  assert(cliProfileRequireValidPlan.readyToSign === true, "require-valid profile dry-run should still be ready to sign");
  assert(cliProfileRequireValidPlan.executionPolicy.readsSigningProfile === true, "require-valid profile dry-run should declare profile reads");
  assert(cliProfileRequireValidPlan.executionPolicy.requiresValidSignatureAfterSigning === true, "require-valid profile dry-run should require signature verification");
  assert(cliProfileRequireValidPlan.executionPolicy.expectedSignerSubjectPolicyConfigured === true, "require-valid profile dry-run should declare expected signer policy");
  assert(cliProfileRequireValidPlan.executionPolicy.requiresExpectedSignerSubjectAfterSigning === true, "require-valid profile dry-run should enforce signer policy after signing");

  const installer = "fake desktop installer bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop-installer.msi"), installer);
  const mixedManifest = {
    ...manifest,
    runId: "desktop-signing-executor-mixed",
    artifacts: [
      {
        target: "windows-desktop",
        artifactKind: "desktop-installer",
        filename: "nexusim-windows-desktop-installer.msi",
        bytes: Buffer.byteLength(installer),
        sha256: sha256(installer),
        sourcePathHash: sha256("desktop-installer-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      },
      ...manifest.artifacts
    ]
  };
  const mixedManifestPath = join(collectedDir, "manifest-mixed.json");
  writeFileSync(mixedManifestPath, `${JSON.stringify(mixedManifest, null, 2)}\n`);
  const mixedDefaultPlan = buildDesktopSigningPlan({
    manifest: mixedManifestPath,
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    ...readyPfxOptions
  });
  const mixedDefaultOutput = buildSigningOutput(mixedDefaultPlan, { execute: false });
  assert(mixedDefaultOutput.signingPlan.artifact.artifactKind === "desktop-executable", "default signing executor should expose executable artifact");
  const mixedInstallerPlan = runSigner([
    "--manifest",
    mixedManifestPath,
    "--artifact-kind",
    "desktop-installer",
    "--signtool",
    fakeSignTool,
    "--cert-file",
    fakePfx,
    "--timestamp-url",
    "https://timestamp.example.test"
  ], {
    ...pfxFixture.env
  });
  assert(mixedInstallerPlan.signingPlan.artifact.artifactKind === "desktop-installer", "explicit installer signing dry-run should expose installer artifact");

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
        ...pfxFixture.env
      }
    });
    assert(readyExecute.status === 0, `ready execute should run fake signing tool: ${readyExecute.stderr}`);
    assert(readFileSync(join(tempRoot, "signed.txt"), "utf8").trim() === "signed", "ready execute should invoke the signing tool");

    rmSync(join(tempRoot, "signed.txt"), { force: true });
    const readyExecuteProfile = spawnSync(process.execPath, [
      signer,
      "--execute",
      "--manifest",
      manifestPath,
      "--signing-profile",
      signingProfile
    ], {
      encoding: "utf8",
      env: {
        ...process.env,
        ...pfxFixture.env
      }
    });
    assert(readyExecuteProfile.status === 0, `profile execute should run fake signing tool: ${readyExecuteProfile.stderr}`);
    assert(readFileSync(join(tempRoot, "signed.txt"), "utf8").trim() === "signed", "profile execute should invoke the signing tool");

    rmSync(join(tempRoot, "signed.txt"), { force: true });
    rmSync(join(tempRoot, "signed-args.txt"), { force: true });
    const readyExecuteInstaller = spawnSync(process.execPath, [
      signer,
      "--execute",
      "--manifest",
      mixedManifestPath,
      "--artifact-kind",
      "desktop-installer",
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
        ...pfxFixture.env
      }
    });
    assert(readyExecuteInstaller.status === 0, `installer execute should run fake signing tool: ${readyExecuteInstaller.stderr}`);
    assert(readFileSync(join(tempRoot, "signed.txt"), "utf8").trim() === "signed", "installer execute should invoke the signing tool");
    const installerArgs = readFileSync(join(tempRoot, "signed-args.txt"), "utf8");
    assert(installerArgs.includes("nexusim-windows-desktop-installer.msi"), "installer execute should sign the collected installer artifact");
    assert(!installerArgs.includes("nexusim-windows-desktop.exe"), "installer execute must not sign the executable when artifact-kind is desktop-installer");

    rmSync(join(tempRoot, "signed.txt"), { force: true });
    const readyExecuteRequireValid = spawnSync(process.execPath, [
      signer,
      "--execute",
      "--require-valid",
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
        ...pfxFixture.env
      }
    });
    assert(readyExecuteRequireValid.status === 2, "require-valid execute should fail closed when fake signer leaves an invalid signature");
    assert(readFileSync(join(tempRoot, "signed.txt"), "utf8").trim() === "signed", "require-valid execute should still invoke signing before verification");
    assert(
      readyExecuteRequireValid.stderr.includes("desktop artifact signature is not valid after signing"),
      `require-valid execute should report invalid post-signature state: ${readyExecuteRequireValid.stderr}`
    );
  }
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signing executor ok");
