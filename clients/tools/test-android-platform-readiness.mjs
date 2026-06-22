import { buildAndroidPlatformReadinessReport } from "./report-android-platform-readiness.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const missingToolchain = buildAndroidPlatformReadinessReport({
  prereqs: {
    androidApkReady: false,
    checks: [
      { name: "java>=17", target: "android", label: "JDK 17+", ok: false, detectedMajorVersion: 8 },
      { name: "gradle", target: "android", label: "Gradle", ok: false },
      { name: "ANDROID_HOME", target: "android", label: "Android SDK", ok: false },
      { name: "ANDROID_SDK_ROOT", target: "android", label: "Android SDK", ok: false }
    ]
  },
  dockerBuilder: {
    safeDefaultNoImageBuild: true,
    dockerAvailable: true,
    composeAvailable: true,
    profileReady: true,
    image: "nexusim/client-android-builder:local",
    imagePresent: false,
    outputHint: "clients/artifacts/android/docker-android-debug/manifest.json"
  },
  device: {
    adbAvailable: true,
    readyForInstallSmoke: true,
    counts: { total: 1, device: 1, unauthorized: 0, offline: 0, other: 0 },
    devices: [{ serialHash: "1234567890abcdef", state: "device", transport: "usb", detailKeys: ["model"] }],
    nextActions: []
  }
});

assert(missingToolchain.schemaVersion === "nexusim.android-platform-readiness.v1", "schema mismatch");
assertPlatformReadinessPolicy(missingToolchain.executionPolicy);
assert(missingToolchain.localToolchain.ready === false, "local toolchain should be false");
assert(missingToolchain.localToolchain.missing.some(item => item.name === "java>=17"), "missing JDK not reported");
assert(missingToolchain.dockerBuilder.readyToRun === false, "docker image should not be ready");
assert(missingToolchain.dockerBuilder.bootstrapDownloadsToolchain === true, "bootstrap should be marked as download-heavy");
assert(missingToolchain.device.readyForInstallSmoke === true, "device readiness mismatch");
assert(
  missingToolchain.nextActions.some(action => action.action === "bootstrap-android-docker-builder-image" && action.downloadsToolchain === true),
  "bootstrap next action missing"
);
assert(
  missingToolchain.nextActions.some(action => action.action === "run-android-webview-smoke-after-apk"),
  "device-ready smoke next action missing"
);

const readyDocker = buildAndroidPlatformReadinessReport({
  prereqs: {
    androidApkReady: false,
    checks: [
      { name: "java>=17", target: "android", label: "JDK 17+", ok: false, detectedMajorVersion: 8 }
    ]
  },
  dockerBuilder: {
    safeDefaultNoImageBuild: true,
    dockerAvailable: true,
    composeAvailable: true,
    profileReady: true,
    image: "nexusim/client-android-builder:local",
    imagePresent: true,
    outputHint: "clients/artifacts/android/docker-android-debug/manifest.json"
  },
  device: {
    adbAvailable: true,
    readyForInstallSmoke: false,
    counts: { total: 0, device: 0, unauthorized: 0, offline: 0, other: 0 },
    devices: [],
    nextActions: []
  }
});

assert(readyDocker.dockerBuilder.readyToRun === true, "existing Docker image should be runnable");
assert(readyDocker.nextActions.some(action => action.action === "run-existing-android-docker-builder"), "existing Docker next action missing");
assert(readyDocker.nextActions.some(action => action.action === "prepare-android-device"), "device prep next action missing");

const serialized = JSON.stringify([missingToolchain, readyDocker]);
assert(!serialized.match(/[A-Z]:\\\\/), "report leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "report leaked extended Windows path");
assert(!serialized.match(/token|secret|password|credential|private/i), "report leaked sensitive names");
assert(!serialized.match(/nova|huawei|honor|xiaomi|samsung|pixel/i), "report leaked device model");

console.log("Android platform readiness report ok");

function assertPlatformReadinessPolicy(policy) {
  assert(policy?.reportOnly === true, "platform readiness should be report-only");
  assert(policy.planOnly === false, "platform readiness is an actual local readiness probe");
  assert(policy.runsReadinessCommands === true, "platform readiness should run readiness commands");
  assert(policy.readsLocalToolchainState === true, "platform readiness should read local toolchain state");
  assert(policy.readsDockerBuilderState === true, "platform readiness should read Docker builder state");
  assert(policy.readsAdbDeviceList === true, "platform readiness should include adb device readiness");
  assert(policy.contactsDeviceReadOnly === true, "platform readiness should mark read-only device contact");
  assert(policy.readsWebViewDevtoolsSockets === false, "platform readiness should not read WebView devtools sockets");
  assert(policy.buildsNativeArtifacts === false, "platform readiness must not build artifacts");
  assert(policy.startsServices === false, "platform readiness must not start services");
  assert(policy.startsDocker === false, "platform readiness must not start Docker");
  assert(policy.buildsDockerImages === false, "platform readiness must not build Docker images");
  assert(policy.installsArtifacts === false, "platform readiness must not install artifacts");
  assert(policy.startsDeviceActivities === false, "platform readiness must not start activities");
  assert(policy.opensAdbReverse === false, "platform readiness must not open adb reverse");
  assert(policy.opensAdbForward === false, "platform readiness must not open adb forward");
  assert(policy.downloadsToolchain === false, "platform readiness must not download toolchains");
  assert(policy.exposesRawDeviceIdentifiers === false, "platform readiness must not expose raw device identifiers");
}
