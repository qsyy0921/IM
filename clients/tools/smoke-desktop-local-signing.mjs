import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  copyFileSync,
  existsSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, extname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.desktop-local-signing-smoke.v1";
const artifactManifestSchema = "nexusim.client-artifacts.v1";
const artifactsRoot = join(workspaceRoot, "artifacts");
const repoRoot = dirname(workspaceRoot);
const defaultDesktopArtifactKind = "desktop-executable";
const desktopArtifactKinds = new Set(["desktop-executable", "desktop-installer"]);
const signableExtensions = new Set([".exe", ".msi", ".msix"]);
const defaultSignerSubject = "CN=NexusIM Local Development Signing Smoke";

function main(argv) {
  const options = parseArgs(argv);
  const report = runDesktopLocalSigningSmoke(options);
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
  if (options.requireValid && !report.validSignedArtifactCopy) {
    const missing = report.missing.length > 0 ? report.missing.join(",") : "valid-local-signed-artifact-copy";
    throw new Error(`desktop local signing smoke did not produce a valid signed artifact copy: ${missing}`);
  }
}

export function runDesktopLocalSigningSmoke(options = {}) {
  const plan = buildDesktopLocalSigningSmokePlan(options);
  if (!options.execute) {
    return stripInternal(plan);
  }
  if (!plan.readyToExecute) {
    return {
      ...stripInternal(plan),
      execution: null,
      validSignedArtifactCopy: false,
      nextAction: "resolve local signing smoke readiness before running with --execute"
    };
  }

  const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-local-signing-smoke-"));
  try {
    const artifactCopy = join(tempRoot, plan.artifact.filename);
    copyFileSync(plan.internal.artifactPath, artifactCopy);
    const copyHashBefore = sha256Buffer(readFileSync(artifactCopy));
    if (copyHashBefore !== plan.artifact.sha256) {
      throw new Error("desktop local signing smoke artifact copy hash mismatch before signing");
    }
    const signing = signTemporaryArtifactCopy({
      artifactCopy,
      tempRoot,
      signerSubject: options.signerSubject || defaultSignerSubject
    });
    const validSignedArtifactCopy = signing.verifyStatus === "Valid";
    const output = {
      ...stripInternal(plan),
      validSignedArtifactCopy,
      execution: {
        attempted: true,
        signedTemporaryArtifactCopy: true,
        mutatesCollectedArtifact: false,
        temporaryFilesRemoved: true,
        setStatus: signing.setStatus,
        verifyStatus: signing.verifyStatus,
        signer: {
          subject: signing.signerSubject,
          thumbprintPrefix: signing.signerThumbprint.slice(0, 8),
          thumbprintSuffix: signing.signerThumbprint.slice(-8)
        },
        cleanup: {
          currentUserMyRemoved: signing.currentUserMyRemoved,
          currentUserRootRemoved: signing.currentUserRootRemoved
        }
      },
      missing: validSignedArtifactCopy ? [] : ["valid-local-signed-artifact-copy"],
      nextAction: validSignedArtifactCopy
        ? "use real release signing inputs with sign:desktop-artifact, then verify the collected artifact"
        : "inspect local Authenticode signing status before release signing"
    };
    assertLowSensitiveOutput(output);
    return output;
  } finally {
    rmSync(tempRoot, { recursive: true, force: true });
  }
}

export function buildDesktopLocalSigningSmokePlan(options = {}) {
  const artifactKind = normalizeArtifactKind(options.artifactKind);
  const artifactKindSupported = desktopArtifactKinds.has(artifactKind);
  const manifestPath = options.manifest
    ? resolveInputPath(options.manifest)
    : findLatestArtifactManifest(options.artifactsRoot ?? artifactsRoot, "windows-desktop", artifactKind);
  const execute = Boolean(options.execute);
  const allowLocalTrustStore = Boolean(options.allowLocalTrustStore);
  const missing = [];
  if (process.platform !== "win32") {
    missing.push("windows-host");
  }
  if (execute && !allowLocalTrustStore) {
    missing.push("allow-local-trust-store");
  }
  if (!artifactKindSupported) {
    missing.push("desktop-artifact-kind");
  }
  const base = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    artifactKind,
    readyToExecute: false,
    validSignedArtifactCopy: false,
    missing,
    executionPolicy: executionPolicy({ execute, allowLocalTrustStore, requireValid: options.requireValid }),
    signer: {
      subject: options.signerSubject || defaultSignerSubject,
      scope: "temporary-current-user-development-certificate"
    }
  };

  if (!manifestPath) {
    return {
      ...base,
      missing: unique([...missing, "artifact-manifest"]),
      artifactManifest: {
        present: false,
        manifestHint: "clients/artifacts/<run-id>/manifest.json"
      },
      artifact: null,
      internal: {
        artifactPath: ""
      },
      nextAction: "collect a Windows desktop artifact before running local signing smoke"
    };
  }

  const manifest = readManifest(manifestPath);
  const artifact = artifactKindSupported ? findDesktopArtifact(manifest, artifactKind) : null;
  if (!artifact) {
    return {
      ...base,
      missing: unique([...missing, desktopArtifactMissingReason(manifest, artifactKind, artifactKindSupported)]),
      artifactManifest: artifactManifestInfo(manifest, manifestPath),
      artifact: null,
      internal: {
        artifactPath: ""
      },
      nextAction: "collect the requested Windows desktop artifact kind before running local signing smoke"
    };
  }

  const artifactPath = join(dirname(manifestPath), artifact.filename);
  const artifactInfo = validateArtifact(artifact, artifactPath);
  if (!signableExtensions.has(extname(artifact.filename).toLowerCase())) {
    missing.push("signable-windows-artifact");
  }
  const readyToExecute = missing.length === 0;
  const output = {
    ...base,
    readyToExecute,
    missing: unique(missing),
    artifactManifest: artifactManifestInfo(manifest, manifestPath),
    artifact: artifactInfo,
    internal: {
      artifactPath
    },
    nextAction: readyToExecute
      ? "rerun with --execute --allow-local-trust-store to sign and verify a temporary artifact copy"
      : "resolve local signing smoke readiness before running with --execute"
  };
  assertLowSensitiveOutput(stripInternal(output));
  return output;
}

