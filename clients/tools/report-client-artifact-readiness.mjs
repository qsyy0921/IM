import { existsSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  collectClientBuildPrereqs,
  runCommand,
  workspaceRoot
} from "./client-build-env.mjs";
import { verifyShellAssets } from "./verify-shell-assets.mjs";

const androidBuilderImage = "nexusim/client-android-builder:local";
const androidBuilderCompose = "deploy/local/docker-compose.client-builders.yml";
const androidBuilderDockerfile = "deploy/docker/client-android-builder.Dockerfile";
const androidBuilderImageBuildCommand =
  "npm --prefix clients run build:android-apk:docker:bootstrap";
const androidBuilderRunCommand =
  "npm --prefix clients run build:android-apk:docker";

function main() {
  console.log(JSON.stringify(buildReadinessReport(), null, 2));
}

export function buildReadinessReport() {
  const prereqs = collectClientBuildPrereqs();
  const docker = dockerStatus();
  return {
    schemaVersion: "nexusim.client-artifact-readiness.v1",
    generatedAt: new Date().toISOString(),
    targets: {
      "windows-desktop": {
        ready: prereqs.desktopArtifactReady,
        missing: missingChecks(prereqs.checks, "desktop"),
        shellAssets: shellAssetStatus("windows-desktop"),
        localStore: localStoreReadiness("windows-desktop"),
        buildCommand: "npm --prefix clients run build:desktop-artifact:collect",
        dryRunCommand: "node clients/tools/build-desktop-artifact.mjs --dry-run --collect"
      },
      android: {
        ready: prereqs.androidApkReady,
        missing: missingChecks(prereqs.checks, "android"),
        shellAssets: shellAssetStatus("android"),
        localStore: localStoreReadiness("android"),
        buildCommand: "npm --prefix clients run build:android-apk:collect",
        dryRunCommand: "node clients/tools/build-android-apk.mjs --dry-run --collect",
        dockerBuilder: {
          composeFile: androidBuilderCompose,
          dockerfile: androidBuilderDockerfile,
          profile: "client-builders",
          service: "client-android-apk-builder",
          image: androidBuilderImage,
          profileReady: docker.composeAvailable && docker.composeFilePresent && docker.dockerfilePresent,
          dockerAvailable: docker.dockerAvailable,
          composeAvailable: docker.composeAvailable,
          imagePresent: docker.imagePresent,
          outputHint: "clients/artifacts/android/docker-android-debug/manifest.json",
          imageBuildCommand: androidBuilderImageBuildCommand,
          buildCommand: androidBuilderRunCommand,
          safeDryRunCommand: "node clients/tools/run-android-docker-builder.mjs --dry-run",
          buildImageOnlyCommand: "npm --prefix clients run build:android-apk:docker:image"
        }
      }
    },
    checks: sanitizedChecks(prereqs.checks),
    docker,
    nextActions: nextActions(prereqs, docker)
  };
}

function nextActions(prereqs, docker) {
  const actions = [];
  if (prereqs.desktopArtifactReady) {
    actions.push({
      target: "windows-desktop",
      action: "build-desktop-artifact",
      command: "npm --prefix clients run build:desktop-artifact:collect"
    });
  } else {
    actions.push({
      target: "windows-desktop",
      action: "install-declared-desktop-tauri-cli",
      missing: missingChecks(prereqs.checks, "desktop"),
      command: "npm --prefix clients install",
      downloadsToolchain: true,
      safeDryRunCommand: "node clients/tools/build-desktop-artifact.mjs --dry-run --collect"
    });
  }

  if (prereqs.androidApkReady) {
    actions.push({
      target: "android",
      action: "build-android-apk-local",
      command: "npm --prefix clients run build:android-apk:collect"
    });
  } else if (docker.dockerAvailable && docker.composeAvailable && docker.composeFilePresent && docker.dockerfilePresent) {
    actions.push({
      target: "android",
      action: docker.imagePresent ? "run-android-docker-builder" : "build-android-docker-builder-image",
      command: docker.imagePresent ? androidBuilderRunCommand : androidBuilderImageBuildCommand,
      downloadsToolchain: !docker.imagePresent,
      outputHint: "clients/artifacts/android/docker-android-debug/manifest.json"
    });
  } else {
    actions.push({
      target: "android",
      action: "install-android-toolchain",
      missing: missingChecks(prereqs.checks, "android"),
      safeDryRunCommand: "node clients/tools/build-android-apk.mjs --dry-run --collect"
    });
  }
  return actions;
}

function missingChecks(checks, target) {
  return checks
    .filter(check => check.target === target && !check.ok && !isSatisfiedByAlternative(checks, check))
    .map(check => ({
      name: check.name,
      label: check.label
    }));
}

