import { readFileSync, writeFileSync } from "node:fs";

const allowedTargets = new Set(["browser", "windows-desktop", "android"]);
const allowedKeys = new Set([
  "target",
  "apiBaseURL",
  "pushWebSocketURL",
  "deviceID",
  "installationID",
  "appVersion",
  "sessionKey"
]);
const forbiddenKeyPattern = /(token|secret|password|credential|private)/i;

export function parseShellConfig(input) {
  const parsed = JSON.parse(input);
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error("shell config must be a JSON object");
  }

  const output = {};
  for (const [key, value] of Object.entries(parsed)) {
    if (!allowedKeys.has(key)) {
      throw new Error(`unsupported shell config key: ${key}`);
    }
    if (forbiddenKeyPattern.test(key)) {
      throw new Error(`sensitive shell config key is not allowed: ${key}`);
    }
    if (typeof value !== "string" || value.trim() === "") {
      throw new Error(`shell config ${key} must be a non-empty string`);
    }
    output[key] = value;
  }

  if (!allowedTargets.has(output.target)) {
    throw new Error("shell config target must be browser, windows-desktop, or android");
  }
  if (!output.apiBaseURL?.startsWith("http://") && !output.apiBaseURL?.startsWith("https://")) {
    throw new Error("shell config apiBaseURL must be http(s)");
  }
  if (!output.pushWebSocketURL?.startsWith("ws://") && !output.pushWebSocketURL?.startsWith("wss://")) {
    throw new Error("shell config pushWebSocketURL must be ws(s)");
  }
  return output;
}

export function renderShellConfigScript(config) {
  return [
    "globalThis.__NEXUSIM_CLIENT_SHELL__ = Object.freeze(",
    JSON.stringify(config, null, 2),
    ");",
    ""
  ].join("\n");
}

function main(argv) {
  const inputPath = valueAfter(argv, "--input");
  const outputPath = valueAfter(argv, "--output");
  if (!inputPath) {
    throw new Error("usage: node render-shell-config.mjs --input <config.json> [--output <file>]");
  }
  const config = parseShellConfig(readFileSync(inputPath, "utf8"));
  const script = renderShellConfigScript(config);
  if (outputPath) {
    writeFileSync(outputPath, script, "utf8");
    return;
  }
  process.stdout.write(script);
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
