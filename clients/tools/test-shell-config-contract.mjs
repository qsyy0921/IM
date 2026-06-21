import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { parseShellConfig, renderShellConfigScript } from "./render-shell-config.mjs";

const root = fileURLToPath(new URL("..", import.meta.url));

const desktop = parseShellConfig(read("desktop/shell-config.example.json"));
assertEqual(desktop.target, "windows-desktop", "desktop config target");
assertNoSensitiveKeys(desktop, "desktop config");
assertIncludes(renderShellConfigScript(desktop), "windows-desktop", "desktop script includes target");

const desktopSmoke = parseShellConfig(JSON.stringify({
  ...desktop,
  smokeCallbackURL: "http://127.0.0.1:49152/shell-smoke",
  smokeRunID: "desktop-webview-metadata-test",
  smokeMode: "metadata"
}));
assertEqual(desktopSmoke.smokeMode, "metadata", "desktop smoke mode");
assertIncludes(renderShellConfigScript(desktopSmoke), "smokeCallbackURL", "desktop smoke script includes callback");

const android = parseShellConfig(read("android/shell-config.example.json"));
assertEqual(android.target, "android", "android config target");
assertNoSensitiveKeys(android, "android config");
assertIncludes(renderShellConfigScript(android), "android", "android script includes target");

assertThrows(
  () => parseShellConfig(JSON.stringify({ ...desktop, accessToken: "token" })),
  "unsupported shell config key",
  "unknown sensitive key is rejected"
);
assertThrows(
  () => parseShellConfig(JSON.stringify({ ...desktop, target: "ios" })),
  "shell config target",
  "unsupported target is rejected"
);
assertThrows(
  () => parseShellConfig(JSON.stringify({ ...desktop, smokeCallbackURL: "https://example.com/smoke" })),
  "smokeCallbackURL",
  "non-loopback smoke callback is rejected"
);
assertThrows(
  () => parseShellConfig(JSON.stringify({ ...desktop, smokeMode: "login" })),
  "smokeMode",
  "unsupported smoke mode is rejected"
);

const index = read("web/index.html");
assertIncludes(index, "/nexusim-shell-config.js", "web index loads shell config before app bundle");
assert(index.indexOf("/nexusim-shell-config.js") < index.indexOf("</head>"), "web shell config must load in head before Vite moves the app bundle");
const placeholder = read("web/public/nexusim-shell-config.js");
assertIncludes(placeholder, "__NEXUSIM_CLIENT_SHELL__", "web placeholder declares shell global");

console.log("shell config contract ok");

function read(path) {
  return readFileSync(join(root, path), "utf8");
}

function assertNoSensitiveKeys(config, message) {
  for (const key of Object.keys(config)) {
    if (/token|secret|password|credential|private/i.test(key)) {
      throw new Error(`${message}: sensitive key ${key}`);
    }
  }
}

function assertIncludes(value, expected, message) {
  if (!value.includes(expected)) {
    throw new Error(`${message}: expected ${expected}`);
  }
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: expected ${expected}, got ${actual}`);
  }
}

function assertThrows(fn, expected, message) {
  try {
    fn();
  } catch (error) {
    const actual = error instanceof Error ? error.message : String(error);
    if (!actual.includes(expected)) {
      throw new Error(`${message}: expected ${expected}, got ${actual}`);
    }
    return;
  }
  throw new Error(`${message}: expected error`);
}
