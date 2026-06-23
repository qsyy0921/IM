import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";
import { buildDesktopSigningPlan } from "./plan-desktop-signing.mjs";
import { buildDesktopSignatureVerificationReport } from "./verify-desktop-signature.mjs";
import { applyDesktopSigningProfile, defaultPfxPassEnv, signingProfileEnv } from "./desktop-signing-profile.mjs";

const schemaVersion = "nexusim.desktop-signing-execution.v1";
const artifactManifestSchema = "nexusim.client-artifacts.v1";
const artifactsRoot = join(workspaceRoot, "artifacts");
const defaultDesktopArtifactKind = "desktop-executable";
const desktopArtifactKinds = new Set(["desktop-executable", "desktop-installer"]);

function main(argv) {
  const options = parseArgs(argv, process.env);
  const plan = buildDesktopSigningPlan(options);
  const output = buildSigningOutput(plan, options);
  if (!options.execute) {
    process.stdout.write(`${JSON.stringify(output, null, 2)}\n`);
    return;
  }
  if (!output.readyToExecuteSigning) {
    process.stdout.write(`${JSON.stringify(output, null, 2)}\n`);
    const reasons = [...plan.missing, ...output.executionBlockers].join(",");
    throw new Error(`desktop artifact signing is not ready: ${reasons}`);
  }
  const signedInput = runSigningCommand(options);
  if (options.requireValid) {
    const verification = buildDesktopSignatureVerificationReport({
      manifest: signedInput.manifestPath,
      artifactKind: options.artifactKind,
      expectedSignerSubjectContains: options.expectedSignerSubjectContains,
      requireValid: true
    });
    if (!verification.readyForSignedDistribution) {
      const missing = verification.missing.length > 0 ? verification.missing.join(",") : "valid-authenticode-signature";
      throw new Error(`desktop artifact signature is not valid after signing: ${missing}`);
    }
  }
}

export function buildSigningOutput(plan, options = {}) {
  const execute = Boolean(options.execute);
  const executionBlockers = [];
  const readyToExecuteSigning = plan.readyToSign && executionBlockers.length === 0;
  const output = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    readyToSign: plan.readyToSign,
    readyToExecuteSigning,
    missing: plan.missing,
    executionBlockers,
    executionPolicy: {
      planOnly: !execute,
      executeRequested: execute,
      executesSignCommand: execute && readyToExecuteSigning,
      requiresExplicitExecuteFlag: true,
      signsArtifacts: execute && readyToExecuteSigning,
      installsArtifacts: false,
      launchesDesktopArtifacts: false,
      startsServices: false,
      downloadsToolchain: false,
      readsCollectedArtifactManifest: true,
      readsSigningConfig: true,
      readsSigningProfile: Boolean(options.signingProfile),
      validatesArtifactHashes: true,
      readsAuthenticodeSignature: Boolean(options.requireValid),
      requiresValidSignatureAfterSigning: Boolean(options.requireValid),
      expectedSignerSubjectPolicyConfigured: Boolean(options.expectedSignerSubjectContains),
      requiresExpectedSignerSubjectAfterSigning: Boolean(options.requireValid && options.expectedSignerSubjectContains),
      verifiesSignatureAfterSigning: execute && readyToExecuteSigning && Boolean(options.requireValid)
    },
    commandTemplate: plan.commandTemplate,
    signingPlan: {
      schemaVersion: plan.schemaVersion,
      readyToSign: plan.readyToSign,
      missing: plan.missing,
      artifactManifest: plan.artifactManifest,
      artifact: plan.artifact,
      signing: plan.signing
    },
    nextAction: readyToExecuteSigning
      ? "rerun with --execute in an explicit Windows signing profile; add --require-valid for release signing"
      : "resolve signing readiness before running with --execute"
  };
  assertLowSensitiveOutput(output);
  return output;
}

function runSigningCommand(options) {
  const input = resolveSigningInput(options);
  const signToolPath = resolve(requiredString(options.signToolPath, "signtool path"));
  const timestampURL = requiredString(options.timestampURL, "timestamp URL");
  const args = [
    "sign",
    "/fd",
    "SHA256",
    "/tr",
    timestampURL,
    "/td",
    "SHA256"
  ];

  if (options.certFile) {
    const pfxPassEnv = options.pfxPassEnv || defaultPfxPassEnv;
    args.push("/f", resolve(options.certFile), "/p", requiredString(process.env[pfxPassEnv], "PFX password env"));
  } else {
    args.push("/sha1", normalizeThumbprint(requiredString(options.certSHA1, "certificate SHA1")));
  }
  args.push(input.artifactPath);

  execFileSync(signToolPath, args, {
    cwd: workspaceRoot,
    stdio: "inherit",
    shell: process.platform === "win32"
  });
  return input;
}

function resolveSigningInput(options) {
  const artifactKind = normalizeArtifactKind(options.artifactKind);
  if (!desktopArtifactKinds.has(artifactKind)) {
    throw new Error("desktop artifact kind invalid");
  }
  const manifestPath = options.manifest
    ? resolve(options.manifest)
    : findLatestArtifactManifest(artifactsRoot, "windows-desktop", artifactKind);
  if (!manifestPath) {
    throw new Error("desktop artifact manifest missing");
  }
  const manifest = readManifest(manifestPath);
  const artifact = manifest.artifacts.find(
    candidate => candidate?.target === "windows-desktop" && candidate.artifactKind === artifactKind
  );
  if (!artifact) {
    throw new Error(`windows desktop ${artifactKind} artifact missing`);
  }
  const artifactPath = join(dirname(manifestPath), artifact.filename);
  validateArtifact(artifact, artifactPath);
  return {
    manifestPath,
    artifactPath
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
    execute: false,
    manifest: "",
    signingProfile: env[signingProfileEnv] ?? "",
    artifactKind: defaultDesktopArtifactKind,
    signToolPath: env.NEXUSIM_DESKTOP_SIGNTOOL ?? "",
    certFile: env.NEXUSIM_DESKTOP_SIGN_CERT_FILE ?? "",
    certSHA1: env.NEXUSIM_DESKTOP_SIGN_CERT_SHA1 ?? "",
    timestampURL: env.NEXUSIM_DESKTOP_SIGN_TIMESTAMP_URL ?? "",
    expectedSignerSubjectContains: env.NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT ?? "",
    pfxPassEnv: defaultPfxPassEnv,
    pfxPassEnvPresent: Boolean(env[defaultPfxPassEnv]),
    requireValid: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--execute") {
      options.execute = true;
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
    if (arg === "--expected-signer-subject") {
      options.expectedSignerSubjectContains = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return applyDesktopSigningProfile(options, env);
}

function requiredValue(argv, index, name) {
  const value = argv[index + 1];
  if (!value || value.startsWith("--")) {
    throw new Error(`${name} requires a value`);
  }
  return value;
}

function requiredString(value, label) {
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`${label} missing`);
  }
  return value;
}

function normalizeThumbprint(value) {
  return value.replace(/\s+/g, "").toUpperCase();
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
    throw new Error("desktop signing execution output leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop signing execution output leaked a sensitive field name");
  }
}

function sha256Buffer(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}

function normalizeArtifactKind(value) {
  return typeof value === "string" && value.length > 0 ? value : defaultDesktopArtifactKind;
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
