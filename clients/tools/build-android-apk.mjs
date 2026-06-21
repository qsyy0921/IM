import { existsSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { collectClientBuildPrereqs, workspaceRoot } from "./client-build-env.mjs";
import { prepareShellWebAssets } from "./prepare-shell-web-assets.mjs";

const androidNativeRoot = join(workspaceRoot, "android", "native");

function main(argv) {
  const dryRun = argv.includes("--dry-run");
  const skipWebBuild = argv.includes("--skip-web-build");
  const prereqs = collectClientBuildPrereqs();
  const plan = androidBuildPlan(prereqs);

  if (dryRun) {
    console.log(JSON.stringify(plan, null, 2));
    return;
  }
  if (!plan.ready) {
    console.log(JSON.stringify(plan, null, 2));
    throw new Error("Android APK toolchain is not ready");
  }

  prepareShellWebAssets({
    target: "android",
    build: !skipWebBuild
  });
  execFileSync(plan.command, plan.args, {
    cwd: androidNativeRoot,
    stdio: "inherit",
    shell: plan.shell
  });
}

function androidBuildPlan(prereqs) {
  const gradlew = join(androidNativeRoot, process.platform === "win32" ? "gradlew.bat" : "gradlew");
  const hasGradleWrapper = existsSync(gradlew);
  const missing = prereqs.checks
    .filter(check => check.target === "android" && !check.ok)
    .map(check => check.name);
  return {
    target: "android",
    ready: prereqs.androidApkReady,
    missing,
    command: hasGradleWrapper ? gradlew : "gradle",
    args: hasGradleWrapper ? [":app:assembleDebug"] : ["-p", androidNativeRoot, ":app:assembleDebug"],
    shell: false,
    outputHint: "clients/android/native/app/build/outputs/apk/debug/app-debug.apk",
    gradleWrapperDetected: hasGradleWrapper
  };
}

try {
  main(process.argv.slice(2));
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 2;
}
