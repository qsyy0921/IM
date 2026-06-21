import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const smokeScript = join(toolsDir, "smoke-desktop-webview-metadata.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const output = execFileSync(process.execPath, [
  smokeScript,
  "--dry-run",
  "--run-id",
  "desktop-webview-metadata-test"
], {
  encoding: "utf8"
});
const result = JSON.parse(output);
const serialized = JSON.stringify(result);

assert(result.schemaVersion === "nexusim.desktop-webview-metadata-smoke.v1", "schema mismatch");
assert(result.dryRun === true, "dry-run flag missing");
assert(result.runID === "desktop-webview-metadata-test", "run id mismatch");
assert(result.build.shellConfig === "temporary-loopback-metadata", "temporary shell config marker missing");
assert(result.callback.loopbackOnly === true, "loopback callback marker missing");
assert(result.verdict.metadataWebViewSmoke === false, "dry-run must not claim metadata smoke");
assert(result.verdict.loginLevelDesktopUISmoke === false, "metadata smoke must not claim login smoke");
assert(!serialized.match(/[A-Z]:\\\\/), "dry-run leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "dry-run leaked extended Windows path");
assert(!serialized.match(/token|secret|password|credential|private/i), "dry-run leaked sensitive field name");

console.log("desktop WebView metadata smoke dry-run ok");
