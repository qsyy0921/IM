import { existsSync, readdirSync, rmSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join, resolve } from "node:path";
import {
  collectClientBuildPrereqs,
  commandSucceeded,
  localNodeBin,
  workspaceRoot
} from "./client-build-env.mjs";
import {
  collectArgs,
  collectPlanSummary,
  parseArtifactBuildOptions
} from "./client-artifact-build-options.mjs";
import { prepareShellWebAssets } from "./prepare-shell-web-assets.mjs";
import { verifyShellAssets } from "./verify-shell-assets.mjs";

const desktopRoot = join(workspaceRoot, "desktop");
const desktopTauriRoot = join(desktopRoot, "src-tauri");

function main(argv) {
  const options = parseArtifactBuildOptions(argv);
  const prereqs = collectClientBuildPrereqs();
  const plan = desktopBuildPlan(prereqs, options);

  if (options.dryRun) {
    console.log(JSON.stringify(plan, null, 2));
    return;
  }
  if (!plan.ready) {
    console.log(JSON.stringify(plan, null, 2));
    throw new Error("desktop artifact toolchain is not ready");
  }

  prepareShellWebAssets({
    target: "windows-desktop",
    configPath: options.shellConfigPath || undefined,
    build: !options.skipWebBuild
  });
  verifyShellAssets({ target: "windows-desktop" });
  if (options.shellConfigPath) {
    cleanDesktopPackageForCustomShellConfig();
  }
  execFileSync(plan.command, plan.args, {
    cwd: desktopRoot,
    stdio: "inherit",
    shell: plan.shell,
    env: {
      ...process.env,
      NEXUSIM_SKIP_SHELL_ASSET_PREP: "true"
    }
  });
  if (options.collect) {
    execFileSync(process.execPath, collectArgs("windows-desktop", options), {
      cwd: workspaceRoot,
      stdio: "inherit"
    });
  }
}

function desktopBuildPlan(prereqs, options) {
  const localTauri = localNodeBin("tauri");
  const hasLocalTauri = existsSync(localTauri);
  const hasCargoTauri = commandSucceeded("cargo", ["tauri", "--version"]);
  const missing = prereqs.checks
    .filter(check => check.target === "desktop" && !check.ok)
    .map(check => check.name);

  if (hasLocalTauri) {
    return {
      target: "windows-desktop",
      ready: prereqs.desktopArtifactReady,
      missing,
      command: process.platform === "win32" ? "cmd.exe" : localTauri,
      args: process.platform === "win32"
        ? ["/d", "/c", localTauri, "build"]
        : ["build"],
      shell: false,
      skipShellAssetPrepEnv: "NEXUSIM_SKIP_SHELL_ASSET_PREP",
      shellConfig: options.shellConfigPath ? "custom" : "default",
      forceFreshTauriAssets: Boolean(options.shellConfigPath),
      outputHint: "clients/desktop/src-tauri/target/release/nexusim-desktop.exe or bundle",
      collectArtifacts: collectPlanSummary("windows-desktop", options)
    };
  }
  return {
    target: "windows-desktop",
    ready: prereqs.desktopArtifactReady,
    missing,
    command: "cargo",
    args: ["tauri", "build"],
    shell: false,
    skipShellAssetPrepEnv: "NEXUSIM_SKIP_SHELL_ASSET_PREP",
    shellConfig: options.shellConfigPath ? "custom" : "default",
    forceFreshTauriAssets: Boolean(options.shellConfigPath),
    outputHint: "clients/desktop/src-tauri/target/release/nexusim-desktop.exe or bundle",
    cargoTauriDetected: hasCargoTauri,
    collectArtifacts: collectPlanSummary("windows-desktop", options)
  };
}

function cleanDesktopPackageForCustomShellConfig() {
  const releaseRoot = join(desktopTauriRoot, "target", "release");
  removeMatchingEntries(join(releaseRoot, "build"), /^nexusim-desktop-/);
  removeMatchingEntries(join(releaseRoot, ".fingerprint"), /^nexusim-desktop-/);
  removeMatchingEntries(join(releaseRoot, "deps"), /^nexusim[_-]desktop/);
  for (const filename of ["nexusim-desktop.exe", "nexusim-desktop.d", "nexusim_desktop.pdb"]) {
    removeBuildPath(join(releaseRoot, filename));
  }
}

function removeMatchingEntries(root, pattern) {
  if (!existsSync(root)) {
    return;
  }
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    if (pattern.test(entry.name)) {
      removeBuildPath(join(root, entry.name));
    }
  }
}

function removeBuildPath(path) {
  const resolved = resolve(path);
  const releaseRoot = resolve(desktopTauriRoot, "target", "release");
  if (resolved !== releaseRoot && !resolved.startsWith(`${releaseRoot}\\`) && !resolved.startsWith(`${releaseRoot}/`)) {
    throw new Error("refusing to remove a path outside desktop release build output");
  }
  rmSync(resolved, { recursive: true, force: true });
}

try {
  main(process.argv.slice(2));
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 2;
}
