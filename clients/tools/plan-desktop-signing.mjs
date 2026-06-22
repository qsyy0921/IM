import { createHash } from "node:crypto";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { basename, dirname, extname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.desktop-signing-plan.v1";
const artifactManifestSchema = "nexusim.client-artifacts.v1";
const artifactsRoot = join(workspaceRoot, "artifacts");
const signableExtensions = new Set([".exe", ".msi", ".msix"]);
const defaultDesktopArtifactKind = "desktop-executable";
const desktopArtifactKinds = new Set(["desktop-executable", "desktop-installer"]);

function main(argv) {
  const options = parseArgs(argv, process.env);
  const plan = buildDesktopSigningPlan(options);
  process.stdout.write(`${JSON.stringify(plan, null, 2)}\n`);
}

export function buildDesktopSigningPlan(options = {}) {
  const artifactKind = normalizeArtifactKind(options.artifactKind);
  const artifactKindSupported = desktopArtifactKinds.has(artifactKind);
  const manifestPath = options.manifest
    ? resolve(options.manifest)
    : findLatestArtifactManifest(options.artifactsRoot ?? artifactsRoot, "windows-desktop", artifactKind);
  const base = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    artifactKind,
    executionPolicy: executionPolicy()
  };
  if (!manifestPath) {
    return {
      ...base,
      readyToSign: false,
      missing: ["artifact-manifest"],
      artifactManifest: {
        present: false,
        manifestHint: "clients/artifacts/<run-id>/manifest.json"
      }
    };
  }

  const manifest = readManifest(manifestPath);
  const manifestDir = dirname(manifestPath);
  const artifact = artifactKindSupported ? findDesktopArtifact(manifest, artifactKind) : null;
  if (!artifact) {
    return {
      ...base,
      readyToSign: false,
      missing: [desktopArtifactMissingReason(manifest, artifactKind, artifactKindSupported)],
      artifactManifest: artifactManifestInfo(manifest, manifestPath)
    };
  }

  const artifactPath = join(manifestDir, artifact.filename);
  const artifactInfo = validateArtifact(artifact, artifactPath);
  const missing = [];
  if (!artifactKindSupported) {
    missing.push("desktop-artifact-kind");
  }
  if (!signableExtensions.has(extname(artifact.filename).toLowerCase())) {
    missing.push("signable-windows-artifact");
  }

  const config = signingConfig(options);
  missing.push(...config.missing);
  const readyToSign = missing.length === 0;
  const plan = {
    ...base,
    readyToSign,
    missing,
    artifactManifest: artifactManifestInfo(manifest, manifestPath),
    artifact: artifactInfo,
    signing: {
      mode: config.mode,
      signTool: config.signTool,
      certificate: config.certificate,
      timestamp: config.timestamp,
      digestAlgorithm: "SHA256"
    },
    commandTemplate: readyToSign ? commandTemplate(config, artifactInfo) : null,
    nextAction: readyToSign
      ? "run signing command in a dedicated signing profile, then collect the signed artifact"
      : "configure explicit signtool, certificate and timestamp URL before signing"
  };
  assertLowSensitivePlan(plan);
  return plan;
}

