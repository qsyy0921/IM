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
  const installer = "fake desktop installer bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exe);
  writeFileSync(join(collectedDir, "nexusim-windows-desktop-installer.msi"), installer);
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
  const installerManifest = {
    ...manifest,
    runId: "desktop-signing-readiness-installer-test",
    artifacts: [
      ...manifest.artifacts,
      {
        target: "windows-desktop",
        artifactKind: "desktop-installer",
        filename: "nexusim-windows-desktop-installer.msi",
        bytes: Buffer.byteLength(installer),
        sha256: sha256(installer),
        sourcePathHash: sha256("desktop-installer-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      }
    ]
  };
  const installerManifestPath = join(collectedDir, "manifest-with-installer.json");
  writeJSON(installerManifestPath, installerManifest);

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
    signature: {
      expectedSignerSubjectContains: "NexusIM"
    },
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
  assert(missing.ready.signedInstallerValid === false, "missing installer artifact should not be installer-signature-ready");
  assert(missing.blockers.signedInstaller.includes("desktop-installer-artifact"), "missing installer artifact should be reported");
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
  assert(missing.executionPolicy.readsSigningProfile === false, "missing report should not declare profile reads");
  assert(missing.executionPolicy.checksExpectedSignerSubject === false, "missing report should not declare signer policy checks");
  assert(missing.signaturePolicy.expectedSignerSubjectConfigured === false, "missing report should expose absent signer policy");
  assert(missing.signatureVerification.signaturePolicy.expectedSignerSubjectConfigured === false, "missing signature report should expose absent signer policy");
  assert(missing.installer.postBuildSignatureVerification.signaturePolicy.expectedSignerSubjectConfigured === false, "missing installer signature report should expose absent signer policy");

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
    expectedSignerSubjectContains: "NexusIM",
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
  assert(fullyReady.ready.signedInstallerValid === false, "installer signature readiness should wait for an installer artifact");
  assert(fullyReady.signaturePolicy.expectedSignerSubjectConfigured === true, "readiness report should expose configured signer policy");
  assert(fullyReady.signaturePolicy.expectedSignerSubjectMatched === true, "readiness report should expose matched signer policy");
  assert(fullyReady.signatureVerification.readyForSignedDistribution === true, "valid expected signer should pass signature verification");
  assert(fullyReady.signatureVerification.signaturePolicy.expectedSignerSubjectConfigured === true, "signature verification summary should expose signer policy");
  assert(fullyReady.signatureVerification.signaturePolicy.expectedSignerSubjectMatched === true, "signature verification summary should expose signer match");
  assert(fullyReady.installer.postBuildSignatureVerification.artifactPresent === false, "installer artifact should not be present in exe-only manifest");
  assert(fullyReady.installer.postBuildSignatureVerification.readyForSignedDistribution === false, "missing installer artifact should not be distribution-ready");
  assert(fullyReady.installer.postBuildSignatureVerification.signaturePolicy.expectedSignerSubjectConfigured === true, "missing installer artifact should still expose configured signer policy");
  assert(fullyReady.installer.postBuildSignatureVerification.signaturePolicy.expectedSignerSubjectMatched === false, "missing installer artifact should not claim signer match");
  assert(fullyReady.installer.target === "nsis", "installer target should be preserved");
  assert(Array.isArray(fullyReady.installer.commandTemplate?.build), "ready installer command template missing");
  assert(fullyReady.nextActions.length === 1, "ready report should contain a single next action");
  assert(fullyReady.nextActions[0].includes("installer build execute"), "ready report should point at installer execute path");

  const fullySignedInstaller = buildDesktopSigningReadinessReport({
    manifest: installerManifestPath,
    tauriConfig: activeConfigPath,
    target: "msi",
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    expectedSignerSubjectContains: "NexusIM",
    ...readyPfxOptions,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(fullySignedInstaller.ready.canBuildInstaller === true, "signed installer manifest should keep installer build readiness");
  assert(fullySignedInstaller.ready.signedInstallerValid === true, "valid installer artifact should be installer-signature-ready");
  assert(fullySignedInstaller.installer.postBuildSignatureVerification.artifactPresent === true, "installer artifact should be detected");
  assert(fullySignedInstaller.installer.postBuildSignatureVerification.readyForSignedDistribution === true, "valid installer artifact should be distribution-ready");
  assert(fullySignedInstaller.installer.postBuildSignatureVerification.signaturePolicy.expectedSignerSubjectConfigured === true, "valid installer artifact should expose configured signer policy");
  assert(fullySignedInstaller.installer.postBuildSignatureVerification.signaturePolicy.expectedSignerSubjectMatched === true, "valid installer artifact should expose signer match");
  assert(fullySignedInstaller.nextActions.length === 1, "signed installer report should contain a single next action");
  assert(fullySignedInstaller.nextActions[0].includes("release checks passed"), "signed installer report should report completed release checks");

  const unsignedInstaller = buildDesktopSigningReadinessReport({
    manifest: installerManifestPath,
    tauriConfig: activeConfigPath,
    target: "msi",
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    ...readyPfxOptions,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    },
    mockInstallerSignatureStatus: {
      status: "NotSigned"
    }
  });
  assert(unsignedInstaller.ready.canBuildInstaller === true, "unsigned installer fixture should not affect executable build readiness");
  assert(unsignedInstaller.ready.signedInstallerValid === false, "unsigned installer artifact should not be installer-signature-ready");
  assert(unsignedInstaller.blockers.signedInstaller.includes("valid-authenticode-signature"), "unsigned installer signature blocker missing");
  assert(unsignedInstaller.nextActions.some(action => action.includes("sign the desktop-installer artifact")), "unsigned installer next action should request installer signing");

  const wrongSignerReport = buildDesktopSigningReadinessReport({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "nsis",
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    expectedSignerSubjectContains: "Other Publisher",
    ...readyPfxOptions,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(wrongSignerReport.ready.signatureValid === false, "wrong signer should block signature readiness");
  assert(wrongSignerReport.ready.canBuildInstaller === false, "wrong signer should block installer readiness");
  assert(wrongSignerReport.signaturePolicy.expectedSignerSubjectConfigured === true, "wrong signer report should expose configured signer policy");
  assert(wrongSignerReport.signaturePolicy.expectedSignerSubjectMatched === false, "wrong signer report should expose signer mismatch");
  assert(wrongSignerReport.blockers.signature.includes("expected-signer-subject"), "wrong signer should appear in signature blockers");
  assert(wrongSignerReport.blockers.installer.includes("desktop-signature-valid"), "wrong signer should block installer via signature validity");

  const cliProfileReport = runReporter([
    "--manifest",
    manifestPath,
    "--installer-manifest",
    installerManifestPath,
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
  assert(cliProfileReport.executionPolicy.readsSigningProfile === true, "CLI profile report should declare profile reads");
  assert(cliProfileReport.executionPolicy.checksExpectedSignerSubject === true, "CLI profile report should declare expected signer checks");
  assert(cliProfileReport.signaturePolicy.expectedSignerSubjectConfigured === true, "CLI profile report should expose signer policy");
  assert(cliProfileReport.signaturePolicy.expectedSignerSubjectMatched === false, "CLI profile report should expose signer mismatch for unsigned fixture");
  assert(cliProfileReport.signingExecution.executionPolicy.readsSigningProfile === true, "CLI profile signing execution should declare profile reads");
  assert(cliProfileReport.signingExecution.executionPolicy.expectedSignerSubjectPolicyConfigured === true, "CLI profile signing execution should declare expected signer policy");
  assert(cliProfileReport.signingExecution.executionPolicy.requiresExpectedSignerSubjectAfterSigning === true, "CLI profile signing execution should require signer subject with require-valid");
  assert(cliProfileReport.blockers.signature.includes("expected-signer-subject"), "CLI profile report should apply expected signer policy");
  assert(cliProfileReport.installer.postBuildSignatureVerification.artifactPresent === true, "CLI installer manifest should drive post-build installer verification");
  assert(!cliJSON.includes(tempRoot), "CLI readiness report leaked absolute temp path");
  assert(!cliJSON.match(/token|secret|password|credential|private/i), "CLI readiness report leaked sensitive names");
  assert(readFileSync(fakePfx).length > 0, "fixture pfx should still exist");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signing readiness report ok");
