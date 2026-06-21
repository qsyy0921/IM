import { join } from "node:path";
import { workspaceRoot } from "./client-build-env.mjs";

const collectScript = join(workspaceRoot, "tools", "collect-client-artifacts.mjs");

export function parseArtifactBuildOptions(argv) {
  const options = {
    dryRun: false,
    skipWebBuild: false,
    collect: false,
    artifactOutputDir: "",
    runId: ""
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--dry-run") {
      options.dryRun = true;
      continue;
    }
    if (arg === "--skip-web-build") {
      options.skipWebBuild = true;
      continue;
    }
    if (arg === "--collect") {
      options.collect = true;
      continue;
    }
    if (arg === "--artifact-output-dir") {
      options.artifactOutputDir = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--run-id") {
      options.runId = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return options;
}

export function collectArgs(target, options) {
  const args = [collectScript, "--target", target];
  if (options.artifactOutputDir) {
    args.push("--output-dir", options.artifactOutputDir);
  }
  if (options.runId) {
    args.push("--run-id", options.runId);
  }
  return args;
}

export function collectPlanSummary(target, options) {
  return {
    enabled: options.collect,
    target,
    outputDir: options.artifactOutputDir ? "custom" : "clients/artifacts",
    runId: options.runId || "auto"
  };
}

function requiredValue(argv, index, name) {
  const value = argv[index + 1];
  if (!value || value.startsWith("--")) {
    throw new Error(`${name} requires a value`);
  }
  return value;
}
