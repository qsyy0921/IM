import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { prepareShellWebAssets } from "./prepare-shell-web-assets.mjs";
import { verifyShellAssets } from "./verify-shell-assets.mjs";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));
const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-android-shell-actions-"));

try {
  const webDist = join(tempRoot, "web-dist");
  const androidAssets = join(tempRoot, "android-assets");
  buildWebBundle(webDist);
  prepareShellWebAssets({
    target: "android",
    sourceDir: webDist,
    outputDir: androidAssets,
    configPath: "android/shell-config.example.json",
    build: false
  });

  const verification = verifyShellAssets({ target: "android", outputDir: androidAssets });
  assert(verification.fileCount > 0, "android shell asset verification returned no files");

  const shellConfig = readFileSync(join(androidAssets, "nexusim-shell-config.js"), "utf8");
  assert(shellConfig.includes('"target": "android"'), "android shell assets must render android target config");

  const bundleText = readJavascriptBundle(androidAssets);
  for (const selector of [
    "login-submit",
    "logout-submit",
    "refresh-session",
    "restore-session",
    "native-store-readiness",
    "runtime-status",
    "push-status",
    "ack-status"
  ]) {
    assert(bundleText.includes(selector), `android shell bundle missing selector ${selector}`);
  }
  for (const action of ["login", "refresh", "restoreSession", "logout"]) {
    assert(bundleText.includes(action), `android shell bundle missing lifecycle action ${action}`);
  }
  assert(bundleText.includes("pullInbox"), "android shell bundle must keep PullInbox as the display fact source");

  const serializedManifest = readFileSync(join(androidAssets, "nexusim-shell-assets-manifest.json"), "utf8");
  for (const path of ["manifest.webmanifest", "nexusim-sw.js", "pwa-icon.svg"]) {
    assert(serializedManifest.includes(`"path": "${path}"`), `android shell asset manifest missing ${path}`);
    assert(existsSync(join(androidAssets, path)), `android shell assets missing ${path}`);
  }
  assert(!serializedManifest.match(/[A-Z]:\\\\/), "android shell asset manifest leaked a Windows absolute path");
  assert(!serializedManifest.includes("\\\\?"), "android shell asset manifest leaked an extended Windows path");

  console.log("android shell action assets ok");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

function buildWebBundle(outDir) {
  const npmExecPath = process.env.npm_execpath;
  if (npmExecPath) {
    execFileSync(process.execPath, [
      npmExecPath,
      "--prefix",
      clientsRoot,
      "--workspace",
      "@nexusim/web",
      "run",
      "build",
      "--",
      "--outDir",
      outDir,
      "--emptyOutDir"
    ], {
      cwd: clientsRoot,
      stdio: "inherit"
    });
    return;
  }

  const npm = process.platform === "win32" ? "npm.cmd" : "npm";
  execFileSync(npm, [
    "--prefix",
    clientsRoot,
    "--workspace",
    "@nexusim/web",
    "run",
    "build",
    "--",
    "--outDir",
    outDir,
    "--emptyOutDir"
  ], {
    cwd: clientsRoot,
    stdio: "inherit",
    shell: process.platform === "win32"
  });
}

function readJavascriptBundle(rootDir) {
  const files = listFiles(rootDir).filter(path => path.endsWith(".js"));
  assert(files.length > 0, "android shell bundle has no JavaScript files");
  return files.map(path => readFileSync(path, "utf8")).join("\n");
}

function listFiles(dir) {
  if (!existsSync(dir)) {
    return [];
  }
  const files = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...listFiles(fullPath));
      continue;
    }
    if (entry.isFile()) {
      files.push(fullPath);
    }
  }
  return files.sort();
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}
