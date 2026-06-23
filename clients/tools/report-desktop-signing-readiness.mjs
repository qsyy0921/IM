import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { applyDesktopSigningProfile, defaultPfxPassEnv, signingProfileEnv } from "./desktop-signing-profile.mjs";
import { buildDesktopInstallerPlan, defaultInstallerTauriConfig } from "./plan-desktop-installer.mjs";
import { buildDesktopSigningPlan } from "./plan-desktop-signing.mjs";
import { buildSigningOutput } from "./sign-desktop-artifact.mjs";
import { buildDesktopSignatureVerificationReport } from "./verify-desktop-signature.mjs";

const schemaVersion = "nexusim.desktop-signing-readiness.v1";
const defaultDesktopArtifactKind = "desktop-executable";

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
    pfxPassEnvPresent: options.pfxPassEnvPresent
  });
  const signingExecution = buildSigningOutput(signingPlan, {
    execute: false,
    requireValid: true
  });
  const signatureVerification = buildDesktopSignatureVerificationReport({
    manifest: options.manifest,
    artifactsRoot: options.artifactsRoot,
    artifactKind,
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
    mockSignatureStatus: options.mockSignatureStatus
  });
  const report = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    artifactKind,
    installerTarget,
    executionPolicy: executionPolicy(),
    ready: {
      canAttemptSigning: Boolean(signingPlan.readyToSign),
      signatureValid: Boolean(signatureVerification.readyForSignedDistribution),
      canBuildInstaller: Boolean(installerPlan.readyToBuildInstaller)
    },
    blockers: {
      signing: signingPlan.missing ?? [],
      signature: signatureVerification.missing ?? [],
      installer: installerPlan.missing ?? []
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
      nextAction: installerPlan.nextAction
    },
    nextActions: nextActions(signingPlan, signatureVerification, installerPlan)
  };
  assertLowSensitiveReport(report);
  return report;
}

function nextActions(signingPlan, signatureVerification, installerPlan) {
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
  if (actions.length === 0) {
    actions.push("run the installer build execute path in the dedicated Windows packaging profile");
  }
  return actions;
}

function executionPolicy() {
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
    readsAuthenticodeSignature: true,
    validatesArtifactHashes: true
  };
}

function parseArgs(argv, env) {
  const options = {
    manifest: "",
    signingProfile: env[signingProfileEnv] ?? "",
    artifactKind: defaultDesktopArtifactKind,
    target: "msi",
    tauriConfig: defaultInstallerTauriConfig,
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
