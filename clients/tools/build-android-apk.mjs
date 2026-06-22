import { existsSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { collectClientBuildPrereqs, workspaceRoot } from "./client-build-env.mjs";
import {
  collectArgs,
  collectPlanSummary,
  parseArtifactBuildOptions
} from "./client-artifact-build-options.mjs";
import { prepareShellWebAssets } from "./prepare-shell-web-assets.mjs";
import { verifyShellAssets } from "./verify-shell-assets.mjs";

const androidNativeRoot = join(workspaceRoot, "android", "native");

function main(argv) {
  const options = parseArtifactBuildOptions(argv);
  const prereqs = collectClientBuildPrereqs();
  const plan = androidBuildPlan(prereqs, options);

  if (options.dryRun) {
    console.log(JSON.stringify({
      ...plan,
      executionPolicy: dryRunExecutionPolicy()
    }, null, 2));
    return;
  }
  if (!plan.ready) {
    console.log(JSON.stringify(plan, null, 2));
    throw new Error("Android APK toolchain is not ready");
  }

  prepareShellWebAssets({
    target: "android",
    configPath: options.shellConfigPath || undefined,
    build: !options.skipWebBuild
  });
  verifyShellAssets({ target: "android" });
  execFileSync(plan.command, plan.args, {
    cwd: androidNativeRoot,
    stdio: "inherit",
    shell: plan.shell
  });
  if (options.collect) {
    execFileSync(process.execPath, collectArgs("android", options), {
      cwd: workspaceRoot,
      stdio: "inherit"
    });
  }
}

function dryRunExecutionPolicy() {
  return {
    planOnly: true,
    executesBuildCommand: false,
    preparesShellAssets: false,
    verifiesShellAssets: false,
    collectsArtifacts: false,
    writesBuildOutput: false,
    startsDocker: false,
    installsArtifacts: false,
    startsActivity: false,
    contactsDevice: false,
    downloadsToolchain: false
  };
}

function androidBuildPlan(prereqs, options) {
  const gradlew = join(androidNativeRoot, process.platform === "win32" ? "gradlew.bat" : "gradlew");
  const hasGradleWrapper = existsSync(gradlew);
  const gradleArgs = ["-Pnexusim.skipWebAssetPrep=true", ":app:assembleDebug"];
  const gradleCommand = hasGradleWrapper ? gradlew : "gradle";
  const missing = prereqs.checks
    .filter(check => check.target === "android" && !check.ok)
    .map(check => check.name);
  const command = process.platform === "win32" && !hasGradleWrapper ? "cmd.exe" : gradleCommand;
  const args = process.platform === "win32" && !hasGradleWrapper
    ? ["/d", "/c", "gradle.bat", ...gradleArgs]
    : gradleArgs;
  return {
    target: "android",
    ready: prereqs.androidApkReady,
    missing,
    command,
    args,
    shell: false,
    cwdHint: "clients/android/native",
    outputHint: "clients/android/native/app/build/outputs/apk/debug/app-debug.apk",
    gradleWrapperDetected: hasGradleWrapper,
    shellConfig: options.shellConfigPath ? "custom" : "default",
    collectArtifacts: collectPlanSummary("android", options)
  };
}

try {
  main(process.argv.slice(2));
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 2;
}