function isSatisfiedByAlternative(checks, check) {
  if (check.name === "cargo tauri" && checkOK(checks, "local:tauri")) {
    return true;
  }
  if (check.name === "local:tauri" && checkOK(checks, "cargo tauri")) {
    return true;
  }
  if (check.name === "ANDROID_HOME" && checkOK(checks, "ANDROID_SDK_ROOT")) {
    return true;
  }
  if (check.name === "ANDROID_SDK_ROOT" && checkOK(checks, "ANDROID_HOME")) {
    return true;
  }
  return false;
}

function checkOK(checks, name) {
  return checks.some(check => check.name === name && check.ok);
}

function sanitizedChecks(checks) {
  return checks.map(check => {
    const result = {
      name: check.name,
      target: check.target,
      label: check.label,
      ok: check.ok
    };
    if (typeof check.detectedMajorVersion === "number") {
      result.detectedMajorVersion = check.detectedMajorVersion;
    }
    return result;
  });
}

function shellAssetStatus(target) {
  try {
    const result = verifyShellAssets({ target });
    return {
      verified: true,
      fileCount: result.fileCount
    };
  } catch (error) {
    return {
      verified: false,
      reason: shellAssetFailureReason(error)
    };
  }
}

function shellAssetFailureReason(error) {
  const message = error instanceof Error ? error.message : String(error);
  if (message.includes("missing shell asset manifest")) {
    return "missing-manifest";
  }
  if (message.includes("file set mismatch")) {
    return "file-set-mismatch";
  }
  if (message.includes("byte size mismatch")) {
    return "byte-size-mismatch";
  }
  if (message.includes("hash mismatch")) {
    return "hash-mismatch";
  }
  if (message.includes("leaked local path") || message.includes("leaked sensitive")) {
    return "sensitive-manifest";
  }
  return "invalid-manifest";
}

function localStoreReadiness(target) {
  const bridge = target === "windows-desktop" ? "tauri-sqlite" : "android-sqlite";
  const desktopSourceReady = target === "windows-desktop" && desktopNativeStoreSourceReady();
  const androidSourceReady = target === "android" && androidNativeStoreSourceReady();
  const ready = desktopSourceReady || androidSourceReady;
  const reason = ready ? "" : "sqlite-native-bridge-unavailable";
  return {
    currentDefault: "local-storage",
    productionTarget: "sqlite",
    nativeStoreReadiness: {
      target,
      requestedStore: "sqlite",
      ready,
      reason,
      bridge,
      nextAction: ready ? "" : `${bridge} is required before ${target} can use sqlite local store`
    },
    currentSmokeStore: ready ? "native-sqlite" : "local-storage"
  };
}

function desktopNativeStoreSourceReady() {
  try {
    const main = readFileSync(join(workspaceRoot, "desktop/src-tauri/src/main.rs"), "utf8");
    const permissions = readFileSync(join(workspaceRoot, "desktop/src-tauri/permissions/local_store.toml"), "utf8");
    return (
      main.includes('NATIVE_STORE_READY: &str = "true"') &&
      main.includes('NATIVE_STORE_REASON: &str = ""') &&
      main.includes("fn local_store_get_item(") &&
      main.includes("fn local_store_set_item(") &&
      main.includes("fn local_store_remove_item(") &&
      main.includes("rusqlite") &&
      main.includes("app_local_data_dir()") &&
      main.includes('LOCAL_STORE_KEY_PREFIX: &str = "nexusim:client-message-store:v1:"') &&
      permissions.includes('commands.allow = ["local_store_get_item", "local_store_set_item", "local_store_remove_item"]')
    );
  } catch {
    return false;
  }
}

function androidNativeStoreSourceReady() {
  try {
    const bridge = readFileSync(
      join(workspaceRoot, "android/native/app/src/main/java/com/nexusim/android/NexusIMBridge.kt"),
      "utf8"
    );
    return (
      bridge.includes("NATIVE_STORE_READY: Boolean = true") &&
      bridge.includes('NATIVE_STORE_REASON: String = ""') &&
      bridge.includes("fun localStoreGetItem(key: String): String?") &&
      bridge.includes("fun localStoreSetItem(key: String, value: String)") &&
      bridge.includes("fun localStoreRemoveItem(key: String)") &&
      bridge.includes("SQLiteOpenHelper") &&
      bridge.includes('ALLOWED_LOCAL_STORE_KEY_PREFIX: String = "nexusim:client-message-store:v1:"')
    );
  } catch {
    return false;
  }
}

function dockerStatus() {
  const dockerVersion = runCommand("docker", ["version", "--format", "{{.Server.Version}}"]);
  const composeVersion = runCommand("docker", ["compose", "version", "--short"]);
  const imageInspect = runCommand("docker", ["image", "inspect", androidBuilderImage]);
  return {
    dockerAvailable: dockerVersion.status === 0,
    composeAvailable: composeVersion.status === 0,
    composeFilePresent: existsSync(join(workspaceRoot, "..", androidBuilderCompose)),
    dockerfilePresent: existsSync(join(workspaceRoot, "..", androidBuilderDockerfile)),
    imagePresent: imageInspect.status === 0
  };
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
