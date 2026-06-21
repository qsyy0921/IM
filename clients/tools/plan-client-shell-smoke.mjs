import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { buildReadinessReport } from "./report-client-artifact-readiness.mjs";
import { collectPlan, parseArgs as parseCollectArgs } from "./collect-client-artifacts.mjs";
import { buildClientArtifactInstallPlan } from "./plan-client-artifact-install.mjs";

const smokeSchemaVersion = "nexusim.client-shell-smoke-plan.v1";

function main() {
  process.stdout.write(`${JSON.stringify(buildClientShellSmokePlan(), null, 2)}\n`);
}

export function buildClientShellSmokePlan(options = {}) {
  const readiness = options.readiness ?? buildReadinessReport();
  const artifactPlan = options.artifactPlan ?? collectPlan(parseCollectArgs(["--target", "all", "--dry-run", "--run-id", "shell-smoke-plan"]));
  const installPlan = options.installPlan ?? buildClientArtifactInstallPlan();
  return {
    schemaVersion: smokeSchemaVersion,
    generatedAt: new Date().toISOString(),
    targets: {
      browser: browserTarget(),
      "windows-desktop": nativeTarget("windows-desktop", readiness.targets["windows-desktop"], artifactPlan, installPlan),
      android: nativeTarget("android", readiness.targets.android, artifactPlan, installPlan)
    },
    sharedSmoke: {
      backendCommand: "loadtest/clientweb/run-local-smoke.ps1 -BindHost 127.0.0.1 -ClientHost 127.0.0.1",
      wiredLanExample: "loadtest/clientweb/run-local-smoke.ps1 -BindHost 172.31.50.1 -ClientHost 172.31.50.1",
      evidence: "docs/runbook/loadtest/client-platform/"
    }
  };
}

function browserTarget() {
  return {
    readyForManualShellSmoke: true,
    launchCommand: "npm --prefix clients run dev:web",
    expectedBackend: "api-gateway BFF + push-gateway WebSocket",
    checklist: [
      {
        step: "verify-shell-lifecycle-contract",
        command: "npm --prefix clients run test:web-shell-actions",
        evidence: "Web shell binds login, refresh, restore and logout through shared ClientShellActions"
      },
      {
        step: "start-shared-backend-smoke",
        command: "loadtest/clientweb/run-local-smoke.ps1 -BindHost 127.0.0.1 -ClientHost 127.0.0.1",
        evidence: "clientweb summary shows BFF login, push hello, SendMessage, PullInbox and AckDelivery"
      },
      {
        step: "start-web-shell",
        command: "npm --prefix clients run dev:web",
        evidence: "browser opens the Web shell against api-gateway BFF and push-gateway"
      },
      {
        step: "verify-client-flow",
        evidence: "shell can sign in, open a conversation, send, receive delivery.notify, pull inbox and ack"
      }
    ],
    notes: [
      "Browser smoke uses the existing Web shell and clientweb runner.",
      "It does not prove PC installer or Android APK packaging."
    ]
  };
}

function nativeTarget(target, readinessTarget, artifactPlan, installPlan) {
  const installStatus = installStatusFor(target, installPlan);
  const artifactStatus = artifactStatusFor(target, artifactPlan, installStatus);
  const readyForManualShellSmoke = Boolean(
    readinessTarget?.ready &&
    readinessTarget?.shellAssets?.verified &&
    installStatus.readyForInstall
  );
  return {
    readyForManualShellSmoke,
    shellAssets: readinessTarget.shellAssets,
    nativeToolchainReady: readinessTarget.ready,
    missingToolchain: readinessTarget.missing,
    artifact: artifactStatus,
    install: installStatus,
    commands: nativeCommands(target, readinessTarget),
    checklist: nativeChecklist(target, readinessTarget, artifactStatus, installStatus),
    notes: nativeNotes(target, readinessTarget, artifactStatus, installStatus)
  };
}

function artifactStatusFor(target, artifactPlan, installStatus) {
  const sources = artifactPlan.sources
    .filter(source => source.target === target)
    .map(source => ({
      sourceHint: source.sourceHint,
      outputFilename: source.outputFilename
    }));
  const missing = artifactPlan.missing
    .filter(entry => entry.target === target)
    .flatMap(entry => entry.expected);
  const buildOutputPresent = sources.length > 0;
  const collectedArtifactReady = Boolean(installStatus.artifactReady);
  return {
    present: buildOutputPresent || collectedArtifactReady,
    buildOutputPresent,
    collectedArtifactReady,
    collectedArtifactHint: installStatus.artifactHint,
    sources,
    missing
  };
}

