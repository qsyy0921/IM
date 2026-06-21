import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const smokeScript = join(toolsDir, "smoke-desktop-webview-login.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const tempRoot = mkdtempSync(join(tmpdir(), "desktop-webview-login-test-"));
try {
  const fixturePath = join(tempRoot, "fixture.json");
  writeFileSync(fixturePath, `${JSON.stringify({
    apiBaseURL: "http://127.0.0.1:18080",
    pushWebSocketURL: "ws://127.0.0.1:18088/ws",
    tenantID: "tenant-desktop-login-test",
    userID: "receiver-desktop-login-test",
    authProof: "ReceiverPassw0rd!",
    deviceID: "desktop-webview-login-device",
    conversationID: "conv-desktop-login-test",
    senderUserID: "sender-desktop-login-test",
    senderAuthProof: "SenderPassw0rd!",
    senderDeviceID: "desktop-webview-login-sender",
    messageText: "NexusIM desktop WebView login smoke test message"
  }, null, 2)}\n`, "utf8");

  const output = execFileSync(process.execPath, [
    smokeScript,
    "--dry-run",
    "--fixture",
    fixturePath,
    "--run-id",
    "desktop-webview-login-test"
  ], {
    encoding: "utf8"
  });
  const result = JSON.parse(output);
  const serialized = JSON.stringify(result);

  assert(result.schemaVersion === "nexusim.desktop-webview-login-smoke.v1", "schema mismatch");
  assert(result.dryRun === true, "dry-run flag missing");
  assert(result.runID === "desktop-webview-login-test", "run id mismatch");
  assert(result.input.externalMessageTrigger === true, "external sender trigger marker missing");
  assert(result.automation.driver === "webview2-cdp", "driver mismatch");
  assert(result.verdict.loginLevelDesktopUISmoke === false, "dry-run must not claim login smoke");
  assert(result.verdict.deliveryNotifyInWebView === false, "dry-run must not claim notify smoke");
  assert(!serialized.match(/[A-Z]:\\\\/), "dry-run leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "dry-run leaked extended Windows path");
  assert(!serialized.match(/token|secret|password|credential|private/i), "dry-run leaked sensitive field name");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop WebView login smoke dry-run ok");
