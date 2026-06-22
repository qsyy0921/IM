import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { collectClientBuildPrereqs } from "./client-build-env.mjs";

function main() {
  const result = buildClientBuildPrereqsReport();
  console.log(JSON.stringify(result, null, 2));

  if (!result.desktopArtifactReady || !result.androidApkReady) {
    process.exitCode = 2;
  }
}

export function buildClientBuildPrereqsReport(prereqs = collectClientBuildPrereqs()) {
  return {
    schemaVersion: "nexusim.client-build-prereqs.v1",
    generatedAt: new Date().toISOString(),
    executionPolicy: readinessExecutionPolicy(),
    desktopArtifactReady: Boolean(prereqs.desktopArtifactReady),
    androidApkReady: Boolean(prereqs.androidApkReady),
    checks: sanitizeChecks(prereqs.checks ?? [])
  };
}

function readinessExecutionPolicy() {
  return {
    reportOnly: true,
    planOnly: false,
    runsReadinessCommands: true,
    readsLocalToolchainState: true,
    readsEnvironmentVariables: true,
    readsLocalNodeBinState: true,
    buildsNativeArtifacts: false,
    preparesShellAssets: false,
    startsServices: false,
    startsDocker: false,
    buildsDockerImages: false,
    installsArtifacts: false,
    contactsDevice: false,
    startsDeviceActivities: false,
    opensAdbReverse: false,
    opensAdbForward: false,
    downloadsToolchain: false,
    exposesLocalAbsolutePaths: false,
    exposesCommandOutput: false
  };
}

function sanitizeChecks(checks) {
  return checks.map(check => {
    const result = {
      name: stringOrEmpty(check.name),
      target: stringOrEmpty(check.target),
      label: stringOrEmpty(check.label),
      ok: Boolean(check.ok)
    };
    if (typeof check.detectedMajorVersion === "number") {
      result.detectedMajorVersion = check.detectedMajorVersion;
    }
    return result;
  });
}

function stringOrEmpty(value) {
  return typeof value === "string" ? value : "";
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  main();
}