function installStatusFor(target, installPlan) {
  const targetPlan = installPlan.targets?.[target];
  if (!targetPlan) {
    return {
      artifactReady: false,
      readyForInstall: false,
      missing: ["install-plan-target"],
      installPrereqs: {}
    };
  }
  return {
    artifactReady: Boolean(targetPlan.artifactReady),
    readyForInstall: Boolean(targetPlan.readyForInstall),
    missing: Array.isArray(targetPlan.missing) ? targetPlan.missing : [],
    installPrereqs: targetPlan.installPrereqs ?? {},
    artifactHint: targetPlan.artifact?.artifactHint ?? ""
  };
}

function nativeCommands(target, readinessTarget) {
  if (target === "windows-desktop") {
    return {
      prepareAssets: "npm --prefix clients run build:shell-assets:desktop",
      verifyAssets: "node clients/tools/verify-shell-assets.mjs --target windows-desktop",
      buildArtifact: readinessTarget.buildCommand,
      dryRunBuild: readinessTarget.dryRunCommand,
      installPlan: "npm --prefix clients run plan:artifact-install",
      launchSmoke: "npm --prefix clients run smoke:desktop-artifact-launch",
      composedSmoke: "npm --prefix clients run smoke:desktop-composed -- --clientweb-summary <client-web-summary.json>",
      webviewMetadataSmoke: "npm --prefix clients run smoke:desktop-webview-metadata",
      webviewLoginSmoke: ".\\loadtest\\clientweb\\run-local-smoke.ps1 -RunDesktopWebViewLoginSmoke"
    };
  }
  return {
    prepareAssets: "npm --prefix clients run build:shell-assets:android",
    verifyAssets: "node clients/tools/verify-shell-assets.mjs --target android",
    buildArtifact: readinessTarget.buildCommand,
    dryRunBuild: readinessTarget.dryRunCommand,
    installPlan: "npm --prefix clients run plan:artifact-install",
    deviceReadiness: "npm --prefix clients run report:android-device-readiness",
    webviewMetadataSmoke: "npm --prefix clients run smoke:android-webview-metadata",
    webviewLoginSmokePlan: "npm --prefix clients run plan:android-webview-login-smoke",
    webviewLoginSmoke: "npm --prefix clients run smoke:android-webview-login -- --fixture <clientweb-fixture.json>",
    dockerBuilder: readinessTarget.dockerBuilder?.imagePresent
      ? readinessTarget.dockerBuilder.buildCommand
      : readinessTarget.dockerBuilder?.imageBuildCommand,
    dockerBuilderDryRun: readinessTarget.dockerBuilder?.safeDryRunCommand
  };
}

