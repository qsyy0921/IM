import { fileURLToPath } from "node:url";
import { existsSync, readdirSync, statSync } from "node:fs";
import { basename, isAbsolute, join, relative, resolve, sep } from "node:path";
import { createHash } from "node:crypto";
import { workspaceRoot } from "./client-build-env.mjs";
import { applyDesktopSigningProfile, defaultPfxPassEnv, signingProfileEnv } from "./desktop-signing-profile.mjs";
import { buildDesktopInstallerPlan, defaultInstallerTauriConfig } from "./plan-desktop-installer.mjs";
import { buildDesktopSigningPlan } from "./plan-desktop-signing.mjs";
import { buildSigningOutput } from "./sign-desktop-artifact.mjs";
import { buildDesktopSignatureVerificationReport } from "./verify-desktop-signature.mjs";

const schemaVersion = "nexusim.desktop-signing-readiness.v1";
const defaultDesktopArtifactKind = "desktop-executable";
const desktopInstallerArtifactKind = "desktop-installer";

function main(argv) {
  const options = parseArgs(argv, process.env);
  const report = buildDesktopSigningReadinessReport(options);
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
}

export function buildDesktopSigningReadinessReport(options = {}) {
  const artifactKind = stringValue(options.artifactKind) || defaultDesktopArtifactKind;
  const installerTarget = stringValue(options.target) || "msi";
  const signingPlan = buildDesktopSigningPlan({
    manifest: options.manifest,
    artifactsRoot: options.artifactsRoot,
    artifactKind,
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
  const signingExecution = buildSigningOutput(signingPlan, {
    execute: false,
    requireValid: true,
    signingProfile: options.signingProfile,
    expectedSignerSubjectContains: options.expectedSignerSubjectContains
  });
  const signatureVerification = buildDesktopSignatureVerificationReport({
    manifest: options.manifest,
    artifactsRoot: options.artifactsRoot,
    artifactKind,
    expectedSignerSubjectContains: options.expectedSignerSubjectContains,
    mockSignatureStatus: options.mockSignatureStatus
  });
  const installerPlan = buildDesktopInstallerPlan({
    manifest: options.manifest,
    artifactsRoot: options.artifactsRoot,
    target: installerTarget,
    tauriConfig: options.tauriConfig ?? defaultInstallerTauriConfig,
    signToolPath: options.signToolPath,
    certFile: options.certFile,
    certSHA1: options.certSHA1,
    timestampURL: options.timestampURL,
    pfxPassEnv: options.pfxPassEnv,
    pfxPassEnvPresent: options.pfxPassEnvPresent,
    pfxPassEnvValue: options.pfxPassEnvValue,
    pfxCertificateProbe: options.pfxCertificateProbe,
    certificateStoreProbe: options.certificateStoreProbe,
    expectedSignerSubjectContains: options.expectedSignerSubjectContains,
    mockSignatureStatus: options.mockSignatureStatus
  });
  const installerSignatureVerification = buildDesktopSignatureVerificationReport({
    manifest: options.installerManifest || options.manifest,
    artifactsRoot: options.artifactsRoot,
    artifactKind: desktopInstallerArtifactKind,
    expectedSignerSubjectContains: options.expectedSignerSubjectContains,
    mockSignatureStatus: options.mockInstallerSignatureStatus ?? options.mockSignatureStatus
  });
  const installerArtifactPresent = Boolean(installerSignatureVerification.artifact);
  const report = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    artifactKind,
    installerTarget,
    executionPolicy: executionPolicy(options),
    ready: {
      canAttemptSigning: Boolean(signingPlan.readyToSign),
      signatureValid: Boolean(signatureVerification.readyForSignedDistribution),
      canBuildInstaller: Boolean(installerPlan.readyToBuildInstaller),
      signedInstallerValid: installerArtifactPresent && Boolean(installerSignatureVerification.readyForSignedDistribution)
    },
    blockers: {
      signing: signingPlan.missing ?? [],
      signature: signatureVerification.missing ?? [],
      installer: installerPlan.missing ?? [],
      signedInstaller: installerSignatureVerification.missing ?? []
    },
    artifact: {
      manifest: signingPlan.artifactManifest ?? signatureVerification.artifactManifest,
      selected: signingPlan.artifact ?? signatureVerification.artifact ?? null
    },
    signing: {
      mode: signingPlan.signing?.mode ?? "none",
      readyToSign: Boolean(signingPlan.readyToSign),
      missing: signingPlan.missing ?? [],
      certificate: signingPlan.signing?.certificate ?? { source: "missing" },
      signTool: signingPlan.signing?.signTool ?? { present: false },
      timestamp: signingPlan.signing?.timestamp ?? { present: false },
      commandTemplate: signingPlan.commandTemplate
    },
    localToolHints: localToolHints(options),
    signingExecution: {
      readyToExecuteSigning: Boolean(signingExecution.readyToExecuteSigning),
      executionPolicy: signingExecution.executionPolicy,
      executionBlockers: signingExecution.executionBlockers ?? [],
      nextAction: signingExecution.nextAction
    },
    signatureVerification: {
      readyForSignedDistribution: Boolean(signatureVerification.readyForSignedDistribution),
      missing: signatureVerification.missing ?? [],
      status: signatureVerification.signature?.status ?? "UNKNOWN",
      signed: Boolean(signatureVerification.signature?.signed),
      trusted: Boolean(signatureVerification.signature?.trusted),
      nextAction: signatureVerification.nextAction
    },
    installer: {
      readyToBuildInstaller: Boolean(installerPlan.readyToBuildInstaller),
      missing: installerPlan.missing ?? [],
      target: installerPlan.target,
      tauri: installerPlan.tauri,
      commandTemplate: installerPlan.commandTemplate,
      expectedOutputHint: installerPlan.expectedOutputHint,
      nextAction: installerPlan.nextAction,
      postBuildSignatureVerification: {
        artifactPresent: installerArtifactPresent,
        readyForSignedDistribution: installerArtifactPresent && Boolean(installerSignatureVerification.readyForSignedDistribution),
        missing: installerSignatureVerification.missing ?? [],
        status: installerSignatureVerification.signature?.status ?? "UNKNOWN",
        signed: Boolean(installerSignatureVerification.signature?.signed),
        trusted: Boolean(installerSignatureVerification.signature?.trusted),
        nextAction: installerArtifactPresent
          ? installerSignatureVerification.nextAction
          : "build and collect a desktop installer artifact before installer signature verification"
      }
    },
    nextActions: nextActions(signingPlan, signatureVerification, installerPlan, installerSignatureVerification)
  };
  assertLowSensitiveReport(report);
  return report;
}

function nextActions(signingPlan, signatureVerification, installerPlan, installerSignatureVerification) {
  const actions = [];
  if (signingPlan.artifactManifest?.present === false || signatureVerification.artifactManifest?.present === false) {
    actions.push("collect a windows desktop artifact manifest before release signing checks");
  }
  if (!signingPlan.readyToSign) {
    actions.push("provide explicit signtool, timestamp URL and one code-signing certificate source");
  }
  if (!signatureVerification.readyForSignedDistribution) {
    actions.push("sign the selected artifact, then rerun signature verification with require-valid enabled");
  }
  if (!installerPlan.readyToBuildInstaller) {
    actions.push("resolve installer readiness blockers before running the installer build execute path");
  }
  if (installerPlan.readyToBuildInstaller && !installerSignatureVerification.artifact) {
    actions.push("run the installer build execute path, then collect the desktop-installer artifact");
  }
  if (installerSignatureVerification.artifact && !installerSignatureVerification.readyForSignedDistribution) {
    actions.push("sign the desktop-installer artifact, then rerun installer signature verification with require-valid enabled");
  }
  if (actions.length === 0) {
    actions.push("release checks passed for the signed desktop executable and signed installer artifact");
  }
  return actions;
}

function executionPolicy(options = {}) {
  return {
    reportOnly: true,
    planOnly: true,
    signsArtifacts: false,
    buildsInstaller: false,
    installsArtifacts: false,
    launchesDesktopArtifacts: false,
    startsServices: false,
    startsDocker: false,
    downloadsToolchain: false,
    readsCollectedArtifactManifest: true,
    readsSigningConfig: true,
    readsSigningProfile: Boolean(options.signingProfile),
    readsPfxCertificate: true,
    readsLocalToolHints: true,
    readsAuthenticodeSignature: true,
    readsInstallerAuthenticodeSignature: true,
    checksExpectedSignerSubject: Boolean(options.expectedSignerSubjectContains),
    validatesArtifactHashes: true
  };
}

function localToolHints(options) {
  const probeIssues = [];
  const candidates = collectSignToolCandidates(options, probeIssues);
  return {
    signtool: {
      configured: Boolean(stringValue(options.signToolPath)),
      candidatesUsedForReadiness: false,
      candidateCount: candidates.length,
      candidates,
      probeIssues,
      nextAction: candidates.length > 0
        ? "copy the chosen local path into an explicit signing profile or NEXUSIM_DESKTOP_SIGNTOOL"
        : "install Windows SDK signing tools or provide an explicit signtool path"
    }
  };
}

function collectSignToolCandidates(options, probeIssues) {
  const explicitCandidates = Array.isArray(options.signToolCandidatePaths) ? options.signToolCandidatePaths : [];
  const candidates = [];
  for (const candidate of explicitCandidates) {
    addSignToolCandidate(candidates, candidate, "explicit-candidate");
  }
  for (const root of defaultWindowsKitsRoots()) {
    collectWindowsKitsSignTools(candidates, root, probeIssues);
  }
  return uniqueByHint(candidates).slice(0, 8);
}

function collectWindowsKitsSignTools(candidates, kitsRoot, probeIssues) {
  if (!kitsRoot || !existsSync(kitsRoot)) {
    return;
  }
  const binRoot = join(kitsRoot, "bin");
  if (!existsSync(binRoot)) {
    return;
  }
  for (const versionEntry of safeReadDir(binRoot, probeIssues)) {
    if (!versionEntry.isDirectory()) {
      continue;
    }
    for (const arch of ["x64", "x86", "arm64"]) {
      addSignToolCandidate(candidates, join(binRoot, versionEntry.name, arch, "signtool.exe"), "windows-kits", {
        versionHint: versionEntry.name,
        arch
      });
    }
  }
  for (const arch of ["x64", "x86", "arm64"]) {
    addSignToolCandidate(candidates, join(binRoot, arch, "signtool.exe"), "windows-kits", { arch });
  }
}

function addSignToolCandidate(candidates, path, source, details = {}) {
  if (!path || !existsSync(path)) {
    return;
  }
  const stat = statSync(path);
  if (!stat.isFile()) {
    return;
  }
  candidates.push({
    source,
    hint: safeHint(path),
    ...details
  });
}

function defaultWindowsKitsRoots() {
  const roots = [];
  const programFilesX86 = process.env["ProgramFiles(x86)"] ?? "";
  const programFiles = process.env.ProgramFiles ?? "";
  if (programFilesX86) {
    roots.push(join(programFilesX86, "Windows Kits", "10"));
    roots.push(join(programFilesX86, "Windows Kits", "8.1"));
  }
  if (programFiles) {
    roots.push(join(programFiles, "Windows Kits", "10"));
    roots.push(join(programFiles, "Windows Kits", "8.1"));
  }
  return roots;
}

function safeReadDir(path, probeIssues) {
  try {
    return readdirSync(path, { withFileTypes: true });
  } catch (error) {
    probeIssues.push({
      hint: safeHint(path),
      code: error?.code ?? "READ_FAILED"
    });
    return [];
  }
}

function safeHint(path) {
  const relativePath = relative(workspaceRoot, resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(resolve(path)).slice(0, 12)}`;
  }
  return `clients/${relativePath}`;
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
}

function uniqueByHint(candidates) {
  const seen = new Set();
  const unique = [];
  for (const candidate of candidates) {
    if (seen.has(candidate.hint)) {
      continue;
    }
    seen.add(candidate.hint);
    unique.push(candidate);
  }
  return unique;
}

function parseArgs(argv, env) {
  const options = {
    manifest: "",
    installerManifest: "",
    signingProfile: env[signingProfileEnv] ?? "",
    artifactKind: defaultDesktopArtifactKind,
    target: "msi",
    tauriConfig: defaultInstallerTauriConfig,
    signToolPath: env.NEXUSIM_DESKTOP_SIGNTOOL ?? "",
    certFile: env.NEXUSIM_DESKTOP_SIGN_CERT_FILE ?? "",
    certSHA1: env.NEXUSIM_DESKTOP_SIGN_CERT_SHA1 ?? "",
    timestampURL: env.NEXUSIM_DESKTOP_SIGN_TIMESTAMP_URL ?? "",
    expectedSignerSubjectContains: env.NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT ?? "",
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
    if (arg === "--installer-manifest") {
      options.installerManifest = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--artifact-kind") {
      options.artifactKind = requiredValue(argv, index, arg);
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

function assertLowSensitiveReport(report) {
  const serialized = JSON.stringify(report);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("desktop signing readiness report leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop signing readiness report leaked a sensitive field name");
  }
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
