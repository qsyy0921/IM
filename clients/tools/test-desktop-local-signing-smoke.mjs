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
import {
  buildDesktopLocalSigningSmokePlan,
  runDesktopLocalSigningSmoke
} from "./smoke-desktop-local-signing.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const smoke = join(toolsDir, "smoke-desktop-local-signing.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function runSmoke(args) {
  const output = execFileSync(process.execPath, [smoke, ...args], {
    encoding: "utf8"
  });
  return JSON.parse(output);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-local-signing-smoke-test-"));
try {
  const collectedDir = join(tempRoot, "collected");
  mkdirSync(collectedDir, { recursive: true });
  const exeBody = "fake desktop executable bytes";
  const msiBody = "fake desktop installer bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exeBody);
  writeFileSync(join(collectedDir, "nexusim-windows-desktop-installer.msi"), msiBody);
  const manifest = {
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-23T00:00:00.000Z",
    gitCommit: "testcommit",
    runId: "desktop-local-signing-smoke-test",
    artifacts: [
      {
        target: "windows-desktop",
        artifactKind: "desktop-executable",
        filename: "nexusim-windows-desktop.exe",
        bytes: Buffer.byteLength(exeBody),
        sha256: sha256(exeBody),
        sourcePathHash: sha256("desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/nexusim.exe"
      },
      {
        target: "windows-desktop",
        artifactKind: "desktop-installer",
        filename: "nexusim-windows-desktop-installer.msi",
        bytes: Buffer.byteLength(msiBody),
        sha256: sha256(msiBody),
        sourcePathHash: sha256("desktop-installer-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      }
    ]
  };
  const manifestPath = join(collectedDir, "manifest.json");
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const plan = buildDesktopLocalSigningSmokePlan({ manifest: manifestPath });
  assert(plan.artifact.artifactKind === "desktop-executable", "default local signing smoke should select desktop executable");
  assert(plan.executionPolicy.planOnly === true, "default local signing smoke should be plan-only");
  assert(plan.executionPolicy.signsTemporaryArtifactCopy === false, "default local signing smoke must not sign");
  assert(plan.executionPolicy.mutatesCollectedArtifact === false, "local signing smoke must not mutate collected artifact");
  assert(plan.executionPolicy.requiresAllowLocalTrustStoreFlag === true, "local signing smoke must require explicit local trust flag");
  assert(plan.signer.subject.startsWith("CN=NexusIM "), "local signing smoke signer subject must be project-scoped");

  const dryRun = runSmoke(["--manifest", manifestPath]);
  const dryRunJSON = JSON.stringify(dryRun);
  assert(dryRun.artifact.artifactKind === "desktop-executable", "CLI dry-run should select desktop executable");
  assert(dryRun.executionPolicy.planOnly === true, "CLI dry-run should be plan-only");
  assert(dryRun.executionPolicy.createsTemporaryCurrentUserCodeSigningCertificate === false, "CLI dry-run must not create a certificate");
  assert(!dryRunJSON.includes(tempRoot), "CLI dry-run leaked absolute temp path");
  assert(!dryRunJSON.match(/token|secret|password|credential|private/i), "CLI dry-run leaked sensitive names");

  const installerDryRun = runSmoke(["--manifest", manifestPath, "--artifact-kind", "desktop-installer"]);
  assert(installerDryRun.artifact.artifactKind === "desktop-installer", "explicit local signing smoke should select installer artifact");
  assert(installerDryRun.artifact.filename === "nexusim-windows-desktop-installer.msi", "installer dry-run selected wrong artifact");

  const executeWithoutTrust = runDesktopLocalSigningSmoke({
    execute: true,
    manifest: manifestPath
  });
  const executeWithoutTrustJSON = JSON.stringify(executeWithoutTrust);
  assert(executeWithoutTrust.validSignedArtifactCopy === false, "execute without trust flag must not sign");
  assert(executeWithoutTrust.missing.includes("allow-local-trust-store"), "execute without trust flag should fail closed");
  assert(executeWithoutTrust.executionPolicy.signsTemporaryArtifactCopy === false, "execute without trust flag must not sign copy");
  assert(!executeWithoutTrustJSON.includes(tempRoot), "execute without trust output leaked absolute temp path");

  const badSubject = spawnSync(process.execPath, [
    smoke,
    "--manifest",
    manifestPath,
    "--signer-subject",
    "CN=Other Local Signing Smoke"
  ], {
    encoding: "utf8"
  });
  assert(badSubject.status === 2, "non-project signer subject should fail closed");
  assert(
    badSubject.stderr.includes("signer subject must start with CN=NexusIM"),
    "bad signer subject should explain the policy"
  );

  assert(
    readFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), "utf8") === exeBody,
    "local signing smoke dry-run must not mutate executable fixture"
  );
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop local signing smoke ok");
