import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.desktop-signature-verification.v1";
const artifactManifestSchema = "nexusim.client-artifacts.v1";
const artifactsRoot = join(workspaceRoot, "artifacts");

function main(argv) {
  const options = parseArgs(argv);
  const report = buildDesktopSignatureVerificationReport(options);
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
  if (options.requireValid && !report.readyForSignedDistribution) {
    const missing = report.missing.length > 0 ? report.missing.join(",") : "valid-authenticode-signature";
    throw new Error(`desktop artifact signature is not valid: ${missing}`);
  }
}

export function buildDesktopSignatureVerificationReport(options = {}) {
  const base = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    executionPolicy: executionPolicy(options)
  };
  const manifestPath = options.manifest
    ? resolve(options.manifest)
    : findLatestArtifactManifest(options.artifactsRoot ?? artifactsRoot, "windows-desktop");
  if (!manifestPath) {
    return assertLowSensitiveOutput({
      ...base,
      readyForSignedDistribution: false,
      missing: ["artifact-manifest"],
      artifactManifest: {
        present: false,
        manifestHint: "clients/artifacts/<run-id>/manifest.json"
      },
      signature: {
        checked: false,
        status: "UNAVAILABLE"
      },
      nextAction: "collect a Windows desktop artifact before verifying its signature"
    });
  }

  const manifest = readManifest(manifestPath);
  const artifact = findDesktopArtifact(manifest);
  if (!artifact) {
    return assertLowSensitiveOutput({
      ...base,
      readyForSignedDistribution: false,
      missing: ["windows-desktop-artifact"],
      artifactManifest: artifactManifestInfo(manifest, manifestPath),
      signature: {
        checked: false,
        status: "UNAVAILABLE"
      },
      nextAction: "collect a Windows desktop artifact before verifying its signature"
    });
  }

  const artifactPath = join(dirname(manifestPath), artifact.filename);
  const artifactInfo = validateArtifact(artifact, artifactPath);
  const signature = readSignatureStatus(artifactPath, options);
  const missing = [];
  if (!signature.authenticodeAvailable) {
    missing.push("windows-authenticode");
  } else if (signature.status !== "Valid") {
    missing.push("valid-authenticode-signature");
  }
  const readyForSignedDistribution = missing.length === 0;
  return assertLowSensitiveOutput({
    ...base,
    readyForSignedDistribution,
    missing,
    artifactManifest: artifactManifestInfo(manifest, manifestPath),
    artifact: artifactInfo,
    signature,
    nextAction: readyForSignedDistribution
      ? "continue installer packaging with a signed desktop artifact"
      : "sign the desktop artifact with sign:desktop-artifact, then rerun signature verification"
  });
}

function readSignatureStatus(artifactPath, options) {
  if (options.mockSignatureStatus) {
    return normalizeSignatureStatus(options.mockSignatureStatus);
  }
  if (process.platform !== "win32") {
    return {
      checked: false,
      authenticodeAvailable: false,
      platform: process.platform,
      status: "UNAVAILABLE",
      signed: false,
      trusted: false
    };
  }
  const command = `
& {
  $artifactPath = $env:NEXUSIM_DESKTOP_SIGNATURE_VERIFY_PATH
  $sig = Get-AuthenticodeSignature -LiteralPath $artifactPath
  $signer = $sig.SignerCertificate
  $timestamp = $sig.TimeStamperCertificate
  $result = [pscustomobject]@{
    status = [string]$sig.Status
    signerSubject = if ($signer) { [string]$signer.Subject } else { '' }
    signerThumbprint = if ($signer) { [string]$signer.Thumbprint } else { '' }
    timeStamperSubject = if ($timestamp) { [string]$timestamp.Subject } else { '' }
    timeStamperThumbprint = if ($timestamp) { [string]$timestamp.Thumbprint } else { '' }
  }
  $result | ConvertTo-Json -Compress
}
`;
  const raw = execFileSync("powershell.exe", [
    "-NoProfile",
    "-ExecutionPolicy",
    "Bypass",
    "-Command",
    command,
  ], {
    cwd: workspaceRoot,
    encoding: "utf8",
    env: {
      ...process.env,
      NEXUSIM_DESKTOP_SIGNATURE_VERIFY_PATH: artifactPath
    },
    windowsHide: true
  });
  return normalizeSignatureStatus(JSON.parse(raw));
}

function normalizeSignatureStatus(raw) {
  const status = stringValue(raw.status || raw.Status || "UNKNOWN");
  const signerThumbprint = normalizeThumbprint(raw.signerThumbprint || raw.SignerThumbprint);
  const timeStamperThumbprint = normalizeThumbprint(raw.timeStamperThumbprint || raw.TimeStamperThumbprint);
  const signerSubject = stringValue(raw.signerSubject || raw.SignerSubject);
  const timeStamperSubject = stringValue(raw.timeStamperSubject || raw.TimeStamperSubject);
  return {
    checked: status !== "UNAVAILABLE",
    authenticodeAvailable: status !== "UNAVAILABLE",
    platform: process.platform,
    status,
    signed: Boolean(signerSubject || signerThumbprint),
    trusted: status === "Valid",
    signer: publicCertificateInfo(signerSubject, signerThumbprint),
    timeStamper: publicCertificateInfo(timeStamperSubject, timeStamperThumbprint)
  };
}

