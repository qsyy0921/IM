import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const smokeScript = join(toolsDir, "smoke-desktop-artifact-launch.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-launch-smoke-"));
try {
  const artifactDir = join(tempRoot, "artifact");
  mkdirSync(artifactDir, { recursive: true });
  const exeBody = "fake exe bytes";
  writeFileSync(join(artifactDir, "nexusim-windows-desktop.exe"), exeBody);
  writeFileSync(join(artifactDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-22T00:00:00.000Z",
    gitCommit: "test",
    runId: "desktop-launch-test",
    artifacts: [
      {
        target: "windows-desktop",
        filename: "nexusim-windows-desktop.exe",
        bytes: Buffer.byteLength(exeBody),
        sha256: sha256(exeBody),
        sourcePathHash: sha256("desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/nexusim-desktop.exe"
      }
    ]
  }, null, 2)}\n`);

  const output = execFileSync(process.execPath, [
    smokeScript,
    "--manifest",
    join(artifactDir, "manifest.json"),
    "--hold-ms",
    "1000",
    "--dry-run"
  ], {
    encoding: "utf8"
  });
  const plan = JSON.parse(output);
  const serialized = JSON.stringify(plan);

  assert(plan.schemaVersion === "nexusim.desktop-artifact-launch-smoke.v1", "desktop launch smoke schema mismatch");
  assert(plan.dryRun === true, "dry-run flag missing");
  assertDryRunExecutionPolicy(plan.executionPolicy);
  assert(plan.launched === false, "dry-run must not launch");
  assert(plan.artifact.filename === "nexusim-windows-desktop.exe", "artifact filename mismatch");
  assert(plan.artifact.sha256 === sha256(exeBody), "artifact sha mismatch");
  assert(!serialized.match(/[A-Z]:\\\\/), "desktop launch smoke leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "desktop launch smoke leaked extended Windows path");
  assert(!serialized.match(/token|secret|password|credential|private/i), "desktop launch smoke leaked sensitive field name");

  console.log("desktop artifact launch smoke ok");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

function assertDryRunExecutionPolicy(policy) {
  assert(policy?.planOnly === true, "desktop launch dry-run should be marked plan-only");
  assert(policy.readsManifest === true, "desktop launch dry-run should read manifest");
  assert(policy.validatesArtifactFile === true, "desktop launch dry-run should validate artifact file");
  assert(policy.readsArtifactBytes === true, "desktop launch dry-run should read artifact bytes for hash checks");
  assert(policy.startsArtifact === false, "desktop launch dry-run should not start artifact");
  assert(policy.terminatesArtifact === false, "desktop launch dry-run should not terminate artifact");
  assert(policy.startsServices === false, "desktop launch dry-run should not start services");
  assert(policy.opensNetworkConnection === false, "desktop launch dry-run should not open network connections");
  assert(policy.installsArtifacts === false, "desktop launch dry-run should not install artifacts");
  assert(policy.contactsDevice === false, "desktop launch dry-run should not contact devices");
  assert(policy.downloadsToolchain === false, "desktop launch dry-run should not download toolchains");
}
