import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { basename, dirname, extname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";
import { applyDesktopSigningProfile, defaultPfxPassEnv, signingProfileEnv } from "./desktop-signing-profile.mjs";
import { buildDesktopSigningPlan } from "./plan-desktop-signing.mjs";
import { buildDesktopSignatureVerificationReport } from "./verify-desktop-signature.mjs";

const schemaVersion = "nexusim.desktop-installer-plan.v1";
const artifactManifestSchema = "nexusim.client-artifacts.v1";
export const defaultInstallerTauriConfig = join(workspaceRoot, "desktop", "src-tauri", "tauri.installer.conf.json");
export const defaultInstallerTauriConfigCommandArg = "src-tauri/tauri.installer.conf.json";
const artifactsRoot = join(workspaceRoot, "artifacts");
const supportedTargets = new Set(["msi", "nsis"]);
const desktopExecutableArtifactKind = "desktop-executable";

function main(argv) {
  const options = parseArgs(argv, process.env);
  const plan = buildDesktopInstallerPlan(options);
  process.stdout.write(`${JSON.stringify(plan, null, 2)}\n`);
}

export function buildDesktopInstallerPlan(options = {}) {
  const target = normalizeTarget(options.target || "msi");
  const base = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    executionPolicy: executionPolicy()
  };
  const missing = [];
  if (!supportedTargets.has(target)) {
    missing.push("supported-installer-target");
  }

  const tauriConfigPath = resolve(options.tauriConfig ?? defaultInstallerTauriConfig);
  const tauriConfig = readTauriConfig(tauriConfigPath);
  if (!tauriConfig.present) {
    missing.push("tauri-config");
  }
  if (tauriConfig.present && tauriConfig.bundle.active !== true) {
    missing.push("tauri-bundle-active");
  }
  if (tauriConfig.present && !tauriConfig.bundle.targets.includes(target)) {
    missing.push("installer-target-declared");
  }

  const artifactManifestPath = options.manifest
    ? resolve(options.manifest)
    : findLatestArtifactManifest(options.artifactsRoot ?? artifactsRoot, "windows-desktop", desktopExecutableArtifactKind);
  const artifactState = artifactManifestPath
    ? readDesktopArtifactState(artifactManifestPath)
    : {
        present: false,
        manifestHint: "clients/artifacts/<run-id>/manifest.json",
        desktopArtifactPresent: false,
        artifact: null
      };
  if (!artifactState.desktopArtifactPresent) {
    missing.push("windows-desktop-artifact-baseline");
  }
  if (artifactState.desktopArtifactPresent && artifactState.artifact?.artifactKind !== "desktop-executable") {
    missing.push(artifactState.artifact?.artifactKind ? "desktop-executable-baseline" : "desktop-artifact-kind");
  }

  const signingPlan = buildDesktopSigningPlan({
    manifest: artifactManifestPath || "",
    signToolPath: options.signToolPath,
    certFile: options.certFile,
    certSHA1: options.certSHA1,
    timestampURL: options.timestampURL,
    pfxPassEnv: options.pfxPassEnv,
    pfxPassEnvPresent: options.pfxPassEnvPresent,
    pfxPassEnvValue: options.pfxPassEnvValue,
    pfxCertificateProbe: options.pfxCertificateProbe,
    certificateStoreProbe: options.certificateStoreProbe
  });
  if (!signingPlan.readyToSign) {
    missing.push("desktop-signing-ready");
  }
  const signatureVerification = artifactManifestPath
    ? buildDesktopSignatureVerificationReport({
        manifest: artifactManifestPath,
        mockSignatureStatus: options.mockSignatureStatus
      })
    : {
        readyForSignedDistribution: false,
        missing: ["artifact-manifest"],
        signature: {
          checked: false,
          status: "UNAVAILABLE"
        }
      };
  if (!signatureVerification.readyForSignedDistribution) {
    missing.push("desktop-signature-valid");
  }

  const readyToBuildInstaller = missing.length === 0;
  const plan = {
    ...base,
    target,
    readyToBuildInstaller,
    missing: unique(missing),
    tauri: tauriConfig,
    artifactBaseline: artifactState,
    signing: {
      readyToSign: signingPlan.readyToSign,
      missing: signingPlan.missing ?? [],
      mode: signingPlan.signing?.mode ?? "none"
    },
    signatureVerification: {
      readyForSignedDistribution: signatureVerification.readyForSignedDistribution,
      missing: signatureVerification.missing ?? [],
      status: signatureVerification.signature?.status ?? "UNKNOWN",
      signed: Boolean(signatureVerification.signature?.signed),
      trusted: Boolean(signatureVerification.signature?.trusted)
    },
    commandTemplate: readyToBuildInstaller
      ? {
          build: [
            "npm",
            "--workspace",
            "@nexusim/desktop",
            "run",
            "tauri:build",
            "--",
            "--bundles",
            target,
            "--config",
            tauriConfig.commandArg
          ],
          collect: [
            "npm",
            "--prefix",
            "clients",
            "run",
            "collect:client-artifacts",
            "--",
            "--target",
            "windows-desktop"
          ]
        }
      : null,
    expectedOutputHint: `clients/desktop/src-tauri/target/release/bundle/${target}/`,
    nextAction: readyToBuildInstaller
      ? "run the installer build in a dedicated Windows packaging profile"
      : "provide an explicit repository installer profile, desktop artifact baseline, signing inputs, and a valid signed artifact before building an installer"
  };
  assertLowSensitivePlan(plan);
  return plan;
}

