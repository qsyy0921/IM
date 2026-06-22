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
  assertDryRunExecutionPolicy(result.executionPolicy);
  assert(result.runID === "desktop-webview-login-test", "run id mismatch");
  assert(result.input.externalMessageTrigger === true, "external sender trigger marker missing");
  assert(result.automation.driver === "webview2-cdp", "driver mismatch");
  assert(result.automation.requiredSelectors.includes("native-store-readiness"), "native store readiness selector missing");
  assert(result.automation.requiredSelectors.includes("ack-status"), "ack selector missing");
  assert(result.verdict.loginLevelDesktopUISmoke === false, "dry-run must not claim login smoke");
  assert(result.verdict.deliveryNotifyInWebView === false, "dry-run must not claim notify smoke");
  assert(!serialized.match(/[A-Z]:\\\\/), "dry-run leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "dry-run leaked extended Windows path");
  assert(!serialized.match(/token|secret|password|credential|private/i), "dry-run leaked sensitive field name");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop WebView login smoke dry-run ok");

function assertDryRunExecutionPolicy(policy) {
  assert(policy?.planOnly === true, "desktop login dry-run should be marked plan-only");
  assert(policy.executesPlannedCommands === false, "desktop login dry-run should not execute planned commands");
  assert(policy.buildsArtifact === false, "desktop login dry-run should not build artifacts");
  assert(policy.startsArtifact === false, "desktop login dry-run should not start artifacts");
  assert(policy.opensWebViewDebugPort === false, "desktop login dry-run should not open WebView debug port");
  assert(policy.usesWebViewAutomation === false, "desktop login dry-run should not use WebView automation");
  assert(policy.contactsBFF === false, "desktop login dry-run should not contact BFF");
  assert(policy.sendsMessages === false, "desktop login dry-run should not send messages");
  assert(policy.opensNetworkConnection === false, "desktop login dry-run should not open network connections");
  assert(policy.downloadsToolchain === false, "desktop login dry-run should not download toolchains");
}
