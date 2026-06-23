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

const toolsDir = dirname(fileURLToPath(import.meta.url));
const signingPlanner = join(toolsDir, "plan-desktop-signing.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function runPlanner(args, env = {}) {
  const output = execFileSync(process.execPath, [signingPlanner, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      ...env
    }
  });
  return JSON.parse(output);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-signing-"));
try {
  const collectedDir = join(tempRoot, "collected");
  mkdirSync(collectedDir, { recursive: true });
  const exe = "fake desktop executable bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop.exe"), exe);
  const manifest = {
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-23T00:00:00.000Z",
    gitCommit: "testcommit",
    runId: "desktop-signing-test",
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
  const fakeSignTool = join(tempRoot, "signtool.exe");
  const fakePfx = join(tempRoot, "nexusim-signing.pfx");
  const signingProfile = join(tempRoot, "desktop-signing-profile.json");
  const unsafeSigningProfile = join(tempRoot, "unsafe-desktop-signing-profile.json");
  writeFileSync(fakeSignTool, "fake signtool");
  writeFileSync(fakePfx, "fake pfx");
  writeFileSync(signingProfile, `${JSON.stringify({
    schemaVersion: "nexusim.desktop-signing-profile.v1",
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      pfxPassEnv: "NEXUSIM_TEST_DESKTOP_PFX_PASS"
    }
  }, null, 2)}\n`);
  writeFileSync(unsafeSigningProfile, `${JSON.stringify({
    schemaVersion: "nexusim.desktop-signing-profile.v1",
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      pfxPassword: "do-not-store"
    }
  }, null, 2)}\n`);

  const missing = buildDesktopSigningPlan({ manifest: manifestPath });
  assert(missing.readyToSign === false, "signing plan without config should not be ready");
  assert(missing.missing.includes("signtool-path"), "missing signtool should be reported");
  assert(missing.missing.includes("timestamp-url"), "missing timestamp should be reported");
  assert(missing.missing.includes("certificate-source"), "missing certificate should be reported");
  assert(missing.executionPolicy.planOnly === true, "signing plan should be plan-only");
  assert(missing.executionPolicy.signsArtifacts === false, "signing plan should not sign artifacts");

  const ready = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true
  });
  const readyJSON = JSON.stringify(ready);
  assert(ready.readyToSign === true, "complete signing config should be ready");
  assert(ready.signing.mode === "pfx", "pfx signing mode expected");
  assert(ready.signing.certificate.pfxPassEnvPresent === true, "pfx env presence expected");
  assert(Array.isArray(ready.commandTemplate), "sign command template missing");
  assert(ready.commandTemplate.includes("%NEXUSIM_DESKTOP_SIGN_PFX_PASS%"), "pfx env marker missing");
  assert(!readyJSON.includes(tempRoot), "ready signing plan leaked absolute temp path");
  assert(!readyJSON.match(/token|secret|password|credential|private/i), "ready signing plan leaked sensitive names");

  const cliPlan = runPlanner([
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
  assert(cliPlan.readyToSign === true, "CLI signing plan should be ready");
  assert(!JSON.stringify(cliPlan).includes(tempRoot), "CLI signing plan leaked absolute temp path");

  const cliProfilePlan = runPlanner([
    "--manifest",
    manifestPath,
    "--signing-profile",
    signingProfile
  ], {
    NEXUSIM_TEST_DESKTOP_PFX_PASS: "present"
  });
  assert(cliProfilePlan.readyToSign === true, "CLI signing profile plan should be ready");
  assert(cliProfilePlan.signing.certificate.pfxPassEnv === "NEXUSIM_TEST_DESKTOP_PFX_PASS", "profile pfx pass env should be preserved");
  assert(!JSON.stringify(cliProfilePlan).includes(tempRoot), "CLI signing profile plan leaked absolute temp path");

  const unsafeProfileRun = spawnSync(process.execPath, [
    signingPlanner,
    "--manifest",
    manifestPath,
    "--signing-profile",
    unsafeSigningProfile
  ], {
    encoding: "utf8"
  });
  assert(unsafeProfileRun.status === 2, "unsafe signing profile should fail closed");
  assert(unsafeProfileRun.stderr.includes("desktop signing profile contains a sensitive field name"), "unsafe signing profile should report sensitive field");

  const invalidSHA1 = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certSHA1: "not-a-thumbprint",
    timestampURL: "https://timestamp.example.test"
  });
  assert(invalidSHA1.readyToSign === false, "invalid SHA1 should not be ready");
  assert(invalidSHA1.missing.includes("certificate-sha1"), "invalid SHA1 should be reported");

  const readyStoreSHA1 = "AABBCCDDEEFF00112233445566778899AABBCCDD";
  const storeReady = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certSHA1: readyStoreSHA1,
    timestampURL: "https://timestamp.example.test",
    certificateStoreProbe: () => [
      {
        store: "CurrentUser/My",
        signingKeyAvailable: true,
        notAfter: "2035-01-01T00:00:00.000Z"
      }
    ]
  });
  assert(storeReady.readyToSign === true, "usable cert-store thumbprint should be ready");
  assert(storeReady.signing.mode === "cert-store-sha1", "cert store signing mode expected");
  assert(storeReady.signing.certificate.storeReadiness.usable === true, "cert store readiness should be usable");
  assert(storeReady.signing.certificate.storeReadiness.storeScopes.includes("CurrentUser/My"), "cert store scope should be reported");
  assert(!JSON.stringify(storeReady).includes(readyStoreSHA1), "full cert-store thumbprint must not be echoed");

  const storeMissing = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certSHA1: readyStoreSHA1,
    timestampURL: "https://timestamp.example.test",
    certificateStoreProbe: () => []
  });
  assert(storeMissing.readyToSign === false, "missing cert-store entry should not be ready");
  assert(storeMissing.missing.includes("certificate-store-entry"), "missing cert-store entry should be reported");

  const storeNoKey = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certSHA1: readyStoreSHA1,
    timestampURL: "https://timestamp.example.test",
    certificateStoreProbe: () => [
      {
        store: "CurrentUser/My",
        signingKeyAvailable: false,
        notAfter: "2035-01-01T00:00:00.000Z"
      }
    ]
  });
  assert(storeNoKey.readyToSign === false, "cert-store entry without signing key should not be ready");
  assert(storeNoKey.missing.includes("certificate-key-access"), "missing cert-store signing key should be reported");

  const invalidTimestamp = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "timestamp.example.test",
    pfxPassEnvPresent: true
  });
  assert(invalidTimestamp.readyToSign === false, "invalid timestamp URL should not be ready");
  assert(invalidTimestamp.missing.includes("timestamp-url-valid"), "invalid timestamp should be reported");

  const timestampWithQuery = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test/path?token=do-not-store",
    pfxPassEnvPresent: true
  });
  assert(timestampWithQuery.readyToSign === false, "timestamp URL with query should not be ready");
  assert(timestampWithQuery.missing.includes("timestamp-url-valid"), "timestamp query should be reported");
  assert(!JSON.stringify(timestampWithQuery).includes("do-not-store"), "unsafe timestamp query must not be echoed");

  const installer = "fake desktop installer bytes";
  writeFileSync(join(collectedDir, "nexusim-windows-desktop-installer.msi"), installer);
  const mixedManifest = {
    ...manifest,
    runId: "desktop-signing-mixed",
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
  const mixedDefault = buildDesktopSigningPlan({
    manifest: mixedManifestPath,
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true
  });
  assert(mixedDefault.readyToSign === true, "mixed manifest default signing should be ready");
  assert(mixedDefault.artifact.artifactKind === "desktop-executable", "default signing must select desktop executable");
  assert(mixedDefault.artifact.filename === "nexusim-windows-desktop.exe", "default signing selected wrong artifact filename");
  const mixedInstaller = buildDesktopSigningPlan({
    manifest: mixedManifestPath,
    artifactKind: "desktop-installer",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true
  });
  assert(mixedInstaller.readyToSign === true, "explicit installer signing should be ready");
  assert(mixedInstaller.artifact.artifactKind === "desktop-installer", "explicit signing should select desktop installer");

  const legacyManifest = {
    ...manifest,
    runId: "legacy-missing-artifact-kind",
    artifacts: manifest.artifacts.map(({ artifactKind, ...artifact }) => artifact)
  };
  const legacyManifestPath = join(collectedDir, "manifest-legacy.json");
  writeFileSync(legacyManifestPath, `${JSON.stringify(legacyManifest, null, 2)}\n`);
  const legacyPlan = buildDesktopSigningPlan({ manifest: legacyManifestPath });
  assert(legacyPlan.readyToSign === false, "legacy manifest without artifactKind should fail closed");
  assert(legacyPlan.missing.includes("desktop-artifact-kind"), "legacy manifest should require explicit artifactKind");

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
  const androidOnly = buildDesktopSigningPlan({ manifest: androidManifestPath });
  assert(androidOnly.readyToSign === false, "android-only manifest should not be ready for desktop signing");
  assert(androidOnly.missing.includes("windows-desktop-artifact"), "android-only manifest should report missing desktop artifact");

  const artifactsRoot = join(tempRoot, "artifacts");
  const desktopRun = join(artifactsRoot, "desktop-old");
  const androidRun = join(artifactsRoot, "android-new");
  mkdirSync(desktopRun, { recursive: true });
  mkdirSync(androidRun, { recursive: true });
  writeFileSync(join(desktopRun, "nexusim-windows-desktop.exe"), exe);
  writeFileSync(join(desktopRun, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);
  writeFileSync(join(androidRun, "nexusim-android-debug.apk"), "apk");
  writeFileSync(join(androidRun, "manifest.json"), `${JSON.stringify(androidManifest, null, 2)}\n`);
  const targetSelected = buildDesktopSigningPlan({
    artifactsRoot,
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true
  });
  assert(targetSelected.readyToSign === true, "default signing plan should select latest desktop manifest, not latest android manifest");
  assert(targetSelected.artifactManifest.runId === "desktop-signing-test", "default signing plan selected the wrong manifest");

  const unsignedText = JSON.stringify(missing);
  assert(!unsignedText.includes(tempRoot), "missing signing plan leaked absolute temp path");
  assert(existsSync(fakeSignTool), "fixture signtool should exist");
  assert(readFileSync(fakePfx, "utf8") === "fake pfx", "fixture pfx should exist");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signing plan ok");
