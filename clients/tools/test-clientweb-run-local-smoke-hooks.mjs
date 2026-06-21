import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const workspaceRoot = fileURLToPath(new URL("..", import.meta.url));
const repoRoot = resolve(workspaceRoot, "..");
const script = readFileSync(resolve(repoRoot, "loadtest/clientweb/run-local-smoke.ps1"), "utf8");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

assert(script.includes("[switch]$RunDesktopWebViewLoginSmoke"), "desktop WebView login flag missing");
assert(script.includes("[switch]$RunAndroidWebViewLoginSmoke"), "Android WebView login flag missing");
assert(script.includes("[switch]$AndroidWebViewSkipWebBuild"), "Android skip web build flag missing");
assert(script.includes("smoke:desktop-webview-login"), "desktop WebView login npm hook missing");
assert(script.includes("smoke:android-webview-login"), "Android WebView login npm hook missing");
assert(script.includes("desktop-webview-login-summary.json"), "desktop summary path missing");
assert(script.includes("android-webview-login-summary.json"), "Android summary path missing");
assert(script.includes("Remove-Item -LiteralPath $androidFixturePath"), "Android fixture cleanup missing");

console.log("clientweb run-local-smoke WebView hooks ok");