function readTauriConfig(path) {
  if (!existsSync(path)) {
    return {
      present: false,
      configHint: safeHint(path),
      commandArg: safeCommandArg(path),
      bundle: {
        active: false,
        targets: []
      }
    };
  }
  const raw = readFileSync(path, "utf8");
  assertLowSensitiveText(raw, "tauri config");
  const config = JSON.parse(raw);
  const bundle = config.bundle ?? {};
  return {
    present: true,
    configHint: safeHint(path),
    commandArg: safeCommandArg(path),
    productName: stringValue(config.productName),
    version: stringValue(config.version),
    identifier: stringValue(config.identifier),
    bundle: {
      active: bundle.active === true,
      targets: normalizeTargets(bundle.targets),
      publisherConfigured: Boolean(stringValue(bundle.publisher))
    }
  };
}

function readDesktopArtifactState(manifestPath) {
  const manifest = readManifest(manifestPath);
  const artifact = findDesktopBaselineArtifact(manifest);
  if (!artifact) {
    return {
      present: true,
      manifestHint: safeHint(manifestPath),
      runId: stringValue(manifest.runId),
      desktopArtifactPresent: false,
      artifact: null
    };
  }
  const artifactPath = join(dirname(manifestPath), artifact.filename);
  const artifactInfo = validateArtifact(artifact, artifactPath);
  return {
    present: true,
    manifestHint: safeHint(manifestPath),
    runId: stringValue(manifest.runId),
    gitCommit: stringValue(manifest.gitCommit),
    desktopArtifactPresent: true,
    artifact: artifactInfo
  };
}

function findDesktopBaselineArtifact(manifest) {
  return manifest.artifacts.find(
    candidate => candidate?.target === "windows-desktop" && candidate.artifactKind === desktopExecutableArtifactKind
  ) ?? manifest.artifacts.find(candidate => candidate?.target === "windows-desktop");
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
  if (!existsSync(artifactPath)) {
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
    extension: extname(artifact.filename).toLowerCase()
  };
}

function executionPolicy() {
  return {
    planOnly: true,
    buildsInstaller: false,
    signsArtifacts: false,
    installsArtifacts: false,
    launchesDesktopArtifacts: false,
    startsServices: false,
    downloadsToolchain: false,
    readsTauriConfig: true,
    readsCollectedArtifactManifest: true,
    readsSigningConfig: true,
    readsPfxCertificate: true,
    readsAuthenticodeSignature: true,
    validatesArtifactHashes: true
  };
}

function parseArgs(argv, env) {
  const options = {
    manifest: "",
    signingProfile: env[signingProfileEnv] ?? "",
    target: "msi",
    tauriConfig: defaultInstallerTauriConfig,
    artifactsRoot,
    signToolPath: env.NEXUSIM_DESKTOP_SIGNTOOL ?? "",
    certFile: env.NEXUSIM_DESKTOP_SIGN_CERT_FILE ?? "",
    certSHA1: env.NEXUSIM_DESKTOP_SIGN_CERT_SHA1 ?? "",
    timestampURL: env.NEXUSIM_DESKTOP_SIGN_TIMESTAMP_URL ?? "",
    pfxPassEnv: defaultPfxPassEnv,
    pfxPassEnvPresent: Boolean(env[defaultPfxPassEnv])
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--manifest") {
      options.manifest = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--target") {
      options.target = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--tauri-config") {
      options.tauriConfig = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--signing-profile") {
      options.signingProfile = requiredValue(argv, index, arg);
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
  return applyDesktopSigningProfile(options, env);
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

function normalizeTarget(value) {
  return stringValue(value).toLowerCase();
}

function normalizeTargets(value) {
  if (Array.isArray(value)) {
    return value.map(target => stringValue(target).toLowerCase()).filter(Boolean);
  }
  if (typeof value === "string" && value) {
    return [value.toLowerCase()];
  }
  return [];
}

function safeHint(path) {
  const relativePath = relative(workspaceRoot, resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(resolve(path)).slice(0, 12)}`;
  }
  return `clients/${relativePath}`;
}

function safeCommandArg(path) {
  if (resolve(path) === resolve(defaultInstallerTauriConfig)) {
    return defaultInstallerTauriConfigCommandArg;
  }
  return safeHint(path);
}

function assertLowSensitiveText(text, label) {
  if (text.match(/[A-Za-z]:\\\\/) || text.includes("\\\\?")) {
    throw new Error(`${label} leaked a local absolute path`);
  }
  if (text.match(/(token|secret|password|credential|private)/i)) {
    throw new Error(`${label} contains a sensitive field name`);
  }
}

function assertLowSensitivePlan(plan) {
  const serialized = JSON.stringify(plan);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("desktop installer plan leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop installer plan leaked a sensitive field name");
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
