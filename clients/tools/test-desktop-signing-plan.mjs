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
import { execFileSync } from "node:child_process";
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
  writeFileSync(fakeSignTool, "fake signtool");
  writeFileSync(fakePfx, "fake pfx");

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

  const invalidSHA1 = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certSHA1: "not-a-thumbprint",
    timestampURL: "https://timestamp.example.test"
  });
  assert(invalidSHA1.readyToSign === false, "invalid SHA1 should not be ready");
  assert(invalidSHA1.missing.includes("certificate-sha1"), "invalid SHA1 should be reported");

  const invalidTimestamp = buildDesktopSigningPlan({
    manifest: manifestPath,
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "timestamp.example.test",
    pfxPassEnvPresent: true
  });
  assert(invalidTimestamp.readyToSign === false, "invalid timestamp URL should not be ready");
  assert(invalidTimestamp.missing.includes("timestamp-url-valid"), "invalid timestamp should be reported");

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

  const unsignedText = JSON.stringify(missing);
  assert(!unsignedText.includes(tempRoot), "missing signing plan leaked absolute temp path");
  assert(existsSync(fakeSignTool), "fixture signtool should exist");
  assert(readFileSync(fakePfx, "utf8") === "fake pfx", "fixture pfx should exist");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signing plan ok");
