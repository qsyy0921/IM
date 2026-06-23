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
const androidExtensions = new Set([".apk"]);
const targetNames = new Set(["android", "windows-desktop", "all"]);
const artifactKindNames = new Set(["android-debug-apk", "desktop-executable", "desktop-installer"]);

function main(argv) {
  const options = parseArgs(argv);
  const plan = collectPlan(options);
  if (options.dryRun) {
    console.log(JSON.stringify({
      ...plan,
      executionPolicy: dryRunExecutionPolicy()
    }, null, 2));
    return;
  }
  if (plan.sources.length === 0) {
    console.log(JSON.stringify(plan, null, 2));
    throw new Error("client artifact source was not found");
  }

  const result = writeArtifactBundle(plan, options);
  console.log(JSON.stringify(result, null, 2));
}

function dryRunExecutionPolicy() {
  return {
    planOnly: true,
    discoversArtifactSources: true,
    readsArtifactMetadata: true,
    readsArtifactBytes: false,
    copiesArtifacts: false,
    createsOutputDirectory: false,
    writesManifest: false,
    executesGit: false,
    installsArtifacts: false,
    contactsDevice: false,
    downloadsToolchain: false
  };
}

export function parseArgs(argv) {
  const options = {
    target: "all",
    source: "",
    artifactKind: "",
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
    if (arg === "--artifact-kind") {
      options.artifactKind = requiredValue(argv, index, arg);
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
  if (options.artifactKind && options.target === "all") {
    throw new Error("--artifact-kind requires --target android or --target windows-desktop");
  }
  if (options.artifactKind && !artifactKindNames.has(options.artifactKind)) {
    throw new Error(`unsupported artifact kind: ${options.artifactKind}`);
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
      ? explicitSourcesForTarget(target, options.source)
      : defaultSourcesForTarget(target);
    const existing = targetSources.filter(source => existsSync(source) && statSync(source).isFile());
    if (existing.length === 0) {
      missing.push({
        target,
        expected: targetSources.map(source => safeRelativeSource(source))
      });
      continue;
    }
    const entries = existing.map(source => sourceEntryFor(target, source, sources));
    const selectedEntries = options.artifactKind
      ? entries.filter(entry => entry.artifactKind === options.artifactKind)
      : entries;
    if (selectedEntries.length === 0) {
      missing.push({
        target,
        artifactKind: options.artifactKind,
        expected: targetSources.map(source => safeRelativeSource(source)),
        discoveredArtifactKinds: Array.from(new Set(entries.map(entry => entry.artifactKind))).sort()
      });
      continue;
    }
    for (const selected of selectedEntries) {
      const sourceEntry = sourceEntryFor(target, selected.source, sources);
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

function sourceEntryFor(target, source, existingSources) {
  const entry = {
    target,
    sourceHint: safeRelativeSource(source),
    artifactKind: artifactKindForSource(target, source),
    outputFilename: uniqueArtifactFilename(target, source, existingSources)
  };
  Object.defineProperty(entry, "source", {
    value: source,
    enumerable: false
  });
  return entry;
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
      artifactKind: source.artifactKind,
      filename: source.outputFilename,
      bytes,
      sha256: sha256File(outputPath),
      sourcePathHash: sha256Text(sourcePath),
      sourceHint: source.sourceHint
    };
  });
  const supportFiles = writeSupportFiles(outputDir, artifacts);

  const manifest = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    gitCommit: currentGitCommit(),
    runId: options.runId,
    artifacts,
    supportFiles
  };
  const manifestPath = join(outputDir, "manifest.json");
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, "utf8");

  return {
    schemaVersion,
    runId: options.runId,
    outputDirHint: safeRelativeSource(outputDir),
    manifest: "manifest.json",
    artifacts,
    supportFiles
  };
}

function writeSupportFiles(outputDir, artifacts) {
  const supportFiles = [];
  const desktopArtifacts = artifacts.filter(artifact => artifact.target === "windows-desktop");
  if (desktopArtifacts.length === 0) {
    return supportFiles;
  }

  const readme = desktopReadme(desktopArtifacts);
  writeSupportFile(outputDir, "README-windows-desktop.txt", readme, supportFiles);

  const desktopExe = desktopArtifacts.find(artifact => extname(artifact.filename).toLowerCase() === ".exe");
  if (desktopExe) {
    writeSupportFile(outputDir, "launch-nexusim-windows.ps1", desktopLauncher(desktopExe.filename), supportFiles);
  }
  return supportFiles;
}

