import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const workspaceRoot = fileURLToPath(new URL("..", import.meta.url));
const repoRoot = resolve(workspaceRoot, "..");
const script = readFileSync(resolve(repoRoot, "loadtest/clientweb/run-local-smoke.ps1"), "utf8");
const devScript = readFileSync(resolve(repoRoot, "loadtest/clientweb/run-local-dev.ps1"), "utf8");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

assert(script.includes("[switch]$RunDesktopWebViewLoginSmoke"), "desktop WebView login flag missing");
assert(script.includes("[switch]$RunBrowserMultiuserUISmoke"), "browser multi-user UI flag missing");
assert(script.includes("[switch]$RunAndroidWebViewLoginSmoke"), "Android WebView login flag missing");
assert(script.includes("[switch]$AndroidWebViewSkipWebBuild"), "Android skip web build flag missing");
assert(script.includes("[int]$BffPort = 0"), "fixed BFF port parameter missing");
assert(script.includes("[int]$PushPort = 0"), "fixed push port parameter missing");
assert(script.includes("[string]$ClientTenantId = \"\""), "client tenant override parameter missing");
assert(script.includes("[string]$ClientReceiverUserId = \"\""), "client receiver override parameter missing");
assert(script.includes("[switch]$KeepAlive"), "keep-alive flag missing");
assert(script.includes("Assert-TcpPortAvailable -HostName $BindHost -Port $PushPort"), "push port availability check missing");
assert(script.includes("Assert-TcpPortAvailable -HostName $BindHost -Port $BffPort"), "BFF port availability check missing");
assert(script.includes("if ($KeepAlive)"), "keep-alive process branch missing");
assert(script.includes("smoke:desktop-webview-login"), "desktop WebView login npm hook missing");
assert(script.includes("smoke:browser-multiuser-ui"), "browser multi-user UI npm hook missing");
assert(script.includes("smoke:android-webview-login"), "Android WebView login npm hook missing");
assert(script.includes("browser-multiuser-ui-smoke-summary.json"), "browser multi-user UI summary path missing");
assert(script.includes("desktop-webview-login-summary.json"), "desktop summary path missing");
assert(script.includes("android-webview-login-summary.json"), "Android summary path missing");
assert(script.includes("Remove-Item -LiteralPath $browserFixturePath"), "browser multi-user UI fixture cleanup missing");
assert(script.includes("Remove-Item -LiteralPath $androidFixturePath"), "Android fixture cleanup missing");
assert(devScript.includes("BffPort = 8080"), "local dev wrapper must expose fixed BFF port 8080");
assert(devScript.includes("PushPort = 8088"), "local dev wrapper must expose fixed push port 8088");
assert(devScript.includes("ClientTenantId = \"tenant-client-local\""), "local dev wrapper tenant mismatch");
assert(devScript.includes("ClientReceiverUserId = \"user-a\""), "local dev wrapper receiver mismatch");
assert(devScript.includes("KeepAlive = $true"), "local dev wrapper must keep services alive");

console.log("clientweb run-local-smoke WebView hooks ok");
