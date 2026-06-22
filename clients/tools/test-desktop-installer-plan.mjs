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
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname } from "node:path";
import { buildDesktopInstallerPlan } from "./plan-desktop-installer.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const installerPlanner = join(toolsDir, "plan-desktop-installer.mjs");

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

function runPlanner(args, env = {}) {
  const output = execFileSync(process.execPath, [installerPlanner, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      ...env
    }
  });
  return JSON.parse(output);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-installer-"));
try {
  const collectedDir = join(tempRoot, "collected");
  mkdirSync(collectedDir, { recursive: true });
  const exe = "fake desktop executable bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exe);
  const manifest = {
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-23T00:00:00.000Z",
    gitCommit: "testcommit",
    runId: "desktop-installer-test",
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

  const inactiveConfigPath = join(tempRoot, "tauri-inactive.json");
  writeJSON(inactiveConfigPath, {
    productName: "NexusIM",
    version: "0.1.0",
    identifier: "com.nexusim.desktop",
    bundle: {
      active: false,
      targets: ["msi"],
      publisher: "NexusIM"
    }
  });

  const inactive = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: inactiveConfigPath,
    target: "msi"
  });
  assert(inactive.readyToBuildInstaller === false, "inactive bundle config should not be ready");
  assert(inactive.missing.includes("tauri-bundle-active"), "inactive bundle should be reported");
  assert(inactive.missing.includes("desktop-signing-ready"), "missing signing config should be reported");
  assert(inactive.missing.includes("desktop-signature-valid"), "missing valid signature should be reported");
  assert(inactive.executionPolicy.planOnly === true, "installer plan should be plan-only");
  assert(inactive.executionPolicy.buildsInstaller === false, "installer plan should not build installers");
  assert(inactive.executionPolicy.readsAuthenticodeSignature === true, "installer plan should read signature state");

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
  const msiOnlyConfigPath = join(tempRoot, "tauri-msi-only.json");
  writeJSON(msiOnlyConfigPath, {
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

  const ready = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "msi",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  const readyJSON = JSON.stringify(ready);
  assert(ready.readyToBuildInstaller === true, "active MSI config with signing inputs should be ready");
  assert(ready.target === "msi", "ready plan target mismatch");
  assert(Array.isArray(ready.commandTemplate?.build), "ready plan should include build command template");
  assert(ready.commandTemplate.build.includes("tauri:build"), "ready build command should invoke Tauri build");
  assert(ready.commandTemplate.build.includes("--bundles"), "ready build command should select bundle target");
  assert(ready.commandTemplate.build.includes("msi"), "ready build command should include MSI target");
  assert(ready.commandTemplate.build.includes("--config"), "ready build command should use an explicit Tauri config");
  assert(ready.commandTemplate.build.some(value => value.includes("tauri-active") || value.includes("tauri.installer.conf.json")), "ready build command should include a Tauri config hint");
  assert(Array.isArray(ready.commandTemplate?.collect), "ready plan should include collect command template");
  assert(ready.commandTemplate.collect.includes("collect:client-artifacts"), "ready collect command should collect artifacts");
  assert(ready.expectedOutputHint.endsWith("/msi/"), "ready plan output hint should point at MSI bundle output");
  assert(ready.artifactBaseline.artifact.artifactKind === "desktop-executable", "ready plan should expose executable artifact kind");
  assert(ready.signatureVerification.readyForSignedDistribution === true, "ready plan should require a valid signature");
  assert(ready.signatureVerification.status === "Valid", "ready plan should carry signature status");
  assert(!readyJSON.includes(tempRoot), "ready installer plan leaked absolute temp path");
  assert(!readyJSON.match(/token|secret|password|credential|private/i), "ready installer plan leaked sensitive names");

  const installerArtifact = "fake msi bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop-installer.msi"), installerArtifact);
  const mixedManifestPath = join(collectedDir, "manifest-mixed.json");
  writeJSON(mixedManifestPath, {
    ...manifest,
    runId: "desktop-installer-mixed",
    artifacts: [
      {
        target: "windows-desktop",
        artifactKind: "desktop-installer",
        filename: "nexusim-windows-desktop-installer.msi",
        bytes: Buffer.byteLength(installerArtifact),
        sha256: sha256(installerArtifact),
        sourcePathHash: sha256("desktop-installer-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      },
      ...manifest.artifacts
    ]
  });
  const mixedReady = buildDesktopInstallerPlan({
    manifest: mixedManifestPath,
    tauriConfig: activeConfigPath,
    target: "msi",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(mixedReady.readyToBuildInstaller === true, "mixed manifest should select executable baseline");
  assert(mixedReady.artifactBaseline.artifact.artifactKind === "desktop-executable", "mixed manifest selected wrong baseline kind");
  assert(mixedReady.artifactBaseline.artifact.filename === "nexusim-windows-desktop.exe", "mixed manifest selected wrong baseline filename");

  const nsisReady = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "nsis",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(nsisReady.readyToBuildInstaller === true, "active NSIS config with signing inputs should be ready");
  assert(nsisReady.target === "nsis", "NSIS ready plan target mismatch");
  assert(nsisReady.commandTemplate.build.includes("nsis"), "NSIS ready build command should include NSIS target");
  assert(nsisReady.expectedOutputHint.endsWith("/nsis/"), "NSIS ready output hint should point at NSIS bundle output");

  const nsisMissing = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: msiOnlyConfigPath,
    target: "nsis",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(nsisMissing.readyToBuildInstaller === false, "undeclared NSIS target should not be ready");
  assert(nsisMissing.missing.includes("installer-target-declared"), "undeclared target should be reported");

  const unsupported = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "portable",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(unsupported.readyToBuildInstaller === false, "unsupported target should not be ready");
  assert(unsupported.missing.includes("supported-installer-target"), "unsupported target should be reported");

  writeJSON(manifestPath, {
    ...manifest,
    artifacts: [
      {
        target: "windows-desktop",
        artifactKind: "desktop-installer",
        filename: "nexusim-windows-desktop-installer.msi",
        bytes: Buffer.byteLength(installerArtifact),
        sha256: sha256(installerArtifact),
        sourcePathHash: sha256("desktop-installer-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      }
    ]
  });
  const installerAsBaseline = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "msi",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(installerAsBaseline.readyToBuildInstaller === false, "installer artifact must not be accepted as installer build baseline");
  assert(installerAsBaseline.missing.includes("desktop-executable-baseline"), "installer artifact baseline should require executable baseline");
  assert(installerAsBaseline.artifactBaseline.artifact.artifactKind === "desktop-installer", "installer baseline kind should be exposed");

  writeJSON(manifestPath, {
    ...manifest,
    artifacts: [
      {
        target: "windows-desktop",
        filename: "nexusim-windows-desktop.exe",
        bytes: Buffer.byteLength(exe),
        sha256: sha256(exe),
        sourcePathHash: sha256("legacy-desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/nexusim.exe"
      }
    ]
  });
  const legacyArtifact = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "msi",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(legacyArtifact.readyToBuildInstaller === false, "legacy artifact without kind must not be accepted as installer build baseline");
  assert(legacyArtifact.missing.includes("desktop-artifact-kind"), "legacy artifact should require explicit artifact kind");
  assert(legacyArtifact.artifactBaseline.artifact.artifactKind === "", "legacy baseline kind should be empty");

  writeJSON(manifestPath, manifest);

  const artifactsRoot = join(tempRoot, "artifacts");
  const desktopRun = join(artifactsRoot, "desktop-old");
  const androidRun = join(artifactsRoot, "android-new");
  mkdirSync(desktopRun, { recursive: true });
  mkdirSync(androidRun, { recursive: true });
  writeFileSync(join(desktopRun, "nexusim-windows-desktop.exe"), exe);
  writeJSON(join(desktopRun, "manifest.json"), manifest);
  writeFileSync(join(androidRun, "nexusim-android-debug.apk"), "apk");
  writeJSON(join(androidRun, "manifest.json"), {
    ...manifest,
    runId: "android-newer",
    artifacts: [
      {
        target: "android",
        filename: "nexusim-android-debug.apk",
        bytes: 3,
        sha256: sha256("apk")
      }
    ]
  });
  const targetSelected = buildDesktopInstallerPlan({
    artifactsRoot,
    tauriConfig: activeConfigPath,
    target: "msi",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true,
    mockSignatureStatus: {
      status: "Valid",
      signerSubject: "CN=NexusIM Test Code Signing",
      signerThumbprint: "0123456789abcdef0123456789abcdef01234567"
    }
  });
  assert(targetSelected.readyToBuildInstaller === true, "default installer plan should select latest desktop manifest, not latest android manifest");
  assert(targetSelected.artifactBaseline.runId === "desktop-installer-test", "default installer plan selected the wrong manifest");

  const cliPlan = runPlanner([
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
  assert(cliPlan.readyToBuildInstaller === false, "CLI installer plan should not be ready for unsigned fixtures");
  assert(cliPlan.missing.includes("desktop-signature-valid"), "CLI installer plan should require valid signature");
  assert(cliPlan.signatureVerification.trusted === false, "CLI installer plan should not trust unsigned fixtures");
  assert(!JSON.stringify(cliPlan).includes(tempRoot), "CLI installer plan leaked absolute temp path");

  assert(readFileSync(fakePfx, "utf8") === "fake pfx", "fixture pfx should still exist");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop installer plan ok");
