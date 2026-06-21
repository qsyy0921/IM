import { existsSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  collectClientBuildPrereqs,
  runCommand,
  workspaceRoot
} from "./client-build-env.mjs";

const androidBuilderImage = "nexusim/client-android-builder:local";
const androidBuilderCompose = "deploy/local/docker-compose.client-builders.yml";
const androidBuilderDockerfile = "deploy/docker/client-android-builder.Dockerfile";

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
        buildCommand: "npm --prefix clients run build:desktop-artifact:collect",
        dryRunCommand: "node clients/tools/build-desktop-artifact.mjs --dry-run --collect"
      },
      android: {
        ready: prereqs.androidApkReady,
        missing: missingChecks(prereqs.checks, "android"),
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
          buildCommand:
            "docker compose -f deploy/local/docker-compose.client-builders.yml --profile client-builders run --rm client-android-apk-builder"
        }
      }
    },
    checks: sanitizedChecks(prereqs.checks),
    docker
  };
}

function missingChecks(checks, target) {
  return checks
    .filter(check => check.target === target && !check.ok)
    .map(check => ({
      name: check.name,
      label: check.label
    }));
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
