import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { prepareShellWebAssets } from "./prepare-shell-web-assets.mjs";
import { verifyShellAssets } from "./verify-shell-assets.mjs";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));
const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-shell-actions-"));

try {
  const webDist = join(tempRoot, "web-dist");
  const desktopAssets = join(tempRoot, "desktop-assets");
  buildWebBundle(webDist);
  prepareShellWebAssets({
    target: "windows-desktop",
    sourceDir: webDist,
    outputDir: desktopAssets,
    configPath: "desktop/shell-config.example.json",
    build: false
  });

  const verification = verifyShellAssets({ target: "windows-desktop", outputDir: desktopAssets });
  assert(verification.fileCount > 0, "desktop shell asset verification returned no files");

  const shellConfig = readFileSync(join(desktopAssets, "nexusim-shell-config.js"), "utf8");
  assert(shellConfig.includes('"target": "windows-desktop"'), "desktop shell assets must render desktop target config");

  const bundleText = readJavascriptBundle(desktopAssets);
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
    assert(bundleText.includes(selector), `desktop shell bundle missing selector ${selector}`);
  }
  for (const action of ["login", "refresh", "restoreSession", "logout"]) {
    assert(bundleText.includes(action), `desktop shell bundle missing lifecycle action ${action}`);
  }
  assert(bundleText.includes("PullInbox"), "desktop shell bundle must keep PullInbox as the display fact source");

  const serializedManifest = readFileSync(join(desktopAssets, "nexusim-shell-assets-manifest.json"), "utf8");
  for (const path of ["manifest.webmanifest", "nexusim-sw.js", "pwa-icon.svg"]) {
    assert(serializedManifest.includes(`"path": "${path}"`), `desktop shell asset manifest missing ${path}`);
    assert(existsSync(join(desktopAssets, path)), `desktop shell assets missing ${path}`);
  }
  assert(!serializedManifest.match(/[A-Z]:\\\\/), "desktop shell asset manifest leaked a Windows absolute path");
  assert(!serializedManifest.includes("\\\\?"), "desktop shell asset manifest leaked an extended Windows path");

  console.log("desktop shell action assets ok");
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
  assert(files.length > 0, "desktop shell bundle has no JavaScript files");
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