function writeSupportFile(outputDir, filename, body, supportFiles) {
  const outputPath = join(outputDir, filename);
  writeFileSync(outputPath, body, "utf8");
  const bytes = Buffer.byteLength(body, "utf8");
  supportFiles.push({
    target: "windows-desktop",
    filename,
    bytes,
    sha256: sha256Text(body)
  });
}

function desktopReadme(artifacts) {
  const filenames = artifacts.map(artifact => `- ${artifact.filename}`).join("\n");
  const executableArtifacts = artifacts.filter(artifact => artifact.artifactKind === "desktop-executable");
  const installerArtifacts = artifacts.filter(artifact => artifact.artifactKind === "desktop-installer");
  const hasExe = executableArtifacts.some(artifact => extname(artifact.filename).toLowerCase() === ".exe");
  const launchLine = hasExe
    ? "Run .\\launch-nexusim-windows.ps1 from this directory, or start the exe directly."
    : "Use the installer artifact according to the signed installer plan before running NexusIM.";
  const installerLine = installerArtifacts.length > 0
    ? "Installer artifacts must be signed and verified before distribution."
    : "No installer artifact is included in this package.";
  return [
    "NexusIM Windows desktop local package",
    "",
    "This directory is a local development package, not a signed production installer.",
    "It contains the collected Windows desktop artifact plus low-sensitive hashes in manifest.json.",
    "",
    "Artifacts:",
    filenames,
    "",
    "How to run:",
    launchLine,
    installerLine,
    "",
    "Before login, start the local NexusIM backend and Web client support services as documented in clients/README.md.",
    "The desktop shell talks only to api-gateway BFF and push-gateway.",
    ""
  ].join("\n");
}

function desktopLauncher(exeFilename) {
  const escaped = exeFilename.replaceAll("'", "''");
  return [
    "$ErrorActionPreference = 'Stop'",
    `$exe = Join-Path -Path $PSScriptRoot -ChildPath '${escaped}'`,
    "if (-not (Test-Path -LiteralPath $exe)) {",
    "    throw 'NexusIM desktop executable is missing from this package.'",
    "}",
    "Start-Process -FilePath $exe",
    ""
  ].join("\r\n");
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
    walkArtifacts(desktopBundleRoot, "windows-desktop", found);
  }
  if (existsSync(desktopStandaloneExe) && statSync(desktopStandaloneExe).isFile()) {
    found.push(desktopStandaloneExe);
  }
  if (found.length === 0) {
    return [desktopBundleRoot, desktopStandaloneExe];
  }
  return found.sort((left, right) => left.localeCompare(right));
}

function explicitSourcesForTarget(target, source) {
  const resolved = resolve(source);
  if (!existsSync(resolved)) {
    return [resolved];
  }
  const stats = statSync(resolved);
  if (stats.isFile()) {
    return [resolved];
  }
  if (!stats.isDirectory()) {
    return [resolved];
  }
  const found = [];
  walkArtifacts(resolved, target, found);
  return found.length > 0 ? found.sort((left, right) => left.localeCompare(right)) : [resolved];
}

function walkArtifacts(dir, target, found) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      walkArtifacts(fullPath, target, found);
      continue;
    }
    if (isArtifactFile(target, entry.name)) {
      found.push(fullPath);
    }
  }
}

function isArtifactFile(target, filename) {
  const ext = extname(filename).toLowerCase();
  if (target === "android") {
    return androidExtensions.has(ext);
  }
  if (target === "windows-desktop") {
    return desktopExtensions.has(ext);
  }
  return false;
}

function artifactFilename(target, source) {
  if (target === "android") {
    return "nexusim-android-debug.apk";
  }
  const ext = extname(source).toLowerCase() || ".artifact";
  if (artifactKindForSource(target, source) === "desktop-installer") {
    return `nexusim-windows-desktop-installer${ext}`;
  }
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

function artifactKindForSource(target, source) {
  if (target === "android") {
    return "android-debug-apk";
  }
  if (target !== "windows-desktop") {
    return "unknown";
  }
  const ext = extname(source).toLowerCase();
  const normalized = resolve(source).toLowerCase().replaceAll("\\", "/");
  if (ext === ".msi" || ext === ".msix" || normalized.includes("/bundle/")) {
    return "desktop-installer";
  }
  if (ext === ".exe") {
    return "desktop-executable";
  }
  return "desktop-installer";
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