function signTemporaryArtifactCopy({ artifactCopy, tempRoot, signerSubject }) {
  const certPath = join(tempRoot, "nexusim-local-signing-smoke.cer");
  const script = [
    "$ErrorActionPreference = 'Stop'",
    "$target = $env:NEXUSIM_DESKTOP_LOCAL_SIGN_TARGET",
    "$subject = $env:NEXUSIM_DESKTOP_LOCAL_SIGN_SUBJECT",
    "$certPath = $env:NEXUSIM_DESKTOP_LOCAL_SIGN_CERT",
    "$cert = $null",
    "$rootRemoved = $false",
    "$myRemoved = $false",
    "$result = $null",
    "try {",
    "  $cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject $subject -CertStoreLocation 'Cert:\\CurrentUser\\My' -KeyAlgorithm RSA -KeyLength 2048 -HashAlgorithm SHA256 -NotAfter (Get-Date).AddDays(1)",
    "  Export-Certificate -Cert $cert -FilePath $certPath | Out-Null",
    "  Import-Certificate -FilePath $certPath -CertStoreLocation 'Cert:\\CurrentUser\\Root' | Out-Null",
    "  $set = Set-AuthenticodeSignature -FilePath $target -Certificate $cert -HashAlgorithm SHA256",
    "  $verify = Get-AuthenticodeSignature -FilePath $target",
    "  $result = [pscustomobject]@{",
    "    setStatus = [string]$set.Status",
    "    verifyStatus = [string]$verify.Status",
    "    signerSubject = if ($verify.SignerCertificate) { [string]$verify.SignerCertificate.Subject } else { '' }",
    "    signerThumbprint = if ($verify.SignerCertificate) { [string]$verify.SignerCertificate.Thumbprint } else { '' }",
    "    currentUserRootRemoved = $false",
    "    currentUserMyRemoved = $false",
    "  }",
    "} finally {",
    "  if ($cert) {",
    "    $rootPath = 'Cert:\\CurrentUser\\Root\\' + $cert.Thumbprint",
    "    $myPath = 'Cert:\\CurrentUser\\My\\' + $cert.Thumbprint",
    "    if (Test-Path $rootPath) { Remove-Item -LiteralPath $rootPath -Force -ErrorAction SilentlyContinue; $rootRemoved = $true }",
    "    if (Test-Path $myPath) { Remove-Item -LiteralPath $myPath -Force -ErrorAction SilentlyContinue; $myRemoved = $true }",
    "  }",
    "}",
    "if ($null -eq $result) { throw 'desktop local signing smoke did not produce a signature result' }",
    "$result.currentUserRootRemoved = $rootRemoved",
    "$result.currentUserMyRemoved = $myRemoved",
    "$result | ConvertTo-Json -Compress -Depth 4"
  ].join("\n");
  const output = execFileSync("powershell", ["-NoProfile", "-Command", script], {
    encoding: "utf8",
    env: {
      ...process.env,
      NEXUSIM_DESKTOP_LOCAL_SIGN_TARGET: artifactCopy,
      NEXUSIM_DESKTOP_LOCAL_SIGN_SUBJECT: signerSubject,
      NEXUSIM_DESKTOP_LOCAL_SIGN_CERT: certPath
    },
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"]
  }).trim();
  const result = output ? JSON.parse(output) : {};
  return {
    setStatus: stringValue(result.setStatus),
    verifyStatus: stringValue(result.verifyStatus),
    signerSubject: stringValue(result.signerSubject),
    signerThumbprint: normalizeThumbprint(result.signerThumbprint),
    currentUserRootRemoved: Boolean(result.currentUserRootRemoved),
    currentUserMyRemoved: Boolean(result.currentUserMyRemoved)
  };
}