function nativeChecklist(target, readinessTarget, artifactStatus, installStatus) {
  const label = target === "windows-desktop" ? "desktop" : "android";
  const checklist = [
    {
      step: "verify-shell-lifecycle-contract",
      command: "npm --prefix clients run test:web-shell-actions",
      evidence: "shared WebView shell action contract is guarded before platform smoke"
    },
    {
      step: "prepare-shell-assets",
      command: nativeCommands(target, readinessTarget).prepareAssets,
      evidence: `${label} shell asset manifest is generated`
    },
    {
      step: "verify-shell-assets",
      command: nativeCommands(target, readinessTarget).verifyAssets,
      evidence: `${label} shell asset manifest verifies before native build`
    }
  ];

  if (target === "android") {
    checklist.push({
      step: "check-android-device-readiness",
      command: nativeCommands(target, readinessTarget).deviceReadiness,
      evidence: "adb is available and at least one authorized Android device is visible without exposing raw serial or model"
    });
  }

  if (!readinessTarget.ready) {
    if (target === "windows-desktop" && missingDesktopTauriCLI(readinessTarget)) {
      checklist.push({
        step: "install-declared-desktop-tauri-cli",
        command: "npm --prefix clients install",
        evidence: "repo-declared @tauri-apps/cli is installed into clients/node_modules before artifact build"
      });
    }
    checklist.push({
      step: "resolve-native-toolchain",
      command: nativeCommands(target, readinessTarget).dryRunBuild,
      evidence: "dry-run reports remaining toolchain gaps without downloading tools"
    });
    if (target === "android" && readinessTarget.dockerBuilder) {
      checklist.push({
        step: readinessTarget.dockerBuilder.imagePresent ? "run-android-builder" : "build-android-builder-image",
        command: nativeCommands(target, readinessTarget).dockerBuilder,
        evidence: "Android builder path produces a collected APK manifest when explicitly run"
      });
    }
    return checklist;
  }

  checklist.push({
    step: target === "windows-desktop" ? "build-desktop-artifact" : "build-android-apk",
    command: nativeCommands(target, readinessTarget).buildArtifact,
    evidence: "native artifact is collected with a SHA-256 manifest"
  });

  checklist.push({
    step: "plan-artifact-install",
    command: nativeCommands(target, readinessTarget).installPlan,
    evidence: "install plan reports low-sensitive commands for the collected artifact"
  });

  if (!artifactStatus.present && !installStatus.artifactReady) {
    checklist.push({
      step: "collect-native-artifact",
      evidence: "artifact collector finds at least one native package for this target"
    });
  }

  if (!installStatus.readyForInstall) {
    checklist.push({
      step: "resolve-install-prereqs",
      command: nativeCommands(target, readinessTarget).installPlan,
      evidence: "install plan reports no missing artifact or install-side prerequisites"
    });
    return checklist;
  }

  if (target === "windows-desktop") {
    checklist.push({
      step: "launch-desktop-artifact-smoke",
      command: nativeCommands(target, readinessTarget).launchSmoke,
      evidence: "desktop artifact process starts, stays alive during the hold window and terminates cleanly"
    });
    checklist.push({
      step: "run-desktop-composed-smoke",
      command: nativeCommands(target, readinessTarget).composedSmoke,
      evidence: "clientweb BFF/push summary and desktop artifact launch evidence are combined into one low-sensitive desktop composed smoke result"
    });
    checklist.push({
      step: "run-desktop-webview-metadata-smoke",
      command: nativeCommands(target, readinessTarget).webviewMetadataSmoke,
      evidence: "Tauri WebView loads the prepared shell, reads runtime_metadata via native bridge and posts low-sensitive metadata to a loopback callback"
    });
    checklist.push({
      step: "run-desktop-webview-login-smoke",
      command: nativeCommands(target, readinessTarget).webviewLoginSmoke,
      evidence: "Tauri WebView is externally driven through login, delivery.notify, PullInbox and AckDelivery while the clientweb local stack is alive"
    });
  } else {
    checklist.push({
      step: "run-android-webview-metadata-smoke",
      command: nativeCommands(target, readinessTarget).webviewMetadataSmoke,
      evidence: "Android WebView loads the prepared shell, reads NexusIMNative runtime metadata and posts a low-sensitive callback through adb reverse"
    });
    checklist.push({
      step: "plan-android-webview-login-smoke",
      command: nativeCommands(target, readinessTarget).webviewLoginSmokePlan,
      evidence: "login-level Android WebView smoke prerequisites, selector contract and low-sensitive execution steps are explicit before a real runner is enabled"
    });
    checklist.push({
      step: "run-android-webview-login-smoke",
      command: nativeCommands(target, readinessTarget).webviewLoginSmoke,
      evidence: "Android WebView is externally driven through login, delivery.notify, PullInbox and AckDelivery while the clientweb local stack is alive"
    });
  }

  checklist.push({
    step: "run-platform-shell",
    evidence: `shell metadata reports target=${target} and the app can sign in, pull inbox, receive wakeup and ack`
  });
  return checklist;
}

function missingDesktopTauriCLI(readinessTarget) {
  const missing = readinessTarget.missing ?? [];
  return missing.some(item => item.name === "local:tauri" || item.name === "cargo tauri");
}

function nativeNotes(target, readinessTarget, artifactStatus, installStatus) {
  const notes = [];
  if (!readinessTarget.shellAssets?.verified) {
    notes.push("Prepare and verify shell assets before native smoke.");
  }
  if (!readinessTarget.ready) {
    notes.push("Native toolchain is not ready; run the dry-run command or install/build the reported toolchain first.");
  }
  if (!artifactStatus.present && !installStatus.artifactReady) {
    notes.push("No native artifact is present yet; build and collect one before manual shell smoke.");
  }
  if (installStatus.artifactReady && !installStatus.readyForInstall) {
    notes.push("A collected artifact exists, but the install plan still reports missing install-side prerequisites.");
  }
  if (target === "android" && readinessTarget.dockerBuilder && !readinessTarget.ready) {
    notes.push("Android can use the opt-in Docker builder path; the first image build may download toolchains.");
  }
  return notes;
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
