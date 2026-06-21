import { existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { prepareShellWebAssets } from "./prepare-shell-web-assets.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function createSourceDist(root) {
  const sourceDir = join(root, "web-dist");
  mkdirSync(join(sourceDir, "assets"), { recursive: true });
  writeFileSync(join(sourceDir, "index.html"), "<html><script src=\"/nexusim-shell-config.js\"></script></html>", "utf8");
  writeFileSync(join(sourceDir, "assets", "index.js"), "console.log('nexusim');\n", "utf8");
  writeFileSync(join(sourceDir, "nexusim-shell-config.js"), "globalThis.__NEXUSIM_CLIENT_SHELL__ = {};\n", "utf8");
  return sourceDir;
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-shell-assets-"));
try {
  const sourceDir = createSourceDist(tempRoot);

  const desktopOut = join(tempRoot, "desktop-out");
  mkdirSync(join(desktopOut, "assets"), { recursive: true });
  writeFileSync(join(desktopOut, "assets", "stale.js"), "console.log('stale');\n", "utf8");
  const desktopResult = prepareShellWebAssets({
    target: "windows-desktop",
    sourceDir,
    outputDir: desktopOut,
    configPath: "desktop/shell-config.example.json",
    build: false
  });
  const desktopConfig = readFileSync(join(desktopOut, "nexusim-shell-config.js"), "utf8");
  assert(desktopResult.outputDir === desktopOut, "desktop output path mismatch");
  assert(readFileSync(join(desktopOut, "index.html"), "utf8").includes("nexusim-shell-config.js"), "desktop index not copied");
  assert(desktopConfig.includes('"target": "windows-desktop"'), "desktop config not rendered");
  assert(!desktopConfig.match(/token|secret|password|credential|private/i), "desktop rendered config contains sensitive key");
  assert(!existsSync(join(desktopOut, "assets", "stale.js")), "desktop output must remove stale assets");

  const androidOut = join(tempRoot, "android-out");
  mkdirSync(join(androidOut, "assets"), { recursive: true });
  writeFileSync(join(androidOut, "assets", "stale.js"), "console.log('stale');\n", "utf8");
  prepareShellWebAssets({
    target: "android",
    sourceDir,
    outputDir: androidOut,
    configPath: "android/shell-config.example.json",
    build: false
  });
  const androidConfig = readFileSync(join(androidOut, "nexusim-shell-config.js"), "utf8");
  assert(readFileSync(join(androidOut, "assets", "index.js"), "utf8").includes("nexusim"), "android assets not copied");
  assert(androidConfig.includes('"target": "android"'), "android config not rendered");
  assert(!existsSync(join(androidOut, "assets", "stale.js")), "android output must remove stale assets");

  let rejected = false;
  try {
    prepareShellWebAssets({
      target: "browser",
      sourceDir,
      outputDir: join(tempRoot, "bad-out"),
      build: false
    });
  } catch {
    rejected = true;
  }
  assert(rejected, "unsupported shell asset target should be rejected");

  console.log("shell web assets ok");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}
