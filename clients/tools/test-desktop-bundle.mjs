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
import {
  buildDesktopBundlePlan,
  writeDesktopBundle
} from "./package-desktop-bundle.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-bundle-"));
try {
  const collectedDir = join(tempRoot, "collected");
  const outputDir = join(tempRoot, "bundles");
  mkdirSync(collectedDir, { recursive: true });
  const exe = "fake desktop executable bytes";
  const readme = "NexusIM Windows desktop local package\n";
  const launcher = "$ErrorActionPreference = 'Stop'\nStart-Process -FilePath (Join-Path $PSScriptRoot 'nexusim-windows-desktop.exe')\n";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exe);
  writeFileSync(join(collectedDir, "README-windows-desktop.txt"), readme);
  writeFileSync(join(collectedDir, "launch-nexusim-windows.ps1"), launcher);
  const manifest = {
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-23T00:00:00.000Z",
    gitCommit: "testcommit",
    runId: "collected-test",
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
    ],
    supportFiles: [
      {
        target: "windows-desktop",
        filename: "README-windows-desktop.txt",
        bytes: Buffer.byteLength(readme),
        sha256: sha256(readme)
      },
      {
        target: "windows-desktop",
        filename: "launch-nexusim-windows.ps1",
        bytes: Buffer.byteLength(launcher),
        sha256: sha256(launcher)
      }
    ]
  };
  const manifestPath = join(collectedDir, "manifest.json");
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const dryRun = buildDesktopBundlePlan({
    manifest: manifestPath,
    outputDir,
    runId: "dry-run",
    dryRun: true
  });
  assert(dryRun.ready === true, "desktop bundle dry-run should be ready");
  assert(dryRun.executionPolicy.planOnly === true, "desktop bundle dry-run should be plan-only");
  assert(dryRun.executionPolicy.writesBundleZip === false, "desktop bundle dry-run should not write zip");
  assert(dryRun.signing.signed === false, "desktop bundle should not pretend to be signed");
  assert(dryRun.signing.status === "unsigned-local-dev", "desktop bundle signing status mismatch");
  assert(!Object.keys(dryRun).includes("internal"), "desktop bundle internal state should be non-enumerable");
  assert(!JSON.stringify(dryRun).includes(tempRoot), "desktop bundle dry-run leaked absolute temp path");
  assert(!JSON.stringify(dryRun).match(/token|secret|password|credential|private/i), "desktop bundle dry-run leaked sensitive names");

  const plan = buildDesktopBundlePlan({
    manifest: manifestPath,
    outputDir,
    runId: "bundle-test"
  });
  const result = writeDesktopBundle(plan);
  const bundleDir = join(outputDir, "bundle-test");
  const bundlePath = join(bundleDir, "nexusim-windows-desktop-bundle.zip");
  const summaryPath = join(bundleDir, "desktop-bundle-summary.json");
  assert(existsSync(bundlePath), "desktop bundle zip missing");
  assert(existsSync(summaryPath), "desktop bundle summary missing");
  const bundleBytes = readFileSync(bundlePath);
  const summary = JSON.parse(readFileSync(summaryPath, "utf8"));
  assert(bundleBytes[0] === 0x50 && bundleBytes[1] === 0x4b, "desktop bundle is not a zip file");
  assert(bundleBytes.includes(Buffer.from("nexusim-windows-desktop.exe")), "desktop bundle zip missing exe entry");
  assert(bundleBytes.includes(Buffer.from("launch-nexusim-windows.ps1")), "desktop bundle zip missing launcher entry");
  assert(bundleBytes.includes(Buffer.from("bundle-manifest.json")), "desktop bundle zip missing bundle manifest entry");
  assert(summary.bundle.sha256 === sha256(bundleBytes), "desktop bundle summary hash mismatch");
  assert(summary.signing.status === "unsigned-local-dev", "desktop bundle summary signing status mismatch");
  assert(result.bundle.sha256 === summary.bundle.sha256, "desktop bundle result hash mismatch");
  assert(!JSON.stringify(summary).includes(tempRoot), "desktop bundle summary leaked absolute temp path");

  const missingLauncherManifest = {
    ...manifest,
    supportFiles: manifest.supportFiles.filter(file => file.filename !== "launch-nexusim-windows.ps1")
  };
  const missingLauncherManifestPath = join(collectedDir, "manifest-missing-launcher.json");
  writeFileSync(missingLauncherManifestPath, `${JSON.stringify(missingLauncherManifest, null, 2)}\n`);
  let failed = false;
  try {
    buildDesktopBundlePlan({
      manifest: missingLauncherManifestPath,
      outputDir,
      runId: "missing-launcher"
    });
  } catch (error) {
    failed = String(error).includes("desktop launcher support file missing");
  }
  assert(failed, "desktop bundle should require launcher support file for exe packages");

  const installerOnly = "fake desktop installer bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop-installer.msi"), installerOnly);
  const installerOnlyManifest = {
    ...manifest,
    artifacts: [
      {
        target: "windows-desktop",
        artifactKind: "desktop-installer",
        filename: "nexusim-windows-desktop-installer.msi",
        bytes: Buffer.byteLength(installerOnly),
        sha256: sha256(installerOnly),
        sourcePathHash: sha256("installer-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      }
    ],
    supportFiles: manifest.supportFiles.filter(file => file.filename === "README-windows-desktop.txt")
  };
  const installerOnlyManifestPath = join(collectedDir, "manifest-installer-only.json");
  writeFileSync(installerOnlyManifestPath, `${JSON.stringify(installerOnlyManifest, null, 2)}\n`);
  const installerOnlyPlan = buildDesktopBundlePlan({
    manifest: installerOnlyManifestPath,
    outputDir,
    runId: "installer-only"
  });
  assert(installerOnlyPlan.ready === false, "desktop portable bundle should not accept installer-only manifests");
  assert(installerOnlyPlan.missing.includes("windows-desktop-artifact"), "installer-only manifest should report missing desktop executable");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop bundle package ok");
