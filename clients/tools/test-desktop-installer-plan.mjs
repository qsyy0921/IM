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
  assert(inactive.executionPolicy.planOnly === true, "installer plan should be plan-only");
  assert(inactive.executionPolicy.buildsInstaller === false, "installer plan should not build installers");

  const activeConfigPath = join(tempRoot, "tauri-active.json");
  writeJSON(activeConfigPath, {
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
    pfxPassEnvPresent: true
  });
  const readyJSON = JSON.stringify(ready);
  assert(ready.readyToBuildInstaller === true, "active MSI config with signing inputs should be ready");
  assert(ready.target === "msi", "ready plan target mismatch");
  assert(Array.isArray(ready.commandTemplate), "ready plan should include command template");
  assert(ready.expectedOutputHint.endsWith("/msi/"), "ready plan output hint should point at MSI bundle output");
  assert(!readyJSON.includes(tempRoot), "ready installer plan leaked absolute temp path");
  assert(!readyJSON.match(/token|secret|password|credential|private/i), "ready installer plan leaked sensitive names");

  const nsisMissing = buildDesktopInstallerPlan({
    manifest: manifestPath,
    tauriConfig: activeConfigPath,
    target: "nsis",
    signToolPath: fakeSignTool,
    certFile: fakePfx,
    timestampURL: "https://timestamp.example.test",
    pfxPassEnvPresent: true
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
    pfxPassEnvPresent: true
  });
  assert(unsupported.readyToBuildInstaller === false, "unsupported target should not be ready");
  assert(unsupported.missing.includes("supported-installer-target"), "unsupported target should be reported");

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
  assert(cliPlan.readyToBuildInstaller === true, "CLI installer plan should be ready");
  assert(!JSON.stringify(cliPlan).includes(tempRoot), "CLI installer plan leaked absolute temp path");

  assert(readFileSync(fakePfx, "utf8") === "fake pfx", "fixture pfx should still exist");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop installer plan ok");
