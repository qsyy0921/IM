import { createHash } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  statSync,
  writeFileSync
} from "node:fs";
import { basename, dirname, extname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.desktop-bundle.v1";
const artifactManifestSchema = "nexusim.client-artifacts.v1";
const artifactsRoot = join(workspaceRoot, "artifacts");
const defaultBundleRoot = join(artifactsRoot, "desktop-bundles");
const bundleFilename = "nexusim-windows-desktop-bundle.zip";
const bundleSummaryFilename = "desktop-bundle-summary.json";
const bundleManifestFilename = "bundle-manifest.json";

function main(argv) {
  const options = parseArgs(argv);
  const plan = buildDesktopBundlePlan(options);
  if (options.dryRun) {
    process.stdout.write(`${JSON.stringify(plan, null, 2)}\n`);
    return;
  }
  const result = writeDesktopBundle(plan, options);
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
}

export function buildDesktopBundlePlan(options = {}) {
  const manifestPath = options.manifest
    ? resolve(options.manifest)
    : findLatestArtifactManifest(options.artifactsRoot ?? artifactsRoot);
  if (!manifestPath) {
    return {
      schemaVersion,
      generatedAt: new Date().toISOString(),
      executionPolicy: executionPolicy(options.dryRun ?? false),
      ready: false,
      missing: ["artifact-manifest"],
      artifactManifest: {
        present: false,
        manifestHint: "clients/artifacts/<run-id>/manifest.json"
      }
    };
  }

  const manifest = readManifest(manifestPath);
  const manifestDir = dirname(manifestPath);
  const desktopArtifact = findDesktopArtifact(manifest);
  if (!desktopArtifact) {
    return {
      schemaVersion,
      generatedAt: new Date().toISOString(),
      executionPolicy: executionPolicy(options.dryRun ?? false),
      ready: false,
      missing: ["windows-desktop-artifact"],
      artifactManifest: {
        present: true,
        manifestHint: safeHint(manifestPath),
        runId: stringValue(manifest.runId)
      }
    };
  }

  const artifactFile = validatePackageFile(desktopArtifact, join(manifestDir, desktopArtifact.filename), "artifact");
  const supportFiles = supportFilesForDesktop(manifest, manifestDir, desktopArtifact);
  const files = [
    artifactFile,
    ...supportFiles,
    packageFile("manifest.json", join(manifestDir, "manifest.json"))
  ];
  const outputDir = resolve(options.outputDir ?? defaultBundleRoot, options.runId ?? defaultRunID());
  const bundleManifest = buildBundleManifest(manifest, manifestPath, files);
  const bundleManifestBytes = Buffer.from(`${JSON.stringify(bundleManifest, null, 2)}\n`, "utf8");
  const bundleFiles = [
    ...files,
    {
      filename: bundleManifestFilename,
      bytes: bundleManifestBytes.length,
      sha256: sha256Buffer(bundleManifestBytes),
      sourcePath: "",
      sourceHint: bundleManifestFilename,
      zipPath: bundleManifestFilename,
      inlineBytes: bundleManifestBytes
    }
  ];

  const plan = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    executionPolicy: executionPolicy(options.dryRun ?? false),
    ready: true,
    missing: [],
    artifactManifest: {
      present: true,
      manifestHint: safeHint(manifestPath),
      runId: stringValue(manifest.runId)
    },
    output: {
      outputDirHint: safeHint(outputDir),
      bundle: bundleFilename,
      summary: bundleSummaryFilename
    },
    signing: signingStatus(),
    files: bundleFiles.map(file => ({
      filename: file.filename,
      bytes: file.bytes,
      sha256: file.sha256,
      packagePath: file.zipPath
    }))
  };
  assertLowSensitivePlan(plan);
  Object.defineProperty(plan, "internal", {
    value: {
      outputDir,
      bundleFiles,
      bundleManifest
    },
    enumerable: false
  });
  return plan;
}

export function writeDesktopBundle(plan, options = {}) {
  if (!plan.ready) {
    throw new Error(`desktop bundle is not ready: ${plan.missing?.join(",") || "unknown"}`);
  }
  const outputDir = plan.internal?.outputDir ?? resolve(options.outputDir ?? defaultBundleRoot, options.runId ?? defaultRunID());
  const bundleFiles = plan.internal?.bundleFiles;
  const bundleManifest = plan.internal?.bundleManifest;
  if (!Array.isArray(bundleFiles) || !bundleManifest) {
    throw new Error("desktop bundle plan is missing internal file state");
  }

  mkdirSync(outputDir, { recursive: true });
  const bundlePath = join(outputDir, bundleFilename);
  const zipBytes = createZip(bundleFiles);
  writeFileSync(bundlePath, zipBytes);
  const summary = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    artifactManifest: plan.artifactManifest,
    signing: plan.signing,
    bundle: {
      filename: bundleFilename,
      bytes: zipBytes.length,
      sha256: sha256Buffer(zipBytes),
      bundleHint: safeHint(bundlePath)
    },
    files: plan.files,
    bundleManifest
  };
  assertLowSensitivePlan(summary);
  const summaryPath = join(outputDir, bundleSummaryFilename);
  writeFileSync(summaryPath, `${JSON.stringify(summary, null, 2)}\n`, "utf8");
  return {
    schemaVersion,
    outputDirHint: safeHint(outputDir),
    bundle: summary.bundle,
    summary: bundleSummaryFilename,
    signing: summary.signing
  };
}

