import { createHash } from "node:crypto";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { buildClientArtifactInstallPlan } from "./plan-client-artifact-install.mjs";
import { workspaceRoot } from "./client-build-env.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

const runDir = join(workspaceRoot, "artifacts", "install-plan-test");
try {
  rmSync(runDir, { recursive: true, force: true });
  mkdirSync(runDir, { recursive: true });

  const apk = "fake apk bytes";
  const msi = "fake msi bytes";
  writeFileSync(join(runDir, "nexusim-android-debug.apk"), apk);
  writeFileSync(join(runDir, "nexusim-windows-desktop.msi"), msi);
  writeFileSync(join(runDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-22T00:00:00.000Z",
    gitCommit: "test",
    runId: "install-plan-test",
    artifacts: [
      {
        target: "android",
        filename: "nexusim-android-debug.apk",
        bytes: Buffer.byteLength(apk),
        sha256: sha256(apk),
        sourcePathHash: sha256("android-source"),
        sourceHint: "android/native/app/build/outputs/apk/debug/app-debug.apk"
      },
      {
        target: "windows-desktop",
        filename: "nexusim-windows-desktop.msi",
        bytes: Buffer.byteLength(msi),
        sha256: sha256(msi),
        sourcePathHash: sha256("desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      }
    ]
  }, null, 2)}\n`);

  const plan = buildClientArtifactInstallPlan({
    manifest: join(runDir, "manifest.json"),
    installPrereqs: {
      adbAvailable: true,
      windowsInstallerLaunchSupported: true
    }
  });
  const serialized = JSON.stringify(plan);

  assert(plan.schemaVersion === "nexusim.client-artifact-install-plan.v1", "install plan schema mismatch");
  assertInstallPlanExecutionPolicy(plan.executionPolicy);
  assert(plan.artifactManifest.present === true, "manifest should be present");
  assert(plan.artifactManifest.manifestHint === "clients/artifacts/install-plan-test/manifest.json", "manifest hint mismatch");
  assert(plan.targets.android.artifactReady === true, "android artifact should be ready");
  assert(plan.targets.android.readyForInstall === true, "android install plan should be ready");
  assert(plan.targets.android.installPrereqs.adbAvailable === true, "android adb prereq should be true");
  assert(plan.targets.android.artifact.artifactHint === "clients/artifacts/install-plan-test/nexusim-android-debug.apk", "android artifact hint mismatch");
  const androidInstallStep = plan.targets.android.checklist.find(item => item.step === "install-apk");
  assert(androidInstallStep?.command?.includes("adb install -r clients/artifacts/install-plan-test/nexusim-android-debug.apk"), "android install command missing");
  assert(androidInstallStep.manualOnly === true, "android install step should be manual-only");
  assert(androidInstallStep.contactsDevice === true, "android install step should be marked as contacting a device");
  assert(androidInstallStep.installsArtifacts === true, "android install step should be marked as installing artifacts");
  assert(androidInstallStep.startsDeviceActivities === false, "android install step should not be marked as starting activities");
  assert(androidInstallStep.requiresExplicitUserAction === true, "android install step should require explicit user action");
  const androidSmokeStep = plan.targets.android.checklist.find(item => item.step === "run-client-smoke");
  assert(androidSmokeStep?.manualOnly === true, "android smoke step should be manual-only");
  assert(androidSmokeStep.contactsDevice === true, "android smoke step should be marked as contacting a device");
  assert(androidSmokeStep.startsDeviceActivities === true, "android smoke step should be marked as starting device activities");
  assert(plan.targets["windows-desktop"].artifactReady === true, "desktop artifact should be ready");
  assert(plan.targets["windows-desktop"].readyForInstall === true, "desktop install plan should be ready");
  assert(plan.targets["windows-desktop"].installPrereqs.windowsInstallerLaunchSupported === true, "desktop install prereq should be true");
  const desktopLaunchStep = plan.targets["windows-desktop"].checklist.find(item => item.step === "launch-desktop-artifact");
  assert(desktopLaunchStep, "desktop launch step missing");
  assert(desktopLaunchStep.command?.includes("Start-Process clients/artifacts/install-plan-test/nexusim-windows-desktop.msi"), "desktop launch command missing");
  assert(desktopLaunchStep.manualOnly === true, "desktop launch step should be manual-only");
  assert(desktopLaunchStep.launchesDesktopArtifact === true, "desktop launch step should be marked as launching the artifact");
  assert(desktopLaunchStep.startsLocalProcess === true, "desktop launch step should be marked as starting a local process");
  assert(desktopLaunchStep.requiresExplicitUserAction === true, "desktop launch step should require explicit user action");
  assert(!serialized.match(/[A-Z]:\\\\/), "install plan leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "install plan leaked extended Windows path");
  assert(!serialized.match(/token|secret|password|credential|private/i), "install plan leaked sensitive field name");

  const nestedRoot = join(runDir, "android");
  const nestedDir = join(nestedRoot, "docker-android-debug");
  mkdirSync(nestedDir, { recursive: true });
  writeFileSync(join(nestedDir, "nexusim-android-debug.apk"), apk);
  writeFileSync(join(nestedDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-22T00:00:00.000Z",
    gitCommit: "test",
    runId: "docker-android-debug",
    artifacts: [
      {
        target: "android",
        filename: "nexusim-android-debug.apk",
        bytes: Buffer.byteLength(apk),
        sha256: sha256(apk),
        sourcePathHash: sha256("android-source"),
        sourceHint: "android/native/app/build/outputs/apk/debug/app-debug.apk"
      }
    ]
  }, null, 2)}\n`);
  const nestedPlan = buildClientArtifactInstallPlan({
    artifactsRoot: nestedRoot,
    installPrereqs: {
      adbAvailable: true,
      windowsInstallerLaunchSupported: true
    }
  });
  assert(nestedPlan.artifactManifest.manifestHint === "clients/artifacts/install-plan-test/android/docker-android-debug/manifest.json", "nested manifest discovery failed");
  assert(nestedPlan.targets.android.readyForInstall === true, "nested android artifact should be ready");

  const missingAdbPlan = buildClientArtifactInstallPlan({
    manifest: join(nestedDir, "manifest.json"),
    installPrereqs: {
      adbAvailable: false,
      windowsInstallerLaunchSupported: true
    }
  });
  assert(missingAdbPlan.targets.android.artifactReady === true, "android artifact should remain ready when adb is missing");
  assert(missingAdbPlan.targets.android.readyForInstall === false, "android install should not be ready without adb");
  assert(missingAdbPlan.targets.android.missing.includes("adb"), "android missing list should include adb");
  assert(missingAdbPlan.targets.android.installPrereqs.adbAvailable === false, "android adb prereq should be false");

  const emptyPlan = buildClientArtifactInstallPlan({
    artifactsRoot: join(runDir, "missing-artifacts"),
    installPrereqs: {
      adbAvailable: false,
      windowsInstallerLaunchSupported: false
    }
  });
  assert(emptyPlan.artifactManifest.present === false, "empty plan should report missing manifest");
  assertInstallPlanExecutionPolicy(emptyPlan.executionPolicy);
  assert(emptyPlan.targets.android.missing.includes("artifact-manifest"), "empty android plan should miss manifest");
  assert(emptyPlan.targets.android.missing.includes("adb"), "empty android plan should include adb prereq");
  assert(emptyPlan.targets["windows-desktop"].missing.includes("windows-installer-launch"), "empty desktop plan should include installer launch prereq");
  const emptyAndroidBuildStep = emptyPlan.targets.android.checklist.find(item => item.step === "build-and-collect-artifact");
  assert(emptyAndroidBuildStep?.manualOnly === true, "empty android build step should be manual-only");
  assert(emptyAndroidBuildStep.buildsNativeArtifacts === true, "empty android build step should be marked as building artifacts");
  assert(emptyAndroidBuildStep.requiresExplicitUserAction === true, "empty android build step should require explicit user action");

  writeFileSync(join(runDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    runId: "install-plan-test",
    artifacts: [
      {
        target: "android",
        filename: "nexusim-android-debug.apk",
        bytes: Buffer.byteLength(apk),
        sha256: sha256("wrong"),
        sourcePathHash: sha256("android-source"),
        sourceHint: "android/native/app/build/outputs/apk/debug/app-debug.apk"
      }
    ]
  }, null, 2)}\n`);

  let rejectedHashMismatch = false;
  try {
    buildClientArtifactInstallPlan({
      manifest: join(runDir, "manifest.json"),
      installPrereqs: {
        adbAvailable: true,
        windowsInstallerLaunchSupported: true
      }
    });
  } catch {
    rejectedHashMismatch = true;
  }
  assert(rejectedHashMismatch, "install plan should reject artifact hash mismatch");

  console.log("client artifact install plan ok");
} finally {
  rmSync(runDir, { recursive: true, force: true });
}

function assertInstallPlanExecutionPolicy(policy) {
  assert(policy?.planOnly === true, "install plan should be marked plan-only");
  assert(policy.executesChecklistCommands === false, "install plan should not execute checklist commands");
  assert(policy.buildsNativeArtifacts === false, "install plan should not build native artifacts");
  assert(policy.installsArtifacts === false, "install plan should not install artifacts");
  assert(policy.launchesDesktopArtifacts === false, "install plan should not launch desktop artifacts");
  assert(policy.startsDeviceActivities === false, "install plan should not start device activities");
  assert(policy.opensAdbReverse === false, "install plan should not open adb reverse");
  assert(policy.startsDocker === false, "install plan should not start Docker");
  assert(policy.contactsDevices === false, "install plan should not contact devices");
  assert(policy.downloadsToolchain === false, "install plan should not download toolchains");
  assert(policy.readsLocalInstallPrereqs === true, "install plan should only read local install prerequisites");
}
