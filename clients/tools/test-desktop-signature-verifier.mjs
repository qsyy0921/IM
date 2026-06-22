import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { execFileSync, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { buildDesktopSignatureVerificationReport } from "./verify-desktop-signature.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const verifier = join(toolsDir, "verify-desktop-signature.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function runVerifier(args) {
  const output = execFileSync(process.execPath, [verifier, ...args], {
    encoding: "utf8",
    env: {
      ...process.env
    }
  });
  return JSON.parse(output);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-signature-verifier-"));
try {
  const collectedDir = join(tempRoot, "collected");
  mkdirSync(collectedDir, { recursive: true });
  const exe = "fake desktop executable bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exe);
  const manifest = {
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-23T00:00:00.000Z",
    gitCommit: "testcommit",
    runId: "desktop-signature-verifier-test",
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

  const valid = buildDesktopSignatureVerificationReport({
    manifest: manifestPath,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567",
      timeStamperSubject: "CN=Timestamp Test",
      timeStamperThumbprint: "fedcba9876543210fedcba9876543210fedcba98"
    }
  });
  const validJSON = JSON.stringify(valid);
  assert(valid.readyForSignedDistribution === true, "valid signature should be ready");
  assert(valid.signature.trusted === true, "valid signature should be trusted");
  assert(valid.signature.signer.thumbprintPrefix === "01234567", "signer prefix missing");
  assert(valid.executionPolicy.signsArtifacts === false, "signature verifier must not sign artifacts");
  assert(valid.executionPolicy.installsArtifacts === false, "signature verifier must not install artifacts");
  assert(!validJSON.includes(tempRoot), "valid signature report leaked absolute temp path");
  assert(!validJSON.match(/token|secret|password|credential|private/i), "valid signature report leaked sensitive names");

  const unsigned = buildDesktopSignatureVerificationReport({
    manifest: manifestPath,
    mockSignatureStatus: {
      status: "NotSigned"
    }
  });
  assert(unsigned.readyForSignedDistribution === false, "unsigned artifact should not be ready");
  assert(unsigned.missing.includes("valid-authenticode-signature"), "unsigned artifact should require a valid signature");
  assert(unsigned.signature.signed === false, "unsigned artifact should report signed=false");

  const installer = "fake desktop installer bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop-installer.msi"), installer);
  const mixedManifest = {
    ...manifest,
    runId: "desktop-signature-verifier-mixed",
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
  const mixedDefault = buildDesktopSignatureVerificationReport({
    manifest: mixedManifestPath,
    mockSignatureStatus: { status: "NotSigned" }
  });
  assert(mixedDefault.artifact.artifactKind === "desktop-executable", "default verifier must select desktop executable");
  assert(mixedDefault.artifact.filename === "nexusim-windows-desktop.exe", "default verifier selected wrong filename");
  const mixedInstaller = buildDesktopSignatureVerificationReport({
    manifest: mixedManifestPath,
    artifactKind: "desktop-installer",
    mockSignatureStatus: { status: "Valid" }
  });
  assert(mixedInstaller.readyForSignedDistribution === true, "explicit installer verifier should accept valid signature");
  assert(mixedInstaller.artifact.artifactKind === "desktop-installer", "explicit verifier should select desktop installer");

  const legacyManifest = {
    ...manifest,
    runId: "legacy-missing-artifact-kind",
    artifacts: manifest.artifacts.map(({ artifactKind, ...artifact }) => artifact)
  };
  const legacyManifestPath = join(collectedDir, "manifest-legacy.json");
  writeFileSync(legacyManifestPath, `${JSON.stringify(legacyManifest, null, 2)}\n`);
  const legacyReport = buildDesktopSignatureVerificationReport({ manifest: legacyManifestPath });
  assert(legacyReport.readyForSignedDistribution === false, "legacy verifier manifest should fail closed");
  assert(legacyReport.missing.includes("desktop-artifact-kind"), "legacy verifier manifest should require explicit artifactKind");

  const cliReport = runVerifier(["--manifest", manifestPath]);
  assert(cliReport.readyForSignedDistribution === false, "fixture artifact should not be treated as signed");
  assert(cliReport.executionPolicy.readOnly === true, "CLI verifier should be read-only");
  assert(cliReport.executionPolicy.signsArtifacts === false, "CLI verifier must not sign artifacts");
  assert(!JSON.stringify(cliReport).includes(tempRoot), "CLI signature report leaked absolute temp path");

  const requireValid = spawnSync(process.execPath, [
    verifier,
    "--require-valid",
    "--manifest",
    manifestPath
  ], {
    encoding: "utf8"
  });
  assert(requireValid.status === 2, "require-valid should fail closed for unsigned fixtures");
  const requireValidReport = JSON.parse(requireValid.stdout);
  assert(requireValidReport.executionPolicy.requireValidSignature === true, "require-valid report should mark require-valid mode");
  assert(requireValidReport.readyForSignedDistribution === false, "require-valid report should not be ready for unsigned fixture");

  const androidManifest = {
    ...manifest,
    runId: "android-only",
    artifacts: [
      {
        target: "android",
        filename: "nexusim-android-debug.apk",
        bytes: 3,
        sha256: sha256("apk")
      }
    ]
  };
  const androidManifestPath = join(collectedDir, "manifest-android.json");
  writeFileSync(androidManifestPath, `${JSON.stringify(androidManifest, null, 2)}\n`);
  const androidOnly = buildDesktopSignatureVerificationReport({ manifest: androidManifestPath });
  assert(androidOnly.readyForSignedDistribution === false, "android-only manifest should not be ready");
  assert(androidOnly.missing.includes("windows-desktop-artifact"), "android-only manifest should report missing desktop artifact");

  assert(readFileSync(manifestPath, "utf8").includes("desktop-signature-verifier-test"), "fixture manifest should remain unchanged");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signature verifier ok");
