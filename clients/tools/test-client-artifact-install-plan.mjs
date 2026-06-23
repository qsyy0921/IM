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
  const exe = "fake exe bytes";
  const launcher = "$ErrorActionPreference = 'Stop'\nStart-Process -FilePath (Join-Path $PSScriptRoot 'nexusim-windows-desktop.exe')\n";
  writeFileSync(join(runDir, "nexusim-android-debug.apk"), apk);
  writeFileSync(join(runDir, "nexusim-windows-desktop.exe"), exe);
  writeFileSync(join(runDir, "launch-nexusim-windows.ps1"), launcher);
  writeFileSync(join(runDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-22T00:00:00.000Z",
    gitCommit: "test",
    runId: "install-plan-test",
    artifacts: [
      {
        target: "android",
        artifactKind: "android-debug-apk",
        filename: "nexusim-android-debug.apk",
        bytes: Buffer.byteLength(apk),
        sha256: sha256(apk),
        sourcePathHash: sha256("android-source"),
        sourceHint: "android/native/app/build/outputs/apk/debug/app-debug.apk"
      },
      {
        target: "windows-desktop",
        artifactKind: "desktop-executable",
        filename: "nexusim-windows-desktop.exe",
        bytes: Buffer.byteLength(exe),
        sha256: sha256(exe),
        sourcePathHash: sha256("desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/nexusim.exe"
      }
    ],
    supportFiles: [
      {
        target: "windows-desktop",
        filename: "launch-nexusim-windows.ps1",
        bytes: Buffer.byteLength(launcher),
        sha256: sha256(launcher)
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
  assert(plan.targets.android.installMode === "android-apk", "android install mode mismatch");
  assert(plan.targets.android.artifact.artifactKind === "android-debug-apk", "android artifact kind mismatch");
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
  assert(plan.targets["windows-desktop"].installMode === "portable-executable", "desktop executable install mode mismatch");
  assert(plan.targets["windows-desktop"].artifact.artifactKind === "desktop-executable", "desktop executable artifact kind mismatch");
  assert(plan.targets["windows-desktop"].installPrereqs.windowsInstallerLaunchSupported === true, "desktop install prereq should be true");
  assert(plan.targets["windows-desktop"].supportFiles.length === 1, "desktop support files should be exposed");
  assert(plan.targets["windows-desktop"].supportFiles[0].supportHint === "clients/artifacts/install-plan-test/launch-nexusim-windows.ps1", "desktop support hint mismatch");
  const desktopLaunchStep = plan.targets["windows-desktop"].checklist.find(item => item.step === "launch-desktop-artifact");
  assert(desktopLaunchStep, "desktop launch step missing");
  assert(desktopLaunchStep.command === "powershell -NoProfile -File clients/artifacts/install-plan-test/launch-nexusim-windows.ps1", "desktop launcher command missing");
  assert(desktopLaunchStep.manualOnly === true, "desktop launch step should be manual-only");
  assert(desktopLaunchStep.launchesDesktopArtifact === true, "desktop launch step should be marked as launching the artifact");
  assert(desktopLaunchStep.startsLocalProcess === true, "desktop launch step should be marked as starting a local process");
  assert(desktopLaunchStep.requiresExplicitUserAction === true, "desktop launch step should require explicit user action");
  assert(!plan.targets["windows-desktop"].checklist.some(item => item.step === "install-desktop-installer"), "desktop executable plan should not use installer checklist");
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
        artifactKind: "android-debug-apk",
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

  const installer = "fake msi bytes";
  writeFileSync(join(runDir, "nexusim-windows-desktop-installer.msi"), installer);
  writeFileSync(join(runDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-22T00:00:00.000Z",
    gitCommit: "test",
    runId: "install-plan-test",
    artifacts: [
      {
        target: "windows-desktop",
        artifactKind: "desktop-installer",
        filename: "nexusim-windows-desktop-installer.msi",
        bytes: Buffer.byteLength(installer),
        sha256: sha256(installer),
        sourcePathHash: sha256("desktop-installer-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      }
    ]
  }, null, 2)}\n`);
  const installerPlan = buildClientArtifactInstallPlan({
    manifest: join(runDir, "manifest.json"),
    installPrereqs: {
      adbAvailable: true,
      windowsInstallerLaunchSupported: true
    }
  });
  assert(installerPlan.targets["windows-desktop"].artifactReady === true, "desktop installer artifact should be ready");
  assert(installerPlan.targets["windows-desktop"].readyForInstall === false, "unsigned desktop installer install plan must not be ready");
  assert(installerPlan.targets["windows-desktop"].installMode === "signed-installer", "desktop installer install mode mismatch");
  assert(installerPlan.targets["windows-desktop"].artifact.artifactKind === "desktop-installer", "desktop installer artifact kind mismatch");
  assert(installerPlan.targets["windows-desktop"].installerSignatureVerification.readyForSignedDistribution === false, "desktop installer signature verification should block unsigned fixtures");
  assert(
    installerPlan.targets["windows-desktop"].missing.includes("valid-authenticode-signature") ||
      installerPlan.targets["windows-desktop"].missing.includes("windows-authenticode"),
    "unsigned desktop installer missing list should require Authenticode verification"
  );
  assert(installerPlan.targets["windows-desktop"].checklist.some(item => item.step === "verify-desktop-installer-signature"), "desktop installer signature verification step missing");
  const installerStep = installerPlan.targets["windows-desktop"].checklist.find(item => item.step === "install-desktop-installer");
  assert(installerStep?.command === "Start-Process clients/artifacts/install-plan-test/nexusim-windows-desktop-installer.msi", "desktop installer command mismatch");
  assert(installerStep.installsArtifacts === true, "desktop installer step should install artifacts");
  assert(installerStep.launchesDesktopArtifact === false, "desktop installer step should not be marked as launching the app");
  assert(!installerPlan.targets["windows-desktop"].checklist.some(item => item.step === "launch-desktop-artifact"), "desktop installer plan should not use portable launch step");

  const signedInstallerPlan = buildClientArtifactInstallPlan({
    manifest: join(runDir, "manifest.json"),
    installPrereqs: {
      adbAvailable: true,
      windowsInstallerLaunchSupported: true
    },
    desktopInstallerSignatureVerification: {
      readyForSignedDistribution: true,
      missing: [],
      signature: {
        status: "Valid",
        signed: true,
        trusted: true
      },
      nextAction: "continue with signed installer install"
    }
  });
  assert(signedInstallerPlan.targets["windows-desktop"].readyForInstall === true, "valid signed desktop installer install plan should be ready");
  assert(signedInstallerPlan.targets["windows-desktop"].missing.length === 0, "valid signed desktop installer should have no missing inputs");
  assert(signedInstallerPlan.targets["windows-desktop"].installerSignatureVerification.status === "Valid", "signed desktop installer status should be preserved");

  writeFileSync(join(runDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-22T00:00:00.000Z",
    gitCommit: "test",
    runId: "install-plan-test",
    artifacts: [
      {
        target: "windows-desktop",
        artifactKind: "desktop-installer",
        filename: "nexusim-windows-desktop-installer.msi",
        bytes: Buffer.byteLength(installer),
        sha256: sha256(installer),
        sourcePathHash: sha256("desktop-installer-source"),
        sourceHint: "desktop/src-tauri/target/release/bundle/msi/nexusim.msi"
      },
      {
        target: "windows-desktop",
        artifactKind: "desktop-executable",
        filename: "nexusim-windows-desktop.exe",
        bytes: Buffer.byteLength(exe),
        sha256: sha256(exe),
        sourcePathHash: sha256("desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/nexusim.exe"
      }
    ]
  }, null, 2)}\n`);
  const mixedDefaultPlan = buildClientArtifactInstallPlan({
    manifest: join(runDir, "manifest.json"),
    installPrereqs: {
      adbAvailable: true,
      windowsInstallerLaunchSupported: true
    }
  });
  assert(mixedDefaultPlan.targets["windows-desktop"].artifact.artifactKind === "desktop-executable", "mixed desktop manifest should default to executable install path");
  assert(mixedDefaultPlan.targets["windows-desktop"].installMode === "portable-executable", "mixed desktop default install mode should be portable executable");
  assert(mixedDefaultPlan.targets["windows-desktop"].checklist.some(item => item.step === "launch-desktop-artifact"), "mixed desktop default should keep direct launch checklist");
  const mixedInstallerPlan = buildClientArtifactInstallPlan({
    manifest: join(runDir, "manifest.json"),
    artifactKind: "desktop-installer",
    installPrereqs: {
      adbAvailable: true,
      windowsInstallerLaunchSupported: true
    },
    desktopInstallerSignatureVerification: {
      readyForSignedDistribution: true,
      missing: [],
      signature: {
        status: "Valid",
        signed: true,
        trusted: true
      }
    }
  });
  assert(mixedInstallerPlan.targets["windows-desktop"].artifact.artifactKind === "desktop-installer", "explicit mixed desktop installer plan should select installer");
  assert(mixedInstallerPlan.targets["windows-desktop"].readyForInstall === true, "explicit signed mixed installer should be install-ready");
  assert(mixedInstallerPlan.targets["windows-desktop"].checklist.some(item => item.step === "install-desktop-installer"), "explicit mixed installer checklist should include install step");

  writeFileSync(join(runDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-22T00:00:00.000Z",
    gitCommit: "test",
    runId: "install-plan-test",
    artifacts: [
      {
        target: "windows-desktop",
        filename: "nexusim-windows-desktop.exe",
        bytes: Buffer.byteLength(exe),
        sha256: sha256(exe),
        sourcePathHash: sha256("legacy-desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/nexusim.exe"
      }
    ]
  }, null, 2)}\n`);
  const legacyKindPlan = buildClientArtifactInstallPlan({
    manifest: join(runDir, "manifest.json"),
    installPrereqs: {
      adbAvailable: true,
      windowsInstallerLaunchSupported: true
    }
  });
  assert(legacyKindPlan.targets["windows-desktop"].artifactReady === true, "legacy desktop artifact file should still be visible");
  assert(legacyKindPlan.targets["windows-desktop"].readyForInstall === false, "legacy desktop manifest without artifactKind must not be install-ready");
  assert(legacyKindPlan.targets["windows-desktop"].missing.includes("windows-desktop-artifact-kind"), "legacy desktop missing list should require artifact kind");
  assert(legacyKindPlan.targets["windows-desktop"].checklist.some(item => item.step === "recollect-artifact-with-kind"), "legacy desktop manifest should require recollecting with artifact kind");
  assert(!legacyKindPlan.targets["windows-desktop"].checklist.some(item => item.step === "launch-desktop-artifact"), "legacy desktop manifest should not produce a launch step");

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
        artifactKind: "android-debug-apk",
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
  assert(policy.readsDesktopInstallerSignature === true, "install plan should declare desktop installer signature reads");
}
