import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";
import { buildDesktopInstallerPlan } from "./plan-desktop-installer.mjs";

const schemaVersion = "nexusim.desktop-installer-build.v1";
const defaultTauriConfig = resolve(workspaceRoot, "desktop", "src-tauri", "tauri.conf.json");

function main(argv) {
  const options = parseArgs(argv, process.env);
  const plan = buildDesktopInstallerPlan(options);
  const output = buildInstallerOutput(plan, options);
  if (!options.execute) {
    process.stdout.write(`${JSON.stringify(output, null, 2)}\n`);
    return;
  }
  if (!output.readyToExecuteInstallerBuild) {
    process.stdout.write(`${JSON.stringify(output, null, 2)}\n`);
    const reasons = [...plan.missing, ...output.executionBlockers].join(",");
    throw new Error(`desktop installer build is not ready: ${reasons}`);
  }
  runBuildCommand();
}

export function buildInstallerOutput(plan, options = {}) {
  const execute = Boolean(options.execute);
  const executionBlockers = buildExecutionBlockers(options);
  const readyToExecuteInstallerBuild = plan.readyToBuildInstaller && executionBlockers.length === 0;
  const output = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    target: plan.target,
    readyToBuildInstaller: plan.readyToBuildInstaller,
    readyToExecuteInstallerBuild,
    missing: plan.missing,
    executionBlockers,
    executionPolicy: {
      planOnly: !execute,
      executeRequested: execute,
      executesBuildCommand: execute && readyToExecuteInstallerBuild,
      requiresExplicitExecuteFlag: true,
      buildsInstaller: execute && readyToExecuteInstallerBuild,
      signsArtifacts: false,
      installsArtifacts: false,
      launchesDesktopArtifacts: false,
      startsServices: false,
      downloadsToolchain: false,
      readsTauriConfig: true,
      readsCollectedArtifactManifest: true,
      readsSigningConfig: true,
      validatesArtifactHashes: true
    },
    command: "npm --prefix clients run build:desktop-artifact:collect",
    installerPlan: {
      schemaVersion: plan.schemaVersion,
      target: plan.target,
      readyToBuildInstaller: plan.readyToBuildInstaller,
      missing: plan.missing,
      expectedOutputHint: plan.expectedOutputHint,
      tauri: plan.tauri,
      signing: plan.signing,
      artifactBaseline: plan.artifactBaseline
    },
    nextAction: readyToExecuteInstallerBuild
      ? "rerun with --execute in an explicit Windows installer build profile"
      : "resolve installer readiness before running with --execute"
  };
  assertLowSensitiveOutput(output);
  return output;
}

function buildExecutionBlockers(options) {
  const blockers = [];
  if (options.tauriConfig && resolve(options.tauriConfig) !== defaultTauriConfig) {
    blockers.push("repository-tauri-config-required");
  }
  return blockers;
}

function runBuildCommand() {
  const npmExecPath = process.env.npm_execpath;
  if (npmExecPath) {
    execFileSync(process.execPath, [
      npmExecPath,
      "--prefix",
      "clients",
      "run",
      "build:desktop-artifact:collect"
    ], {
      cwd: workspaceRoot,
      stdio: "inherit"
    });
    return;
  }

  const npm = process.platform === "win32" ? "npm.cmd" : "npm";
  execFileSync(npm, ["--prefix", "clients", "run", "build:desktop-artifact:collect"], {
    cwd: workspaceRoot,
    stdio: "inherit",
    shell: process.platform === "win32"
  });
}

function parseArgs(argv, env) {
  const options = {
    execute: false,
    manifest: "",
    target: "msi",
    tauriConfig: undefined,
    tauriConfigExplicit: false,
    signToolPath: env.NEXUSIM_DESKTOP_SIGNTOOL ?? "",
    certFile: env.NEXUSIM_DESKTOP_SIGN_CERT_FILE ?? "",
    certSHA1: env.NEXUSIM_DESKTOP_SIGN_CERT_SHA1 ?? "",
    timestampURL: env.NEXUSIM_DESKTOP_SIGN_TIMESTAMP_URL ?? "",
    pfxPassEnvPresent: Boolean(env.NEXUSIM_DESKTOP_SIGN_PFX_PASS)
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--execute") {
      options.execute = true;
      continue;
    }
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
      options.tauriConfigExplicit = true;
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
  if (options.tauriConfig) {
    options.tauriConfig = resolve(options.tauriConfig);
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

function assertLowSensitiveOutput(output) {
  const serialized = JSON.stringify(output);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("desktop installer build output leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop installer build output leaked a sensitive field name");
  }
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
