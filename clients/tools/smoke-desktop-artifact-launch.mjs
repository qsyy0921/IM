import { createHash } from "node:crypto";
import { spawn, spawnSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.desktop-artifact-launch-smoke.v1";
const artifactManifestSchema = "nexusim.client-artifacts.v1";
const artifactsRoot = join(workspaceRoot, "artifacts");

async function main(argv) {
  const options = parseArgs(argv);
  const manifestPath = options.manifest
    ? resolve(options.manifest)
    : findLatestArtifactManifest(artifactsRoot);
  if (!manifestPath) {
    throw new Error("desktop artifact manifest was not found");
  }

  const manifest = readManifest(manifestPath);
  const manifestDir = dirname(manifestPath);
  const artifact = findDesktopArtifact(manifest);
  const artifactPath = join(manifestDir, artifact.filename);
  validateArtifactFile(artifact, artifactPath);

  const base = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    dryRun: options.dryRun,
    manifestHint: safeHint(manifestPath),
    artifact: {
      filename: artifact.filename,
      bytes: artifact.bytes,
      sha256: artifact.sha256,
      artifactHint: safeHint(artifactPath)
    },
    holdMs: options.holdMs
  };
  assertLowSensitive(base);

  if (options.dryRun) {
    process.stdout.write(`${JSON.stringify({
      ...base,
      executionPolicy: dryRunExecutionPolicy(),
      launched: false
    }, null, 2)}\n`);
    return;
  }

  if (process.platform !== "win32") {
    throw new Error("desktop artifact launch smoke is supported on Windows only");
  }

  const launched = await launchAndTerminate(artifactPath, options.holdMs);
  const result = {
    ...base,
    launched: true,
    processStarted: true,
    aliveMs: options.holdMs,
    terminated: launched.terminated
  };
  assertLowSensitive(result);
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
}

function dryRunExecutionPolicy() {
  return {
    planOnly: true,
    readsManifest: true,
    validatesArtifactFile: true,
    readsArtifactBytes: true,
    startsArtifact: false,
    terminatesArtifact: false,
    startsServices: false,
    opensNetworkConnection: false,
    installsArtifacts: false,
    contactsDevice: false,
    downloadsToolchain: false
  };
}

function parseArgs(argv) {
  const options = {
    manifest: "",
    holdMs: 5000,
    dryRun: false
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
    if (arg === "--hold-ms") {
      const value = Number.parseInt(requiredValue(argv, index, arg), 10);
      if (!Number.isInteger(value) || value < 1000 || value > 30000) {
        throw new Error("--hold-ms must be between 1000 and 30000");
      }
      options.holdMs = value;
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return options;
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
  const artifact = manifest.artifacts.find(entry => entry?.target === "windows-desktop");
  if (!artifact) {
    throw new Error("desktop artifact is missing from manifest");
  }
  return artifact;
}

function validateArtifactFile(artifact, artifactPath) {
  if (typeof artifact.filename !== "string" || artifact.filename.length === 0) {
    throw new Error("artifact filename missing");
  }
  if (artifact.filename.includes("/") || artifact.filename.includes("\\") || isAbsolute(artifact.filename)) {
    throw new Error("artifact filename is not relative-safe");
  }
  if (!artifact.filename.toLowerCase().endsWith(".exe")) {
    throw new Error("desktop launch smoke requires a Windows exe artifact");
  }
  if (!Number.isInteger(artifact.bytes) || artifact.bytes <= 0) {
    throw new Error("artifact byte size invalid");
  }
  if (typeof artifact.sha256 !== "string" || !artifact.sha256.match(/^[a-f0-9]{64}$/)) {
    throw new Error("artifact sha256 invalid");
  }
  if (!existsSync(artifactPath) || !statSync(artifactPath).isFile()) {
    throw new Error("artifact file missing");
  }
  const bytes = readFileSync(artifactPath);
  const sha256 = createHash("sha256").update(bytes).digest("hex");
  if (bytes.length !== artifact.bytes || sha256 !== artifact.sha256) {
    throw new Error("artifact manifest does not match artifact bytes");
  }
}

async function launchAndTerminate(artifactPath, holdMs) {
  const child = spawn(artifactPath, [], {
    cwd: dirname(artifactPath),
    stdio: "ignore",
    windowsHide: false
  });
  let exited = false;
  let spawnFailed = false;

  child.once("error", () => {
    spawnFailed = true;
  });
  child.once("exit", () => {
    exited = true;
  });

  await sleep(holdMs);
  if (spawnFailed || !child.pid) {
    throw new Error("desktop artifact process did not start");
  }
  if (exited) {
    throw new Error("desktop artifact exited before smoke hold time");
  }

  const terminated = terminateProcess(child.pid);
  await sleep(500);
  return { terminated };
}

function terminateProcess(pid) {
  if (process.platform === "win32") {
    const killed = spawnSync("taskkill", ["/PID", String(pid), "/T", "/F"], {
      encoding: "utf8",
      stdio: "ignore",
      windowsHide: true
    });
    return killed.status === 0;
  }
  try {
    process.kill(pid, "SIGTERM");
    return true;
  } catch {
    return false;
  }
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
    if (entry.isDirectory()) {
      collectManifestCandidates(fullPath, candidates);
      continue;
    }
    if (entry.isFile() && entry.name === "manifest.json") {
      candidates.push({
        path: fullPath,
        mtimeMs: statSync(fullPath).mtimeMs
      });
    }
  }
}

function safeHint(path) {
  const relativePath = relative(workspaceRoot, resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(resolve(path)).slice(0, 12)}`;
  }
  return `clients/${relativePath}`;
}

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("desktop launch smoke leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop launch smoke leaked a sensitive field name");
  }
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
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
  main(process.argv.slice(2)).catch(error => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  });
}
