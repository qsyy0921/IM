import { existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { prepareShellWebAssetsIfNeeded } from "./prepare-shell-web-assets-if-needed.mjs";

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

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-shell-prep-wrapper-"));
try {
  const sourceDir = createSourceDist(tempRoot);
  const outputDir = join(tempRoot, "desktop-out");

  const prepared = prepareShellWebAssetsIfNeeded({
    target: "windows-desktop",
    sourceDir,
    outputDir,
    configPath: "desktop/shell-config.example.json",
    build: false,
    skip: false
  });
  assert(prepared.skipped === false, "wrapper should prepare assets when skip is false");
  assert(existsSync(join(outputDir, "nexusim-shell-assets-manifest.json")), "wrapper did not write manifest");

  const skipped = prepareShellWebAssetsIfNeeded({
    target: "windows-desktop",
    outputDir,
    skip: true
  });
  assert(skipped.skipped === true, "wrapper should skip preparation when requested");
  assert(skipped.verified === true, "wrapper skip path must verify existing manifest");
  assert(skipped.fileCount === 3, "wrapper skip path verifier file count mismatch");

  rmSync(join(outputDir, "nexusim-shell-assets-manifest.json"), { force: true });
  let rejectedMissingManifest = false;
  try {
    prepareShellWebAssetsIfNeeded({
      target: "windows-desktop",
      outputDir,
      skip: true
    });
  } catch {
    rejectedMissingManifest = true;
  }
  assert(rejectedMissingManifest, "wrapper skip path must reject missing manifest");

  const serialized = JSON.stringify({ prepared, skipped });
  assert(!serialized.match(/token|secret|password|credential|private/i), "wrapper result leaked sensitive field name");
  assert(!serialized.match(/[A-Z]:\\\\/), "wrapper result leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "wrapper result leaked extended Windows path");
  assert(!readFileSync(join(outputDir, "nexusim-shell-config.js"), "utf8").match(/token|secret|password|credential|private/i), "rendered config leaked sensitive field name");

  console.log("shell asset prep wrapper ok");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}
