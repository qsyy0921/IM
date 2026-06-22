import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { runCommand, workspaceRoot } from "./client-build-env.mjs";

const repoRoot = join(workspaceRoot, "..");
const composeFile = "deploy/local/docker-compose.client-builders.yml";
const dockerfile = "deploy/docker/client-android-builder.Dockerfile";
const profile = "client-builders";
const service = "client-android-apk-builder";
const image = "nexusim/client-android-builder:local";

function main(argv) {
  const options = parseArgs(argv);
  const plan = buildAndroidDockerBuilderPlan(options);
  if (options.dryRun) {
    console.log(JSON.stringify(plan, null, 2));
    return;
  }

  if (!plan.dockerAvailable || !plan.composeAvailable || !plan.profileReady) {
    console.log(JSON.stringify(plan, null, 2));
    throw new Error("Android Docker builder profile is not ready");
  }

  if (!plan.imagePresent) {
    if (!options.allowImageBuild) {
      console.log(JSON.stringify(plan, null, 2));
      throw new Error("Android Docker builder image is not present; rerun with --allow-image-build to download and build toolchains");
    }
    runDocker(plan.commands.buildImageArgs);
  }

  if (options.buildImageOnly) {
    return;
  }
  runDocker(plan.commands.runBuilderArgs);
}

export function buildAndroidDockerBuilderPlan(options = {}) {
  const dockerVersion = runCommand("docker", ["version", "--format", "{{.Server.Version}}"]);
  const composeVersion = runCommand("docker", ["compose", "version", "--short"]);
  const imageInspect = runCommand("docker", ["image", "inspect", image]);
  const composeFilePresent = existsSync(join(repoRoot, composeFile));
  const dockerfilePresent = existsSync(join(repoRoot, dockerfile));
  const imagePresent = imageInspect.status === 0;
  const allowImageBuild = Boolean(options.allowImageBuild);
  const buildImageOnly = Boolean(options.buildImageOnly);
  return {
    schemaVersion: "nexusim.android-docker-builder-plan.v1",
    generatedAt: new Date().toISOString(),
    executionPolicy: dockerBuilderExecutionPolicy({
      dryRun: Boolean(options.dryRun),
      imagePresent,
      allowImageBuild,
      buildImageOnly
    }),
    dryRun: Boolean(options.dryRun),
    allowImageBuild,
    buildImageOnly,
    safeDefaultNoImageBuild: !allowImageBuild,
    dockerAvailable: dockerVersion.status === 0,
    composeAvailable: composeVersion.status === 0,
    composeFilePresent,
    dockerfilePresent,
    profileReady: dockerVersion.status === 0 && composeVersion.status === 0 && composeFilePresent && dockerfilePresent,
    image,
    imagePresent,
    downloadsToolchain: !imagePresent && allowImageBuild,
    outputHint: "clients/artifacts/android/docker-android-debug/manifest.json",
    commands: {
      buildImage: `docker compose -f ${composeFile} --profile ${profile} build ${service}`,
      runBuilder: `docker compose -f ${composeFile} --profile ${profile} run --rm ${service}`,
      buildImageArgs: ["compose", "-f", composeFile, "--profile", profile, "build", service],
      runBuilderArgs: ["compose", "-f", composeFile, "--profile", profile, "run", "--rm", service]
    },
    nextAction: nextAction({ imagePresent, allowImageBuild })
  };
}

function dockerBuilderExecutionPolicy({ dryRun, imagePresent, allowImageBuild, buildImageOnly }) {
  const canBuildImage = !imagePresent && allowImageBuild;
  const canRunBuilder = !buildImageOnly && (imagePresent || allowImageBuild);
  return {
    planOnly: dryRun,
    reportOnly: dryRun,
    readsDockerBuilderState: true,
    runsDockerCommands: !dryRun,
    startsDocker: !dryRun && (canBuildImage || canRunBuilder),
    buildsDockerImages: !dryRun && canBuildImage,
    startsBuilderContainer: !dryRun && canRunBuilder,
    buildsAndroidApk: !dryRun && canRunBuilder,
    collectsArtifacts: !dryRun && canRunBuilder,
    writesArtifactManifest: !dryRun && canRunBuilder,
    installsArtifacts: false,
    contactsDevice: false,
    startsDeviceActivities: false,
    opensAdbReverse: false,
    opensAdbForward: false,
    downloadsToolchain: !dryRun && canBuildImage,
    plannedDownloadsToolchain: canBuildImage,
    requiresExplicitUserOptInForDownloads: !imagePresent,
    exposesLocalAbsolutePaths: false
  };
}

function nextAction({ imagePresent, allowImageBuild }) {
  if (!imagePresent && !allowImageBuild) {
    return {
      action: "bootstrap-android-builder-image",
      command: "npm --prefix clients run build:android-apk:docker:bootstrap",
      downloadsToolchain: true
    };
  }
  if (!imagePresent && allowImageBuild) {
    return {
      action: "build-image-then-run-builder",
      command: "npm --prefix clients run build:android-apk:docker:bootstrap",
      downloadsToolchain: true
    };
  }
  return {
    action: "run-existing-android-builder-image",
    command: "npm --prefix clients run build:android-apk:docker",
    downloadsToolchain: false
  };
}

function runDocker(args) {
  const executed = spawnSync("docker", args, {
    cwd: repoRoot,
    stdio: "inherit"
  });
  if (executed.status !== 0) {
    throw new Error(`docker command failed: docker ${args.join(" ")}`);
  }
}

function parseArgs(argv) {
  const options = {
    dryRun: false,
    allowImageBuild: false,
    buildImageOnly: false
  };
  for (const arg of argv) {
    if (arg === "--dry-run") {
      options.dryRun = true;
    } else if (arg === "--allow-image-build") {
      options.allowImageBuild = true;
    } else if (arg === "--build-image-only") {
      options.buildImageOnly = true;
      options.allowImageBuild = true;
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  return options;
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
