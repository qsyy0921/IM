import { fileURLToPath } from "node:url";
import { isAbsolute, relative, resolve, sep } from "node:path";
import { prepareShellWebAssets } from "./prepare-shell-web-assets.mjs";
import { verifyShellAssets } from "./verify-shell-assets.mjs";

const skipEnvName = "NEXUSIM_SKIP_SHELL_ASSET_PREP";
const clientsRoot = fileURLToPath(new URL("..", import.meta.url));

export function prepareShellWebAssetsIfNeeded(options) {
  if (!options.target) {
    throw new Error("--target is required");
  }

  if (options.skip === true) {
    const verified = verifyShellAssets({
      target: options.target,
      outputDir: options.outputDir
    });
    return {
      target: options.target,
      skipped: true,
      verified: true,
      fileCount: verified.fileCount
    };
  }

  const prepared = prepareShellWebAssets({
    target: options.target,
    configPath: options.configPath,
    outputDir: options.outputDir,
    sourceDir: options.sourceDir,
    build: options.build
  });
  return {
    target: options.target,
    skipped: false,
    outputDirHint: safeHint(prepared.outputDir),
    manifestHint: safeHint(prepared.manifestPath)
  };
}

function safeHint(path) {
  const relativePath = relative(clientsRoot, resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return "custom";
  }
  return relativePath;
}

function main(argv) {
  const target = valueAfter(argv, "--target");
  const result = prepareShellWebAssetsIfNeeded({
    target,
    skip: process.env[skipEnvName]?.toLowerCase() === "true"
  });
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
}

function valueAfter(argv, name) {
  const index = argv.indexOf(name);
  if (index === -1) {
    return undefined;
  }
  return argv[index + 1];
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