function signingConfig(options) {
  const missing = [];
  const signToolPath = stringValue(options.signToolPath);
  if (!signToolPath) {
    missing.push("signtool-path");
  } else if (!existsSync(resolve(signToolPath))) {
    missing.push("signtool-file");
  }

  const timestampURL = stringValue(options.timestampURL);
  if (!timestampURL) {
    missing.push("timestamp-url");
  } else if (!isHTTPURL(timestampURL)) {
    missing.push("timestamp-url-valid");
  }

  const certFile = stringValue(options.certFile);
  const certSHA1 = normalizeThumbprint(options.certSHA1);
  if (certFile && certSHA1) {
    missing.push("single-certificate-source");
  }
  if (!certFile && !certSHA1) {
    missing.push("certificate-source");
  }

  let mode = "none";
  let certificate = {
    source: "missing"
  };
  if (certFile) {
    mode = "pfx";
    const resolvedCert = resolve(certFile);
    if (!existsSync(resolvedCert) || !statSync(resolvedCert).isFile()) {
      missing.push("certificate-file");
    }
    if (!options.pfxPassEnvPresent) {
      missing.push("pfx-pass-env");
    }
    certificate = {
      source: "pfx-file",
      fileHint: safeHint(resolvedCert),
      pfxPassEnv: "NEXUSIM_DESKTOP_SIGN_PFX_PASS",
      pfxPassEnvPresent: Boolean(options.pfxPassEnvPresent)
    };
  } else if (certSHA1) {
    mode = "cert-store-sha1";
    if (!certSHA1.match(/^[A-F0-9]{40}$/)) {
      missing.push("certificate-sha1");
    }
    certificate = {
      source: "windows-cert-store",
      sha1Prefix: certSHA1.slice(0, 8),
      sha1Suffix: certSHA1.slice(-8)
    };
  }

  return {
    missing: unique(missing),
    mode,
    signTool: signToolPath
      ? {
          configured: true,
          toolHint: safeHint(resolve(signToolPath)),
          exists: existsSync(resolve(signToolPath))
        }
      : {
          configured: false,
          toolHint: "NEXUSIM_DESKTOP_SIGNTOOL"
        },
    certificate,
    timestamp: {
      configured: Boolean(timestampURL),
      url: timestampURL
    }
  };
}

function commandTemplate(config, artifactInfo) {
  const common = [
    "<signtool>",
    "sign",
    "/fd",
    "SHA256",
    "/tr",
    config.timestamp.url,
    "/td",
    "SHA256"
  ];
  if (config.mode === "pfx") {
    common.push("/f", "<pfx-file>", "/p", "%NEXUSIM_DESKTOP_SIGN_PFX_PASS%");
  } else if (config.mode === "cert-store-sha1") {
    common.push("/sha1", `${config.certificate.sha1Prefix}...${config.certificate.sha1Suffix}`);
  }
  common.push(artifactInfo.artifactHint);
  return common;
}

function executionPolicy() {
  return {
    planOnly: true,
    signsArtifacts: false,
    installsArtifacts: false,
    launchesDesktopArtifacts: false,
    startsServices: false,
    downloadsToolchain: false,
    readsCollectedArtifactManifest: true,
    readsSigningConfig: true,
    validatesArtifactHashes: true
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
    artifactKind: typeof artifact.artifactKind === "string" ? artifact.artifactKind : "",
    bytes: artifact.bytes,
    sha256: artifact.sha256,
    artifactHint: safeHint(artifactPath),
    signable: signableExtensions.has(extname(artifact.filename).toLowerCase())
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

function parseArgs(argv, env) {
  const options = {
    manifest: "",
    artifactKind: defaultDesktopArtifactKind,
    signToolPath: env.NEXUSIM_DESKTOP_SIGNTOOL ?? "",
    certFile: env.NEXUSIM_DESKTOP_SIGN_CERT_FILE ?? "",
    certSHA1: env.NEXUSIM_DESKTOP_SIGN_CERT_SHA1 ?? "",
    timestampURL: env.NEXUSIM_DESKTOP_SIGN_TIMESTAMP_URL ?? "",
    pfxPassEnvPresent: Boolean(env.NEXUSIM_DESKTOP_SIGN_PFX_PASS)
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
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
    if (arg === "--signtool") {
      options.signToolPath = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--cert-file") {
      options.certFile = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--cert-sha1") {
      options.certSHA1 = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--timestamp-url") {
      options.timestampURL = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
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
    throw new Error("desktop signing plan leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop signing plan leaked a sensitive field name");
  }
}

function normalizeThumbprint(value) {
  return stringValue(value).replace(/\s+/g, "").toUpperCase();
}

function isHTTPURL(value) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
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

function normalizeArtifactKind(value) {
  return stringValue(value) || defaultDesktopArtifactKind;
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
