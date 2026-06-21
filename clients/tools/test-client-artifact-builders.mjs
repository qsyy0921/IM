import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const toolsDir = dirname(fileURLToPath(import.meta.url));

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function dryRun(scriptName) {
  const output = execFileSync(process.execPath, [join(toolsDir, scriptName), "--dry-run"], {
    encoding: "utf8"
  });
  return JSON.parse(output);
}

const desktop = dryRun("build-desktop-artifact.mjs");
assert(desktop.target === "windows-desktop", "desktop target mismatch");
assert(desktop.outputHint.includes("clients/desktop"), "desktop output hint mismatch");
assert(Array.isArray(desktop.args) && desktop.args.includes("build"), "desktop build command missing build arg");
assert(!JSON.stringify(desktop).match(/token|secret|password|credential|private/i), "desktop build plan leaks sensitive names");

const android = dryRun("build-android-apk.mjs");
assert(android.target === "android", "android target mismatch");
assert(android.outputHint.endsWith("app-debug.apk"), "android output hint mismatch");
assert(android.args.join(" ").includes("assembleDebug"), "android build command missing assembleDebug");
assert(!JSON.stringify(android).match(/token|secret|password|credential|private/i), "android build plan leaks sensitive names");

console.log("client artifact builders ok");
