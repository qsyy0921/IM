import { existsSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import {
  collectClientBuildPrereqs,
  commandSucceeded,
  localNodeBin,
  quoteCommand,
  workspaceRoot
} from "./client-build-env.mjs";
import { prepareShellWebAssets } from "./prepare-shell-web-assets.mjs";

const desktopRoot = join(workspaceRoot, "desktop");

function main(argv) {
  const dryRun = argv.includes("--dry-run");
  const skipWebBuild = argv.includes("--skip-web-build");
  const prereqs = collectClientBuildPrereqs();
  const plan = desktopBuildPlan(prereqs);

  if (dryRun) {
    console.log(JSON.stringify(plan, null, 2));
    return;
  }
  if (!plan.ready) {
    console.log(JSON.stringify(plan, null, 2));
    throw new Error("desktop artifact toolchain is not ready");
  }

  prepareShellWebAssets({
    target: "windows-desktop",
    build: !skipWebBuild
  });
  execFileSync(plan.command, plan.args, {
    cwd: desktopRoot,
    stdio: "inherit",
    shell: plan.shell
  });
}

function desktopBuildPlan(prereqs) {
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
        ? ["/d", "/s", "/c", quoteCommand(localTauri, ["build"])]
        : ["build"],
      shell: false,
      outputHint: "clients/desktop/src-tauri/target/release/bundle"
    };
  }
  return {
    target: "windows-desktop",
    ready: prereqs.desktopArtifactReady,
    missing,
    command: "cargo",
    args: ["tauri", "build"],
    shell: false,
    outputHint: "clients/desktop/src-tauri/target/release/bundle",
    cargoTauriDetected: hasCargoTauri
  };
}

try {
  main(process.argv.slice(2));
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 2;
}