function buildBundleManifest(manifest, manifestPath, files) {
  return {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    artifactManifest: {
      manifestHint: safeHint(manifestPath),
      runId: stringValue(manifest.runId),
      gitCommit: stringValue(manifest.gitCommit)
    },
    packageType: "windows-desktop-portable-zip",
    signing: signingStatus(),
    files: files.map(file => ({
      filename: file.filename,
      packagePath: file.zipPath,
      bytes: file.bytes,
      sha256: file.sha256
    }))
  };
}

function signingStatus() {
  return {
    signed: false,
    status: "unsigned-local-dev",
    reason: "no code-signing certificate is configured for this local bundle step",
    nextStep: "use a dedicated signing pipeline before treating this as a production Windows installer"
  };
}

function executionPolicy(dryRun) {
  return {
    planOnly: Boolean(dryRun),
    createsPortableBundle: !dryRun,
    readsCollectedArtifactManifest: true,
    validatesArtifactHashes: true,
    writesBundleZip: !dryRun,
    writesBundleSummary: !dryRun,
    signsArtifacts: false,
    installsArtifacts: false,
    launchesDesktopArtifacts: false,
    startsServices: false,
    contactsDevices: false,
    downloadsToolchain: false
  };
}

function supportFilesForDesktop(manifest, manifestDir, desktopArtifact) {
  if (!Array.isArray(manifest.supportFiles)) {
    throw new Error("desktop support files missing; rerun collect:client-artifacts");
  }
  const desktopSupportFiles = manifest.supportFiles
    .filter(file => file?.target === "windows-desktop")
    .map(file => validatePackageFile(file, join(manifestDir, file.filename), "support"));
  if (!desktopSupportFiles.some(file => file.filename === "README-windows-desktop.txt")) {
    throw new Error("desktop README support file missing; rerun collect:client-artifacts");
  }
  if (extname(desktopArtifact.filename).toLowerCase() === ".exe" &&
    !desktopSupportFiles.some(file => file.filename === "launch-nexusim-windows.ps1")) {
    throw new Error("desktop launcher support file missing; rerun collect:client-artifacts");
  }
  return desktopSupportFiles;
}

function validatePackageFile(entry, path, kind) {
  if (typeof entry.filename !== "string" || entry.filename.length === 0) {
    throw new Error(`${kind} filename missing`);
  }
  if (entry.filename.includes("/") || entry.filename.includes("\\") || isAbsolute(entry.filename)) {
    throw new Error(`${kind} filename is not relative-safe: ${entry.filename}`);
  }
  if (!Number.isInteger(entry.bytes) || entry.bytes < 0) {
    throw new Error(`${kind} byte size invalid: ${entry.filename}`);
  }
  if (typeof entry.sha256 !== "string" || !entry.sha256.match(/^[a-f0-9]{64}$/)) {
    throw new Error(`${kind} sha256 invalid: ${entry.filename}`);
  }
  if (!existsSync(path) || !statSync(path).isFile()) {
    throw new Error(`${kind} file missing: ${entry.filename}`);
  }
  const bytes = readFileSync(path);
  if (bytes.length !== entry.bytes) {
    throw new Error(`${kind} byte size mismatch: ${entry.filename}`);
  }
  const sha256 = sha256Buffer(bytes);
  if (sha256 !== entry.sha256) {
    throw new Error(`${kind} hash mismatch: ${entry.filename}`);
  }
  return {
    filename: entry.filename,
    bytes: entry.bytes,
    sha256: entry.sha256,
    sourcePath: path,
    sourceHint: safeHint(path),
    zipPath: entry.filename
  };
}

function packageFile(filename, path) {
  if (!existsSync(path) || !statSync(path).isFile()) {
    throw new Error(`package file missing: ${filename}`);
  }
  const bytes = readFileSync(path);
  return {
    filename,
    bytes: bytes.length,
    sha256: sha256Buffer(bytes),
    sourcePath: path,
    sourceHint: safeHint(path),
    zipPath: filename
  };
}

function readManifest(manifestPath) {
  const raw = readFileSync(manifestPath, "utf8");
  if (raw.match(/[A-Za-z]:\\\\/) || raw.includes("\\\\?")) {
    throw new Error("artifact manifest leaked a local absolute path");
  }
  if (raw.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("artifact manifest contains a sensitive field name");
  }
  const manifest = JSON.parse(raw);
  if (manifest.schemaVersion !== artifactManifestSchema) {
    throw new Error("artifact manifest schema mismatch");
  }
  if (!Array.isArray(manifest.artifacts)) {
    throw new Error("artifact manifest artifacts missing");
  }
  return manifest;
}

