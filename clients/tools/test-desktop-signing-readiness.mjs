import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { buildDesktopSigningReadinessReport } from "./report-desktop-signing-readiness.mjs";
import { createTemporaryCodeSigningPfx, testPfxValue } from "./test-desktop-signing-fixtures.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const readinessReporter = join(toolsDir, "report-desktop-signing-readiness.mjs");

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

function runReporter(args, env = {}) {
  const output = execFileSync(process.execPath, [readinessReporter, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      ...env
    }
  });
  return JSON.parse(output);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-signing-readiness-"));
try {
  const collectedDir = join(tempRoot, "collected");
  mkdirSync(collectedDir, { recursive: true });
  const exe = "fake desktop executable bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exe);
  const manifest = {
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-23T00:00:00.000Z",
    gitCommit: "testcommit",
    runId: "desktop-signing-readiness-test",
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
  writeJSON(manifestPath, manifest);

  const activeConfigPath = join(tempRoot, "tauri-active.json");
  writeJSON(activeConfigPath, {
    productName: "NexusIM",
    version: "0.1.0",
    identifier: "com.nexusim.desktop",
    bundle: {
      active: true,
      targets: ["msi", "nsis"],
      publisher: "NexusIM"
    }
  });

  const fakeSignTool = join(tempRoot, "signtool.exe");
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
  writeFileSync(fakeSignTool, "fake signtool");
  writeJSON(signingProfile, {
    schemaVersion: "nexusim.desktop-signing-profile.v1",
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      pfxPassEnv: pfxFixture.pfxPassEnv
    }
  });

  const missing = buildDesktopSigningReadinessReport({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    signToolCandidatePaths: [fakeSignTool]
  });
  assert(missing.schemaVersion === "nexusim.desktop-signing-readiness.v1", "schema version mismatch");
  assert(missing.ready.canAttemptSigning === false, "missing signing inputs should not be signing-ready");
  assert(missing.ready.signatureValid === false, "unsigned fixture should not be signature-ready");
  assert(missing.ready.canBuildInstaller === false, "missing signing inputs should not be installer-ready");
  assert(missing.blockers.signing.includes("signtool-path"), "missing signtool should be reported");
  assert(missing.blockers.signing.includes("timestamp-url"), "missing timestamp should be reported");
  assert(missing.blockers.signing.includes("certificate-source"), "missing certificate source should be reported");
  assert(missing.localToolHints.signtool.candidateCount >= 1, "signtool candidate hint should be reported");
  assert(missing.localToolHints.signtool.candidatesUsedForReadiness === false, "signtool hints must not affect readiness");
  assert(missing.localToolHints.signtool.candidates.some(candidate => candidate.source === "explicit-candidate"), "explicit candidate source should be reported");
  assert(!JSON.stringify(missing.localToolHints).includes(tempRoot), "signtool hint leaked absolute temp path");
  assert(missing.executionPolicy.reportOnly === true, "readiness report should be report-only");
  assert(missing.executionPolicy.signsArtifacts === false, "readiness report must not sign artifacts");
  assert(missing.executionPolicy.buildsInstaller === false, "readiness report must not build installers");
  assert(missing.executionPolicy.installsArtifacts === false, "readiness report must not install artifacts");
  assert(missing.executionPolicy.downloadsToolchain === false, "readiness report must not download toolchains");

  const signingReadyUnsigned = buildDesktopSigningReadinessReport({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    ...readyPfxOptions,
    mockSignatureStatus: {
      status: "NotSigned"
    }
  });
  assert(signingReadyUnsigned.ready.canAttemptSigning === true, "complete signing inputs should be signing-ready");
  assert(signingReadyUnsigned.ready.signatureValid === false, "unsigned fixture should remain not ready");
  assert(signingReadyUnsigned.ready.canBuildInstaller === false, "installer should require valid signature");
  assert(signingReadyUnsigned.blockers.signature.includes("valid-authenticode-signature"), "valid signature blocker should be reported");
  assert(signingReadyUnsigned.blockers.installer.includes("desktop-signature-valid"), "installer should report signature blocker");
  assert(signingReadyUnsigned.signingExecution.executionPolicy.executeRequested === false, "report must not request signing execution");
  assert(signingReadyUnsigned.signingExecution.executionPolicy.executesSignCommand === false, "report must not execute signtool");

  const fullyReady = buildDesktopSigningReadinessReport({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "nsis",
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    ...readyPfxOptions,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(fullyReady.ready.canAttemptSigning === true, "valid fixture should be signing-ready");
  assert(fullyReady.ready.signatureValid === true, "valid fixture should be signature-ready");
  assert(fullyReady.ready.canBuildInstaller === true, "valid fixture should be installer-ready");
  assert(fullyReady.installer.target === "nsis", "installer target should be preserved");
  assert(Array.isArray(fullyReady.installer.commandTemplate?.build), "ready installer command template missing");
  assert(fullyReady.nextActions.length === 1, "ready report should contain a single next action");
  assert(fullyReady.nextActions[0].includes("installer build execute"), "ready report should point at installer execute path");

  const cliProfileReport = runReporter([
    "--manifest",
    manifestPath,
    "--tauri-config",
    activeConfigPath,
    "--signing-profile",
    signingProfile
  ], {
    ...pfxFixture.env
  });
  const cliJSON = JSON.stringify(cliProfileReport);
  assert(cliProfileReport.ready.canAttemptSigning === true, "CLI profile report should be signing-ready");
  assert(cliProfileReport.ready.signatureValid === false, "CLI profile report should still require real signature");
  assert(cliProfileReport.signing.mode === "pfx", "CLI profile report should use pfx mode");
  assert(!cliJSON.includes(tempRoot), "CLI readiness report leaked absolute temp path");
  assert(!cliJSON.match(/token|secret|password|credential|private/i), "CLI readiness report leaked sensitive names");
  assert(readFileSync(fakePfx).length > 0, "fixture pfx should still exist");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signing readiness report ok");