function executionPolicy({ execute, allowLocalTrustStore, requireValid }) {
  const canMutateLocalTrustStore = execute && allowLocalTrustStore;
  return {
    planOnly: !execute,
    executeRequested: execute,
    requiresExplicitExecuteFlag: true,
    requiresAllowLocalTrustStoreFlag: true,
    allowLocalTrustStoreRequested: allowLocalTrustStore,
    readsCollectedArtifactManifest: true,
    validatesArtifactHashes: true,
    copiesArtifactToTemporaryDirectory: execute,
    signsTemporaryArtifactCopy: canMutateLocalTrustStore,
    mutatesCollectedArtifact: false,
    createsTemporaryCurrentUserCodeSigningCertificate: canMutateLocalTrustStore,
    importsTemporaryCurrentUserTrustedRoot: canMutateLocalTrustStore,
    removesTemporaryCurrentUserCertificate: canMutateLocalTrustStore,
    removesTemporaryFiles: execute,
    readsAuthenticodeSignature: execute,
    requiresValidSignature: Boolean(requireValid),
    installsArtifacts: false,
    launchesDesktopArtifacts: false,
    startsServices: false,
    startsDocker: false,
    downloadsToolchain: false
  };
}

function stripInternal(report) {
  const { internal, ...publicReport } = report;
  return publicReport;
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
    artifactKind: typeof artifact.artifactKind === "string" ? artifact.artifactKind : "",
    bytes: artifact.bytes,
    sha256: artifact.sha256,
    artifactHint: safeHint(artifactPath),
    signable: signableExtensions.has(extname(artifact.filename).toLowerCase())
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

function findDesktopArtifact(manifest, artifactKind) {
  return manifest.artifacts.find(
    artifact => artifact?.target === "windows-desktop" && artifact.artifactKind === artifactKind
  );
}

function desktopArtifactMissingReason(manifest, artifactKind, artifactKindSupported) {
  if (!artifactKindSupported) {
    return "desktop-artifact-kind";
  }
  const desktopArtifacts = manifest.artifacts.filter(artifact => artifact?.target === "windows-desktop");
  if (desktopArtifacts.length === 0) {
    return "windows-desktop-artifact";
  }
  if (desktopArtifacts.some(artifact => typeof artifact.artifactKind !== "string" || artifact.artifactKind.length === 0)) {
    return "desktop-artifact-kind";
  }
  return `${artifactKind}-artifact`;
}

function findLatestArtifactManifest(root, target, artifactKind) {
  if (!existsSync(root)) {
    return "";
  }
  const candidates = [];
  collectManifestCandidates(root, candidates);
  candidates.sort((left, right) => right.mtimeMs - left.mtimeMs);
  let firstTargetManifest = "";
  for (const candidate of candidates) {
    const status = manifestTargetStatus(candidate.path, target, artifactKind);
    if (status.exact) {
      return candidate.path;
    }
    if (status.target && !firstTargetManifest) {
      firstTargetManifest = candidate.path;
    }
  }
  return firstTargetManifest;
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

function manifestTargetStatus(manifestPath, target, artifactKind) {
  const manifest = readManifest(manifestPath);
  return {
    target: manifest.artifacts.some(artifact => artifact?.target === target),
    exact: manifest.artifacts.some(artifact => artifact?.target === target && artifact.artifactKind === artifactKind)
  };
}

function parseArgs(argv) {
  const options = {
    execute: false,
    allowLocalTrustStore: false,
    requireValid: false,
    manifest: "",
    artifactKind: defaultDesktopArtifactKind,
    signerSubject: defaultSignerSubject
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--execute") {
      options.execute = true;
      continue;
    }
    if (arg === "--allow-local-trust-store") {
      options.allowLocalTrustStore = true;
      continue;
    }
    if (arg === "--require-valid") {
      options.requireValid = true;
      continue;
    }
    if (arg === "--manifest") {
      options.manifest = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--artifact-kind") {
      options.artifactKind = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--signer-subject") {
      options.signerSubject = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  if (!options.signerSubject.startsWith("CN=NexusIM ")) {
    throw new Error("desktop local signing smoke signer subject must start with CN=NexusIM ");
  }
  return options;
}

function resolveInputPath(value) {
  if (isAbsolute(value)) {
    return resolve(value);
  }
  const normalized = value.replaceAll("\\", "/");
  if (normalized === "clients" || normalized.startsWith("clients/")) {
    return resolve(repoRoot, value);
  }
  return resolve(workspaceRoot, value);
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
    throw new Error("desktop local signing smoke output leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop local signing smoke output leaked a sensitive field name");
  }
}

function sha256Buffer(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
}

function normalizeArtifactKind(value) {
  return stringValue(value) || defaultDesktopArtifactKind;
}

function normalizeThumbprint(value) {
  return stringValue(value).replace(/\s+/g, "").toUpperCase();
}

function unique(values) {
  return Array.from(new Set(values));
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

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
