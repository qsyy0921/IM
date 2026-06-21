import { existsSync, readFileSync, readdirSync } from "node:fs";
import { createHash } from "node:crypto";
import { isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));
const shellAssetsManifestFilename = "nexusim-shell-assets-manifest.json";
const manifestSchemaVersion = "nexusim.shell-assets.v1";

const targetOutputDirs = {
  "windows-desktop": "web/dist",
  android: "android/native/app/src/main/assets/nexusim"
};

export function verifyShellAssets(options) {
  const target = options.target;
  const outputDir = resolve(clientsRoot, options.outputDir ?? targetOutputDir(target));
  const manifestPath = join(outputDir, shellAssetsManifestFilename);
  if (!existsSync(manifestPath)) {
    throw new Error(`missing shell asset manifest for ${target}`);
  }

  const rawManifest = readFileSync(manifestPath, "utf8");
  assertLowSensitiveManifest(rawManifest, target);
  const manifest = JSON.parse(rawManifest);
  if (manifest.schemaVersion !== manifestSchemaVersion) {
    throw new Error(`shell asset manifest schema mismatch for ${target}`);
  }
  if (manifest.target !== target) {
    throw new Error(`shell asset manifest target mismatch for ${target}`);
  }
  if (!Array.isArray(manifest.files) || manifest.files.length === 0) {
    throw new Error(`shell asset manifest has no files for ${target}`);
  }

  const expectedPaths = listAssetFiles(outputDir);
  const manifestPaths = manifest.files.map(file => validateManifestFile(file, target));
  if (JSON.stringify(manifestPaths.sort()) !== JSON.stringify(expectedPaths)) {
    throw new Error(`shell asset manifest file set mismatch for ${target}`);
  }

  for (const file of manifest.files) {
    const bytes = readFileSync(join(outputDir, file.path));
    const sha256 = createHash("sha256").update(bytes).digest("hex");
    if (file.bytes !== bytes.length) {
      throw new Error(`shell asset byte size mismatch for ${target}: ${file.path}`);
    }
    if (file.sha256 !== sha256) {
      throw new Error(`shell asset hash mismatch for ${target}: ${file.path}`);
    }
  }

  return {
    target,
    outputDir,
    manifestPath,
    fileCount: manifest.files.length
  };
}

function assertLowSensitiveManifest(rawManifest, target) {
  if (rawManifest.match(/[A-Za-z]:\\\\/) || rawManifest.includes("\\\\?")) {
    throw new Error(`shell asset manifest leaked local path for ${target}`);
  }
  if (rawManifest.match(/(token|secret|password|credential|private)/i)) {
    throw new Error(`shell asset manifest leaked sensitive field name for ${target}`);
  }
}

function validateManifestFile(file, target) {
  if (!file || typeof file.path !== "string" || file.path.length === 0) {
    throw new Error(`shell asset manifest file path missing for ${target}`);
  }
  if (file.path.includes("\\") || file.path.includes("..") || isAbsolute(file.path)) {
    throw new Error(`shell asset manifest file path is not relative-safe for ${target}: ${file.path}`);
  }
  if (file.path === shellAssetsManifestFilename) {
    throw new Error(`shell asset manifest must not list itself for ${target}`);
  }
  if (!Number.isInteger(file.bytes) || file.bytes < 0) {
    throw new Error(`shell asset manifest byte size invalid for ${target}: ${file.path}`);
  }
  if (typeof file.sha256 !== "string" || !file.sha256.match(/^[a-f0-9]{64}$/)) {
    throw new Error(`shell asset manifest sha256 invalid for ${target}: ${file.path}`);
  }
  return file.path;
}

function listAssetFiles(rootDir, dir = rootDir) {
  const files = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...listAssetFiles(rootDir, fullPath));
      continue;
    }
    if (!entry.isFile() || entry.name === shellAssetsManifestFilename) {
      continue;
    }
    files.push(relative(rootDir, fullPath).split(sep).join("/"));
  }
  return files.sort();
}

function targetOutputDir(target) {
  const outputDir = targetOutputDirs[target];
  if (!outputDir) {
    throw new Error("target must be windows-desktop or android");
  }
  return outputDir;
}

function parseArgs(argv) {
  const options = {
    target: "",
    outputDir: ""
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--target") {
      options.target = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--output-dir") {
      options.outputDir = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  if (!options.target) {
    throw new Error("--target is required");
  }
  if (options.target === "all" && options.outputDir) {
    throw new Error("--output-dir can only be used with a single target");
  }
  return options;
}

function requiredValue(argv, index, name) {
  const value = argv[index + 1];
  if (!value || value.startsWith("--")) {
    throw new Error(`${name} requires a value`);
  }
  return value;
}

function main(argv) {
  const options = parseArgs(argv);
  const targets = options.target === "all" ? Object.keys(targetOutputDirs) : [options.target];
  const results = targets.map(target => verifyShellAssets({
    target,
    outputDir: options.outputDir || undefined
  }));
  process.stdout.write(`${JSON.stringify({
    schemaVersion: "nexusim.shell-assets-verification.v1",
    results: results.map(result => ({
      target: result.target,
      fileCount: result.fileCount,
      outputDir: result.outputDir === resolve(clientsRoot, targetOutputDir(result.target))
        ? targetOutputDir(result.target)
        : "custom"
    }))
  }, null, 2)}\n`);
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