function publicCertificateInfo(subject, thumbprint) {
  if (!subject && !thumbprint) {
    return {
      present: false
    };
  }
  return {
    present: true,
    subject,
    thumbprintPrefix: thumbprint.slice(0, 8),
    thumbprintSuffix: thumbprint.slice(-8)
  };
}

function executionPolicy(options) {
  return {
    readOnly: true,
    requireValidSignature: Boolean(options.requireValid),
    signsArtifacts: false,
    installsArtifacts: false,
    launchesDesktopArtifacts: false,
    startsServices: false,
    downloadsToolchain: false,
    readsCollectedArtifactManifest: true,
    validatesArtifactHashes: true,
    readsAuthenticodeSignature: true
  };
}

function validateArtifact(artifact, artifactPath) {
  if (typeof artifact.filename !== "string" || artifact.filename.length === 0) {
    throw new Error("artifact filename missing");
  }
  if (artifact.filename.includes("/") || artifact.filename.includes("\\") || isAbsolute(artifact.filename)) {
    throw new Error(`artifact filename is not relative-safe: ${artifact.filename}`);
  }
  if (!Number.isInteger(artifact.bytes) || artifact.bytes < 0) {
    throw new Error(`artifact byte size invalid: ${artifact.filename}`);
  }
  if (typeof artifact.sha256 !== "string" || !artifact.sha256.match(/^[a-f0-9]{64}$/)) {
    throw new Error(`artifact sha256 invalid: ${artifact.filename}`);
  }
  if (!existsSync(artifactPath) || !statSync(artifactPath).isFile()) {
    throw new Error(`artifact file missing: ${artifact.filename}`);
  }
  const bytes = readFileSync(artifactPath);
  if (bytes.length !== artifact.bytes) {
    throw new Error(`artifact byte size mismatch: ${artifact.filename}`);
  }
  const sha256 = sha256Buffer(bytes);
  if (sha256 !== artifact.sha256) {
    throw new Error(`artifact hash mismatch: ${artifact.filename}`);
  }
  return {
    filename: artifact.filename,
    bytes: artifact.bytes,
    sha256: artifact.sha256,
    artifactHint: safeHint(artifactPath)
  };
}

function readManifest(manifestPath) {
  const raw = readFileSync(manifestPath, "utf8");
  assertLowSensitiveText(raw, "artifact manifest");
  const manifest = JSON.parse(raw);
  if (manifest.schemaVersion !== artifactManifestSchema) {
    throw new Error("artifact manifest schema mismatch");
  }
  if (!Array.isArray(manifest.artifacts)) {
    throw new Error("artifact manifest artifacts missing");
  }
  return manifest;
}

function artifactManifestInfo(manifest, manifestPath) {
  return {
    present: true,
    manifestHint: safeHint(manifestPath),
    runId: stringValue(manifest.runId),
    gitCommit: stringValue(manifest.gitCommit)
  };
}

function findDesktopArtifact(manifest) {
  return manifest.artifacts.find(artifact => artifact?.target === "windows-desktop");
}

function findLatestArtifactManifest(root, target) {
  if (!existsSync(root)) {
    return "";
  }
  const candidates = [];
  collectManifestCandidates(root, candidates);
  candidates.sort((left, right) => right.mtimeMs - left.mtimeMs);
  for (const candidate of candidates) {
    if (manifestContainsTarget(candidate.path, target)) {
      return candidate.path;
    }
  }
  return "";
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

function manifestContainsTarget(manifestPath, target) {
  const manifest = readManifest(manifestPath);
  return manifest.artifacts.some(artifact => artifact?.target === target);
}

function parseArgs(argv) {
  const options = {
    manifest: "",
    requireValid: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--manifest") {
      options.manifest = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--require-valid") {
      options.requireValid = true;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
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

function safeHint(path) {
  const relativePath = relative(workspaceRoot, resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(resolve(path)).slice(0, 12)}`;
  }
  return `clients/${relativePath}`;
}

function assertLowSensitiveText(text, label) {
  if (text.match(/[A-Za-z]:\\\\/) || text.includes("\\\\?")) {
    throw new Error(`${label} leaked a local absolute path`);
  }
  if (text.match(/(token|secret|password|credential|private)/i)) {
    throw new Error(`${label} contains a sensitive field name`);
  }
}

function assertLowSensitiveOutput(output) {
  const serialized = JSON.stringify(output);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("desktop signature verification output leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop signature verification output leaked a sensitive field name");
  }
  return output;
}

function normalizeThumbprint(value) {
  return stringValue(value).replace(/\s+/g, "").toUpperCase();
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
