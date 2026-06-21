import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { dirname, join, relative, resolve, sep } from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { parseShellConfig, renderShellConfigScript } from "./render-shell-config.mjs";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));
const shellAssetsManifestFilename = "nexusim-shell-assets-manifest.json";

const targetSpecs = {
  "windows-desktop": {
    configPath: "desktop/shell-config.example.json",
    outputDir: "web/dist",
    expectedTarget: "windows-desktop"
  },
  android: {
    configPath: "android/shell-config.example.json",
    outputDir: "android/native/app/src/main/assets/nexusim",
    expectedTarget: "android"
  }
};

export function prepareShellWebAssets(options) {
  const target = options.target;
  const spec = targetSpecs[target];
  if (!spec) {
    throw new Error("target must be windows-desktop or android");
  }

  const sourceDir = resolve(clientsRoot, options.sourceDir ?? "web/dist");
  const outputDir = resolve(clientsRoot, options.outputDir ?? spec.outputDir);
  const configPath = resolve(clientsRoot, options.configPath ?? spec.configPath);

  if (options.build !== false) {
    runWebBuild();
  }
  if (!existsSync(sourceDir)) {
    throw new Error(`web dist source does not exist: ${sourceDir}`);
  }

  if (sourceDir !== outputDir) {
    rmSync(outputDir, { recursive: true, force: true });
    mkdirSync(outputDir, { recursive: true });
    cpSync(sourceDir, outputDir, { recursive: true, force: true });
  }

  const config = parseShellConfig(readFileSync(configPath, "utf8"));
  if (config.target !== spec.expectedTarget) {
    throw new Error(`shell config target ${config.target} does not match ${spec.expectedTarget}`);
  }
  const shellConfigPath = resolve(outputDir, "nexusim-shell-config.js");
  mkdirSync(dirname(shellConfigPath), { recursive: true });
  writeFileSync(shellConfigPath, renderShellConfigScript(config), "utf8");
  const manifestPath = resolve(outputDir, shellAssetsManifestFilename);
  writeFileSync(manifestPath, JSON.stringify(buildShellAssetsManifest(target, outputDir), null, 2) + "\n", "utf8");
  return {
    target,
    configPath,
    outputDir,
    shellConfigPath,
    manifestPath
  };
}

function buildShellAssetsManifest(target, outputDir) {
  return {
    schemaVersion: "nexusim.shell-assets.v1",
    target,
    generatedAt: new Date().toISOString(),
    files: listAssetFiles(outputDir).map(path => {
      const bytes = readFileSync(join(outputDir, path));
      return {
        path,
        bytes: bytes.length,
        sha256: createHash("sha256").update(bytes).digest("hex")
      };
    })
  };
}

function listAssetFiles(rootDir, dir = rootDir) {
  const files = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...listAssetFiles(rootDir, fullPath));
      continue;
    }
    if (!entry.isFile() || entry.name === shellAssetsManifestFilename) {
      continue;
    }
    const relativePath = relative(rootDir, fullPath).split(sep).join("/");
    files.push(relativePath);
  }
  return files.sort();
}

function runWebBuild() {
  const npmExecPath = process.env.npm_execpath;
  if (npmExecPath) {
    execFileSync(process.execPath, [npmExecPath, "--prefix", clientsRoot, "--workspace", "@nexusim/web", "run", "build"], {
      cwd: clientsRoot,
      stdio: "inherit"
    });
    return;
  }

  const npm = process.platform === "win32" ? "npm.cmd" : "npm";
  execFileSync(npm, ["--prefix", clientsRoot, "--workspace", "@nexusim/web", "run", "build"], {
    cwd: clientsRoot,
    stdio: "inherit",
    shell: process.platform === "win32"
  });
}

function main(argv) {
  const target = valueAfter(argv, "--target");
  const configPath = valueAfter(argv, "--config");
  const outputDir = valueAfter(argv, "--output-dir");
  const sourceDir = valueAfter(argv, "--source-dir");
  const skipBuild = argv.includes("--skip-build");
  const result = prepareShellWebAssets({
    target,
    configPath,
    outputDir,
    sourceDir,
    build: !skipBuild
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

if (process.argv[1] && import.meta.url.endsWith(process.argv[1].replaceAll("\\", "/"))) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
