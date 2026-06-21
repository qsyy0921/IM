import { createHash } from "node:crypto";
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  statSync,
  writeFileSync
} from "node:fs";
import { basename, extname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.client-artifacts.v1";
const artifactsRoot = join(workspaceRoot, "artifacts");
const androidDefaultArtifact = join(
  workspaceRoot,
  "android",
  "native",
  "app",
  "build",
  "outputs",
  "apk",
  "debug",
  "app-debug.apk"
);
const desktopBundleRoot = join(
  workspaceRoot,
  "desktop",
  "src-tauri",
  "target",
  "release",
  "bundle"
);
const desktopStandaloneExe = join(
  workspaceRoot,
  "desktop",
  "src-tauri",
  "target",
  "release",
  "nexusim-desktop.exe"
);
const desktopExtensions = new Set([".msi", ".exe", ".msix", ".dmg", ".appimage", ".deb", ".rpm"]);
const targetNames = new Set(["android", "windows-desktop", "all"]);

function main(argv) {
  const options = parseArgs(argv);
  const plan = collectPlan(options);
  if (options.dryRun) {
    console.log(JSON.stringify(plan, null, 2));
    return;
  }
  if (plan.sources.length === 0) {
    console.log(JSON.stringify(plan, null, 2));
    throw new Error("client artifact source was not found");
  }

  const result = writeArtifactBundle(plan, options);
  console.log(JSON.stringify(result, null, 2));
}

export function parseArgs(argv) {
  const options = {
    target: "all",
    source: "",
    outputDir: artifactsRoot,
    runId: defaultRunID(),
    dryRun: false
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--dry-run") {
      options.dryRun = true;
      continue;
    }
    if (arg === "--target") {
      options.target = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--source") {
      options.source = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--output-dir") {
      options.outputDir = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--run-id") {
      options.runId = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }

  if (!targetNames.has(options.target)) {
    throw new Error(`unsupported target: ${options.target}`);
  }
  if (options.source && options.target === "all") {
    throw new Error("--source requires --target android or --target windows-desktop");
  }
  options.outputDir = resolve(options.outputDir);
  options.runId = sanitizeRunID(options.runId);
  return options;
}

export function collectPlan(options) {
  const sources = [];
  const missing = [];
  const targets = options.target === "all" ? ["windows-desktop", "android"] : [options.target];

  for (const target of targets) {
    const targetSources = options.source
      ? [resolve(options.source)]
      : defaultSourcesForTarget(target);
    const existing = targetSources.filter(source => existsSync(source) && statSync(source).isFile());
    if (existing.length === 0) {
      missing.push({
        target,
        expected: targetSources.map(source => safeRelativeSource(source))
      });
      continue;
    }
    for (const source of existing) {
      const sourceEntry = {
        target,
        sourceHint: safeRelativeSource(source),
        outputFilename: uniqueArtifactFilename(target, source, sources)
      };
      Object.defineProperty(sourceEntry, "source", {
        value: source,
        enumerable: false
      });
      sources.push(sourceEntry);
    }
  }

  return {
    schemaVersion,
    runId: options.runId,
    outputDirHint: safeRelativeSource(join(options.outputDir, options.runId)),
    dryRun: options.dryRun,
    sources,
    missing
  };
}

export function writeArtifactBundle(plan, options) {
  const outputDir = join(options.outputDir, options.runId);
  mkdirSync(outputDir, { recursive: true });

  const artifacts = plan.sources.map(source => {
    const sourcePath = source.source;
    if (!sourcePath) {
      throw new Error("artifact source path is missing");
    }
    const outputPath = join(outputDir, source.outputFilename);
    copyFileSync(sourcePath, outputPath);
    const bytes = statSync(outputPath).size;
    return {
      target: source.target,
      filename: source.outputFilename,
      bytes,
      sha256: sha256File(outputPath),
      sourcePathHash: sha256Text(sourcePath),
      sourceHint: source.sourceHint
    };
  });

  const manifest = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    gitCommit: currentGitCommit(),
    runId: options.runId,
    artifacts
  };
  const manifestPath = join(outputDir, "manifest.json");
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, "utf8");

  return {
    schemaVersion,
    runId: options.runId,
    outputDirHint: safeRelativeSource(outputDir),
    manifest: "manifest.json",
    artifacts
  };
}

function defaultSourcesForTarget(target) {
  if (target === "android") {
    return [androidDefaultArtifact];
  }
  if (target === "windows-desktop") {
    return findDesktopArtifacts();
  }
  throw new Error(`unsupported target: ${target}`);
}

function findDesktopArtifacts() {
  const found = [];
  if (existsSync(desktopBundleRoot)) {
    walk(desktopBundleRoot, found);
  }
  if (existsSync(desktopStandaloneExe) && statSync(desktopStandaloneExe).isFile()) {
    found.push(desktopStandaloneExe);
  }
  if (found.length === 0) {
    return [desktopBundleRoot, desktopStandaloneExe];
  }
  return found.sort((left, right) => left.localeCompare(right));
}

function walk(dir, found) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(fullPath, found);
      continue;
    }
    const ext = extname(entry.name).toLowerCase();
    if (desktopExtensions.has(ext)) {
      found.push(fullPath);
    }
  }
}

function artifactFilename(target, source) {
  if (target === "android") {
    return "nexusim-android-debug.apk";
  }
  const ext = extname(source).toLowerCase() || ".artifact";
  return `nexusim-windows-desktop${ext}`;
}

function uniqueArtifactFilename(target, source, existingSources) {
  const filename = artifactFilename(target, source);
  const used = new Set(existingSources.map(existing => existing.outputFilename));
  if (!used.has(filename)) {
    return filename;
  }
  const ext = extname(filename);
  const stem = ext ? filename.slice(0, -ext.length) : filename;
  for (let index = 2; ; index += 1) {
    const candidate = `${stem}-${index}${ext}`;
    if (!used.has(candidate)) {
      return candidate;
    }
  }
}

function safeRelativeSource(source) {
  const relativePath = relative(workspaceRoot, resolve(source)).replaceAll("\\", "/");
  if (relativePath.startsWith("..")) {
    return `${basename(source)}#${sha256Text(resolve(source)).slice(0, 12)}`;
  }
  return relativePath;
}

function sha256File(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
}

function currentGitCommit() {
  try {
    return execFileSync("git", ["rev-parse", "HEAD"], {
      cwd: workspaceRoot,
      encoding: "utf8"
    }).trim();
  } catch {
    return "unknown";
  }
}

function defaultRunID() {
  return new Date().toISOString().replaceAll(":", "").replace(/\.\d{3}Z$/, "Z");
}

function sanitizeRunID(value) {
  const sanitized = value.replace(/[^a-zA-Z0-9._-]/g, "-");
  if (!sanitized) {
    throw new Error("run id cannot be empty");
  }
  return sanitized;
}

function requiredValue(argv, index, name) {
  const value = argv[index + 1];
  if (!value || value.startsWith("--")) {
    throw new Error(`${name} requires a value`);
  }
  return value;
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
