import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { buildReadinessReport } from "./report-client-artifact-readiness.mjs";
import { collectPlan, parseArgs as parseCollectArgs } from "./collect-client-artifacts.mjs";

const smokeSchemaVersion = "nexusim.client-shell-smoke-plan.v1";

function main() {
  process.stdout.write(`${JSON.stringify(buildClientShellSmokePlan(), null, 2)}\n`);
}

export function buildClientShellSmokePlan() {
  const readiness = buildReadinessReport();
  const artifactPlan = collectPlan(parseCollectArgs(["--target", "all", "--dry-run", "--run-id", "shell-smoke-plan"]));
  return {
    schemaVersion: smokeSchemaVersion,
    generatedAt: new Date().toISOString(),
    targets: {
      browser: browserTarget(),
      "windows-desktop": nativeTarget("windows-desktop", readiness.targets["windows-desktop"], artifactPlan),
      android: nativeTarget("android", readiness.targets.android, artifactPlan)
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

function nativeTarget(target, readinessTarget, artifactPlan) {
  const artifactStatus = artifactStatusFor(target, artifactPlan);
  const readyForManualShellSmoke = Boolean(
    readinessTarget?.ready &&
    readinessTarget?.shellAssets?.verified &&
    artifactStatus.present
  );
  return {
    readyForManualShellSmoke,
    shellAssets: readinessTarget.shellAssets,
    nativeToolchainReady: readinessTarget.ready,
    missingToolchain: readinessTarget.missing,
    artifact: artifactStatus,
    commands: nativeCommands(target, readinessTarget),
    checklist: nativeChecklist(target, readinessTarget, artifactStatus),
    notes: nativeNotes(target, readinessTarget, artifactStatus)
  };
}

function artifactStatusFor(target, artifactPlan) {
  const sources = artifactPlan.sources
    .filter(source => source.target === target)
    .map(source => ({
      sourceHint: source.sourceHint,
      outputFilename: source.outputFilename
    }));
  const missing = artifactPlan.missing
    .filter(entry => entry.target === target)
    .flatMap(entry => entry.expected);
  return {
    present: sources.length > 0,
    sources,
    missing
  };
}

function nativeCommands(target, readinessTarget) {
  if (target === "windows-desktop") {
    return {
      prepareAssets: "npm --prefix clients run build:shell-assets:desktop",
      verifyAssets: "node clients/tools/verify-shell-assets.mjs --target windows-desktop",
      buildArtifact: readinessTarget.buildCommand,
      dryRunBuild: readinessTarget.dryRunCommand
    };
  }
  return {
    prepareAssets: "npm --prefix clients run build:shell-assets:android",
    verifyAssets: "node clients/tools/verify-shell-assets.mjs --target android",
    buildArtifact: readinessTarget.buildCommand,
    dryRunBuild: readinessTarget.dryRunCommand,
    dockerBuilder: readinessTarget.dockerBuilder?.imagePresent
      ? readinessTarget.dockerBuilder.buildCommand
      : readinessTarget.dockerBuilder?.imageBuildCommand
  };
}

function nativeChecklist(target, readinessTarget, artifactStatus) {
  const label = target === "windows-desktop" ? "desktop" : "android";
  const checklist = [
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

  if (!readinessTarget.ready) {
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

  if (!artifactStatus.present) {
    checklist.push({
      step: "collect-native-artifact",
      evidence: "artifact collector finds at least one native package for this target"
    });
  }

  checklist.push({
    step: "run-platform-shell",
    evidence: `shell metadata reports target=${target} and the app can sign in, pull inbox, receive wakeup and ack`
  });
  return checklist;
}

function nativeNotes(target, readinessTarget, artifactStatus) {
  const notes = [];
  if (!readinessTarget.shellAssets?.verified) {
    notes.push("Prepare and verify shell assets before native smoke.");
  }
  if (!readinessTarget.ready) {
    notes.push("Native toolchain is not ready; run the dry-run command or install/build the reported toolchain first.");
  }
  if (!artifactStatus.present) {
    notes.push("No native artifact is present yet; build and collect one before manual shell smoke.");
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
