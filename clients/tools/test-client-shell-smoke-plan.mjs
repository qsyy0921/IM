import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { buildClientShellSmokePlan } from "./plan-client-shell-smoke.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const planScript = join(toolsDir, "plan-client-shell-smoke.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const output = execFileSync(process.execPath, [planScript], {
  encoding: "utf8"
});
const plan = JSON.parse(output);
const serialized = JSON.stringify(plan);

assert(plan.schemaVersion === "nexusim.client-shell-smoke-plan.v1", "shell smoke plan schema mismatch");
assert(plan.targets.browser.readyForManualShellSmoke === true, "browser shell smoke should be available");
assert(plan.targets.browser.launchCommand.includes("dev:web"), "browser launch command missing");
assert(Array.isArray(plan.targets.browser.checklist) && plan.targets.browser.checklist.length >= 3, "browser checklist missing");
assert(plan.targets.browser.checklist.some(item => item.step === "verify-shell-lifecycle-contract"), "browser shell lifecycle contract check missing");
assert(plan.targets.browser.checklist.some(item => item.step === "verify-client-flow"), "browser flow verification missing");
assert(plan.targets["windows-desktop"].commands.prepareAssets.includes("build:shell-assets:desktop"), "desktop prep command missing");
assert(plan.targets["windows-desktop"].commands.verifyAssets.includes("windows-desktop"), "desktop verify command missing");
assert(plan.targets["windows-desktop"].commands.installPlan.includes("plan:artifact-install"), "desktop install plan command missing");
assert(plan.targets["windows-desktop"].install, "desktop install status missing");
assert(plan.targets["windows-desktop"].localStore?.nativeStoreReadiness?.bridge === "tauri-sqlite", "desktop local store readiness missing");
assert(plan.targets["windows-desktop"].notes.some(note => note.includes("Native SQLite local store is not ready")), "desktop native sqlite note missing");
assert(typeof plan.targets["windows-desktop"].install.readyForInstall === "boolean", "desktop install readiness missing");
assert(Array.isArray(plan.targets["windows-desktop"].install.missing), "desktop install missing list missing");
assert(Array.isArray(plan.targets["windows-desktop"].checklist), "desktop checklist missing");
assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "verify-shell-lifecycle-contract"), "desktop shell lifecycle contract check missing");
const desktopMissingToolchain = plan.targets["windows-desktop"].missingToolchain ?? [];
if (plan.targets["windows-desktop"].nativeToolchainReady) {
  assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "build-desktop-artifact"), "desktop artifact build checklist missing when ready");
  if (plan.targets["windows-desktop"].readyForManualShellSmoke) {
    assert(plan.targets["windows-desktop"].commands.launchSmoke?.includes("smoke:desktop-artifact-launch"), "desktop launch smoke command missing when smoke-ready");
    assert(plan.targets["windows-desktop"].commands.composedSmoke?.includes("smoke:desktop-composed"), "desktop composed smoke command missing when smoke-ready");
    assert(plan.targets["windows-desktop"].commands.webviewMetadataSmoke?.includes("smoke:desktop-webview-metadata"), "desktop WebView metadata smoke command missing when smoke-ready");
    assert(plan.targets["windows-desktop"].commands.webviewLoginSmoke?.includes("-RunDesktopWebViewLoginSmoke"), "desktop WebView login smoke command missing when smoke-ready");
    assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "launch-desktop-artifact-smoke"), "desktop artifact launch smoke checklist missing when smoke-ready");
    assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "run-desktop-composed-smoke"), "desktop composed smoke checklist missing when smoke-ready");
    assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "run-desktop-webview-metadata-smoke"), "desktop WebView metadata smoke checklist missing when smoke-ready");
    assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "run-desktop-webview-login-smoke"), "desktop WebView login smoke checklist missing when smoke-ready");
    assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "run-platform-shell"), "desktop platform shell checklist missing when smoke-ready");
  } else {
    const desktopTarget = plan.targets["windows-desktop"];
    if (!desktopTarget.shellAssets?.verified) {
      assert(desktopTarget.checklist.some(item => item.step === "prepare-shell-assets"), "desktop asset prep checklist missing when shell assets are not smoke-ready");
      assert(desktopTarget.checklist.some(item => item.step === "verify-shell-assets"), "desktop asset verify checklist missing when shell assets are not smoke-ready");
    }
    if (!desktopTarget.artifact.present) {
      assert(desktopTarget.checklist.some(item => item.step === "collect-native-artifact"), "desktop artifact collection checklist missing when artifact is not smoke-ready");
    }
    if (!desktopTarget.install.readyForInstall) {
      assert(desktopTarget.checklist.some(item => item.step === "resolve-install-prereqs"), "desktop install-prereq checklist missing when install is not smoke-ready");
    }
  }
} else {
  if (desktopMissingToolchain.some(item => item.name === "local:tauri")) {
    assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "install-declared-desktop-tauri-cli"), "desktop repo-local Tauri CLI install checklist missing when local CLI is absent");
  }
  assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "resolve-native-toolchain"), "desktop native toolchain resolution checklist missing when not ready");
}
assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "prepare-shell-assets"), "desktop asset prep checklist missing");
assert(plan.targets["windows-desktop"].checklist.some(item => item.step === "verify-shell-assets"), "desktop asset verification checklist missing");
assert(plan.targets.android.commands.prepareAssets.includes("build:shell-assets:android"), "android prep command missing");
assert(plan.targets.android.commands.verifyAssets.includes("android"), "android verify command missing");
assert(plan.targets.android.commands.installPlan.includes("plan:artifact-install"), "android install plan command missing");
assert(plan.targets.android.commands.deviceReadiness.includes("report:android-device-readiness"), "android device readiness command missing");
assert(plan.targets.android.commands.webviewDevtoolsReadiness.includes("report:android-webview-devtools-readiness"), "android WebView devtools readiness command missing");
assert(plan.targets.android.commands.webviewMetadataSmoke.includes("smoke:android-webview-metadata"), "android WebView metadata smoke command missing");
assert(plan.targets.android.install, "android install status missing");
assert(plan.targets.android.localStore?.nativeStoreReadiness?.bridge === "android-sqlite", "android local store readiness missing");
assert(plan.targets.android.notes.some(note => note.includes("Native SQLite local store is not ready")), "android native sqlite note missing");
assert(typeof plan.targets.android.install.readyForInstall === "boolean", "android install readiness missing");
assert(typeof plan.targets.android.install.installPrereqs.adbAvailable === "boolean", "android adb prereq status missing");
assert(Array.isArray(plan.targets.android.install.missing), "android install missing list missing");
assert(Array.isArray(plan.targets.android.checklist), "android checklist missing");
assert(plan.targets.android.checklist.some(item => item.step === "verify-shell-lifecycle-contract"), "android shell lifecycle contract check missing");
assert(plan.targets.android.checklist.some(item => item.step === "check-android-device-readiness"), "android device readiness checklist missing");
assert(plan.targets.android.checklist.some(item => item.step === "check-android-webview-devtools-readiness"), "android WebView devtools readiness checklist missing");
assert(plan.targets.android.checklist.some(item => item.step === "prepare-shell-assets"), "android asset prep checklist missing");
assert(plan.targets.android.checklist.some(item => item.step === "verify-shell-assets"), "android asset verification checklist missing");
assert(Array.isArray(plan.targets.android.notes), "android notes missing");
assert(plan.sharedSmoke.backendCommand.includes("run-local-smoke.ps1"), "shared backend smoke command missing");
assert(plan.sharedSmoke.wiredLanExample.includes("172.31.50.1"), "wired LAN smoke example missing");
assert(!serialized.match(/token|secret|password|credential|private/i), "shell smoke plan leaked sensitive names");
assert(!serialized.match(/[A-Z]:\\\\/), "shell smoke plan leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "shell smoke plan leaked extended Windows path");

const readyReadiness = {
  targets: {
    "windows-desktop": {
      ready: true,
      shellAssets: {
        verified: true,
        fileCount: 4
      },
      missing: [],
      buildCommand: "npm --prefix clients run build:desktop-artifact:collect",
      dryRunCommand: "node clients/tools/build-desktop-artifact.mjs --dry-run --collect",
      localStore: {
        currentDefault: "local-storage",
        productionTarget: "sqlite",
        nativeStoreReadiness: {
          target: "windows-desktop",
          requestedStore: "sqlite",
          ready: false,
          reason: "sqlite-native-bridge-unavailable",
          bridge: "tauri-sqlite",
          nextAction: "tauri-sqlite is required before windows-desktop can use sqlite local store"
        },
        currentSmokeStore: "local-storage"
      }
    },
    android: {
      ready: true,
      shellAssets: {
        verified: true,
        fileCount: 4
      },
      missing: [],
      buildCommand: "npm --prefix clients run build:android-apk:collect",
      dryRunCommand: "node clients/tools/build-android-apk.mjs --dry-run --collect",
      localStore: {
        currentDefault: "local-storage",
        productionTarget: "sqlite",
        nativeStoreReadiness: {
          target: "android",
          requestedStore: "sqlite",
          ready: false,
          reason: "sqlite-native-bridge-unavailable",
          bridge: "android-sqlite",
          nextAction: "android-sqlite is required before android can use sqlite local store"
        },
        currentSmokeStore: "local-storage"
      }
    }
  }
};
const noBuildOutputArtifactPlan = {
  sources: [],
  missing: [
    {
      target: "windows-desktop",
      expected: ["desktop/src-tauri/target/release/bundle"]
    },
    {
      target: "android",
      expected: ["android/native/app/build/outputs/apk/debug/app-debug.apk"]
    }
  ]
};
const collectedInstallPlan = {
  targets: {
    "windows-desktop": {
      artifactReady: true,
      readyForInstall: true,
      missing: [],
      installPrereqs: {
        windowsInstallerLaunchSupported: true
      },
      artifact: {
        artifactHint: "clients/artifacts/run/nexusim-windows-desktop.msi"
      }
    },
    android: {
      artifactReady: true,
      readyForInstall: true,
      missing: [],
      installPrereqs: {
        adbAvailable: true
      },
      artifact: {
        artifactHint: "clients/artifacts/run/nexusim-android-debug.apk"
      }
    }
  }
};
const readyFromCollectedPlan = buildClientShellSmokePlan({
  readiness: readyReadiness,
  artifactPlan: noBuildOutputArtifactPlan,
  installPlan: collectedInstallPlan
});
assert(readyFromCollectedPlan.targets["windows-desktop"].readyForManualShellSmoke === true, "desktop should be smoke-ready from collected artifact");
assert(readyFromCollectedPlan.targets["windows-desktop"].artifact.present === true, "desktop artifact should be present from collected manifest");
assert(readyFromCollectedPlan.targets["windows-desktop"].artifact.buildOutputPresent === false, "desktop build output should remain false");
assert(readyFromCollectedPlan.targets["windows-desktop"].artifact.collectedArtifactReady === true, "desktop collected artifact should be ready");
assert(readyFromCollectedPlan.targets["windows-desktop"].commands.webviewLoginSmoke.includes("-RunDesktopWebViewLoginSmoke"), "ready desktop plan should include WebView login smoke command");
assert(readyFromCollectedPlan.targets["windows-desktop"].checklist.some(item => item.step === "run-desktop-webview-login-smoke"), "ready desktop plan should include WebView login smoke step");
assert(readyFromCollectedPlan.targets.android.readyForManualShellSmoke === true, "android should be smoke-ready from collected artifact");
assert(readyFromCollectedPlan.targets.android.artifact.collectedArtifactHint === "clients/artifacts/run/nexusim-android-debug.apk", "android collected artifact hint mismatch");
assert(readyFromCollectedPlan.targets.android.checklist.some(item => item.step === "run-android-webview-metadata-smoke"), "ready android plan should include WebView metadata smoke step");

const missingAdbPlan = buildClientShellSmokePlan({
  readiness: readyReadiness,
  artifactPlan: noBuildOutputArtifactPlan,
  installPlan: {
    targets: {
      "windows-desktop": collectedInstallPlan.targets["windows-desktop"],
      android: {
        ...collectedInstallPlan.targets.android,
        readyForInstall: false,
        missing: ["adb"],
        installPrereqs: {
          adbAvailable: false
        }
      }
    }
  }
});
assert(missingAdbPlan.targets.android.artifact.collectedArtifactReady === true, "android collected artifact should remain ready when adb is missing");
assert(missingAdbPlan.targets.android.readyForManualShellSmoke === false, "android smoke should not be ready without adb");
assert(missingAdbPlan.targets.android.checklist.some(item => item.step === "resolve-install-prereqs"), "android install-prereq checklist missing");

console.log("client shell smoke plan ok");
