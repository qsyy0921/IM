import { cpSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { parseShellConfig, renderShellConfigScript } from "./render-shell-config.mjs";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));

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
  return {
    target,
    configPath,
    outputDir,
    shellConfigPath
  };
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