function findDesktopArtifact(manifest) {
  return manifest.artifacts.find(artifact => artifact?.target === "windows-desktop");
}

function createZip(files) {
  const localParts = [];
  const centralParts = [];
  let offset = 0;
  for (const file of files) {
    const nameBytes = Buffer.from(file.zipPath, "utf8");
    const data = file.inlineBytes ?? readFileSync(file.sourcePath);
    const crc = crc32(data);
    const localHeader = Buffer.alloc(30);
    localHeader.writeUInt32LE(0x04034b50, 0);
    localHeader.writeUInt16LE(20, 4);
    localHeader.writeUInt16LE(0, 6);
    localHeader.writeUInt16LE(0, 8);
    localHeader.writeUInt16LE(0, 10);
    localHeader.writeUInt16LE(0, 12);
    localHeader.writeUInt32LE(crc, 14);
    localHeader.writeUInt32LE(data.length, 18);
    localHeader.writeUInt32LE(data.length, 22);
    localHeader.writeUInt16LE(nameBytes.length, 26);
    localHeader.writeUInt16LE(0, 28);
    localParts.push(localHeader, nameBytes, data);

    const centralHeader = Buffer.alloc(46);
    centralHeader.writeUInt32LE(0x02014b50, 0);
    centralHeader.writeUInt16LE(20, 4);
    centralHeader.writeUInt16LE(20, 6);
    centralHeader.writeUInt16LE(0, 8);
    centralHeader.writeUInt16LE(0, 10);
    centralHeader.writeUInt16LE(0, 12);
    centralHeader.writeUInt16LE(0, 14);
    centralHeader.writeUInt32LE(crc, 16);
    centralHeader.writeUInt32LE(data.length, 20);
    centralHeader.writeUInt32LE(data.length, 24);
    centralHeader.writeUInt16LE(nameBytes.length, 28);
    centralHeader.writeUInt16LE(0, 30);
    centralHeader.writeUInt16LE(0, 32);
    centralHeader.writeUInt16LE(0, 34);
    centralHeader.writeUInt16LE(0, 36);
    centralHeader.writeUInt32LE(0, 38);
    centralHeader.writeUInt32LE(offset, 42);
    centralParts.push(centralHeader, nameBytes);
    offset += localHeader.length + nameBytes.length + data.length;
  }

  const centralDirectory = Buffer.concat(centralParts);
  const localData = Buffer.concat(localParts);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(0, 4);
  end.writeUInt16LE(0, 6);
  end.writeUInt16LE(files.length, 8);
  end.writeUInt16LE(files.length, 10);
  end.writeUInt32LE(centralDirectory.length, 12);
  end.writeUInt32LE(localData.length, 16);
  end.writeUInt16LE(0, 20);
  return Buffer.concat([localData, centralDirectory, end]);
}

const crcTable = Array.from({ length: 256 }, (_, index) => {
  let crc = index;
  for (let bit = 0; bit < 8; bit += 1) {
    crc = (crc & 1) ? (0xedb88320 ^ (crc >>> 1)) : (crc >>> 1);
  }
  return crc >>> 0;
});

function crc32(buffer) {
  let crc = 0xffffffff;
  for (const byte of buffer) {
    crc = crcTable[(crc ^ byte) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function findLatestArtifactManifest(root) {
  if (!existsSync(root)) {
    return "";
  }
  const candidates = [];
  collectManifestCandidates(root, candidates);
  candidates.sort((left, right) => right.mtimeMs - left.mtimeMs);
  return candidates[0]?.path ?? "";
}

function collectManifestCandidates(dir, candidates) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (!entry.isDirectory()) {
      if (entry.isFile() && entry.name === "manifest.json") {
        candidates.push({
          path: fullPath,
          mtimeMs: statSync(fullPath).mtimeMs
        });
      }
      continue;
    }
    collectManifestCandidates(fullPath, candidates);
  }
}

function parseArgs(argv) {
  const options = {
    dryRun: false,
    manifest: "",
    outputDir: defaultBundleRoot,
    runId: defaultRunID()
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--dry-run") {
      options.dryRun = true;
      continue;
    }
    if (arg === "--manifest") {
      options.manifest = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--output-dir") {
      options.outputDir = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--run-id") {
      options.runId = sanitizeRunID(requiredValue(argv, index, arg));
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  options.outputDir = resolve(options.outputDir);
  return options;
}

function safeHint(path) {
  const relativePath = relative(workspaceRoot, resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(resolve(path)).slice(0, 12)}`;
  }
  return `clients/${relativePath}`;
}

function assertLowSensitivePlan(plan) {
  const serialized = JSON.stringify(plan);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("desktop bundle plan leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop bundle plan leaked a sensitive field name");
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

function stringValue(value) {
  return typeof value === "string" ? value : "";
}

function sha256Buffer(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
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
