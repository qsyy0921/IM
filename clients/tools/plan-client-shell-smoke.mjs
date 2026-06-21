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
