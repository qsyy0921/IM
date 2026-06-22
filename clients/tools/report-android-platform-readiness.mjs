import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { collectClientBuildPrereqs } from "./client-build-env.mjs";
import { buildAndroidDeviceReadinessReport } from "./report-android-device-readiness.mjs";
import { buildAndroidDockerBuilderPlan } from "./run-android-docker-builder.mjs";

const schemaVersion = "nexusim.android-platform-readiness.v1";

function main() {
  process.stdout.write(`${JSON.stringify(buildAndroidPlatformReadinessReport(), null, 2)}\n`);
}

export function buildAndroidPlatformReadinessReport(options = {}) {
  const prereqs = options.prereqs ?? collectClientBuildPrereqs();
  const dockerBuilder = options.dockerBuilder ?? buildAndroidDockerBuilderPlan({ dryRun: true });
  const device = options.device ?? buildAndroidDeviceReadinessReport();
  const androidChecks = sanitizeAndroidChecks(prereqs.checks ?? []);
  const localApkReady = Boolean(prereqs.androidApkReady);
  const dockerImageReady = Boolean(dockerBuilder.profileReady && dockerBuilder.imagePresent);
  const report = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    target: "android",
    executionPolicy: readinessExecutionPolicy(),
    localToolchain: {
      ready: localApkReady,
      missing: androidChecks.filter(check => !check.ok).map(check => ({
        name: check.name,
        label: check.label
      })),
      checks: androidChecks
    },
    dockerBuilder: {
      readyToRun: dockerImageReady,
      safeDefaultNoImageBuild: Boolean(dockerBuilder.safeDefaultNoImageBuild),
      dockerAvailable: Boolean(dockerBuilder.dockerAvailable),
      composeAvailable: Boolean(dockerBuilder.composeAvailable),
      profileReady: Boolean(dockerBuilder.profileReady),
      image: dockerBuilder.image,
      imagePresent: Boolean(dockerBuilder.imagePresent),
      downloadsToolchainByDefault: false,
      bootstrapDownloadsToolchain: !dockerBuilder.imagePresent,
      outputHint: dockerBuilder.outputHint,
      safeDryRunCommand: "npm --prefix clients run report:android-platform-readiness",
      buildCommand: dockerBuilder.imagePresent
        ? "npm --prefix clients run build:android-apk:docker"
        : "npm --prefix clients run build:android-apk:docker:bootstrap"
    },
    device: {
      adbAvailable: Boolean(device.adbAvailable),
      readyForInstallSmoke: Boolean(device.readyForInstallSmoke),
      counts: device.counts,
      devices: device.devices,
      nextActions: device.nextActions
    },
    readiness: {
      canBuildApkLocally: localApkReady,
      canBuildApkWithExistingDockerImage: dockerImageReady,
      canRunInstallOrWebViewSmokeAfterApk: Boolean(device.readyForInstallSmoke),
      apkBaselineReady: false
    },
    nextActions: nextActions({ localApkReady, dockerImageReady, dockerBuilder, device })
  };
  assertLowSensitive(report);
  return report;
}

function readinessExecutionPolicy() {
  return {
    reportOnly: true,
    planOnly: false,
    runsReadinessCommands: true,
    readsLocalToolchainState: true,
    readsDockerBuilderState: true,
    readsAdbDeviceList: true,
    contactsDeviceReadOnly: true,
    readsWebViewDevtoolsSockets: false,
    buildsNativeArtifacts: false,
    startsServices: false,
    startsDocker: false,
    buildsDockerImages: false,
    installsArtifacts: false,
    startsDeviceActivities: false,
    opensAdbReverse: false,
    opensAdbForward: false,
    downloadsToolchain: false,
    exposesRawDeviceIdentifiers: false
  };
}

function sanitizeAndroidChecks(checks) {
  return checks
    .filter(check => check.target === "android")
    .map(check => {
      const sanitized = {
        name: check.name,
        label: check.label,
        ok: Boolean(check.ok)
      };
      if (typeof check.detectedMajorVersion === "number") {
        sanitized.detectedMajorVersion = check.detectedMajorVersion;
      }
      return sanitized;
    });
}

function nextActions({ localApkReady, dockerImageReady, dockerBuilder, device }) {
  const actions = [];
  if (localApkReady) {
    actions.push({
      action: "build-local-android-apk",
      command: "npm --prefix clients run build:android-apk:collect",
      downloadsToolchain: false
    });
  } else if (dockerImageReady) {
    actions.push({
      action: "run-existing-android-docker-builder",
      command: "npm --prefix clients run build:android-apk:docker",
      downloadsToolchain: false
    });
  } else if (dockerBuilder.profileReady) {
    actions.push({
      action: "bootstrap-android-docker-builder-image",
      command: "npm --prefix clients run build:android-apk:docker:bootstrap",
      downloadsToolchain: true,
      reason: "local JDK/Gradle/Android SDK is not ready and builder image is not present"
    });
  } else {
    actions.push({
      action: "install-android-local-toolchain",
      command: "npm --prefix clients run check:build-prereqs",
      downloadsToolchain: true
    });
  }

  if (device.readyForInstallSmoke) {
    actions.push({
      action: "run-android-webview-smoke-after-apk",
      command: "npm --prefix clients run smoke:android-webview-metadata",
      downloadsToolchain: false
    });
  } else {
    actions.push({
      action: "prepare-android-device",
      command: "npm --prefix clients run report:android-device-readiness",
      downloadsToolchain: false
    });
  }
  return actions;
}

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("Android platform readiness report leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("Android platform readiness report leaked a sensitive field name");
  }
  if (serialized.match(/nova|huawei|honor|xiaomi|samsung|pixel/i)) {
    throw new Error("Android platform readiness report leaked a device model string");
  }
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
