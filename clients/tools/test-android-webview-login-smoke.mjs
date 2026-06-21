import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parseAndroidNativeStoreReadinessText } from "./smoke-android-webview-login.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const smokeScript = join(toolsDir, "smoke-android-webview-login.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const tempRoot = mkdtempSync(join(tmpdir(), "android-webview-login-test-"));
try {
  const fixturePath = join(tempRoot, "fixture.json");
  writeFileSync(fixturePath, `${JSON.stringify({
    apiBaseURL: "http://127.0.0.1:18080",
    pushWebSocketURL: "ws://127.0.0.1:18088/ws",
    tenantID: "tenant-android-login-test",
    userID: "receiver-android-login-test",
    authProof: "ReceiverPassw0rd!",
    deviceID: "android-webview-login-device",
    conversationID: "conv-android-login-test",
    senderUserID: "sender-android-login-test",
    senderAuthProof: "SenderPassw0rd!",
    senderDeviceID: "android-webview-login-sender",
    messageText: "NexusIM Android WebView login smoke test message"
  }, null, 2)}\n`, "utf8");

  const output = execFileSync(process.execPath, [
    smokeScript,
    "--dry-run",
    "--fixture",
    fixturePath,
    "--run-id",
    "android-webview-login-test"
  ], {
    encoding: "utf8"
  });
  const result = JSON.parse(output);
  const serialized = JSON.stringify(result);

  assert(result.schemaVersion === "nexusim.android-webview-login-smoke.v1", "schema mismatch");
  assert(result.dryRun === true, "dry-run flag missing");
  assert(result.runID === "android-webview-login-test", "run id mismatch");
  assert(result.input.externalMessageTrigger === true, "external sender trigger marker missing");
  assert(result.build.freshBuildRequired === true, "fresh build marker missing");
  assert(result.adb.webviewDevtoolsForwardRequired === true, "WebView devtools forward marker missing");
  assert(result.automation.driver === "android-webview-cdp-via-adb-forward", "driver mismatch");
  assert(result.automation.requiredSelectors.includes("native-store-readiness"), "native store readiness selector missing");
  assert(result.automation.requiredSelectors.includes("ack-status"), "ack selector missing");
  assert(result.verdict.loginLevelAndroidUISmoke === false, "dry-run must not claim login smoke");
  assert(result.verdict.deliveryNotifyInWebView === false, "dry-run must not claim notify smoke");
  assert(result.verdict.pullInboxInWebView === false, "dry-run must not claim PullInbox smoke");
  assert(result.verdict.ackDeliveryInWebView === false, "dry-run must not claim AckDelivery smoke");
  assert(!serialized.match(/[A-Z]:\\\\/), "dry-run leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "dry-run leaked extended Windows path");
  assert(!serialized.match(/token|secret|password|credential|private/i), "dry-run leaked sensitive field name");

  const readyNativeStore = parseAndroidNativeStoreReadinessText("local-storage -> sqlite; ready; android-sqlite");
  assert(readyNativeStore.ok === true, "Android login smoke must accept ready native SQLite store evidence");
  assert(readyNativeStore.nativeStoreReady === true, "Android native store must be reported ready");
  assert(readyNativeStore.nativeStoreReason === "", "Android ready native store must not carry a failure reason");

  const unavailableNativeStore = parseAndroidNativeStoreReadinessText(
    "local-storage -> sqlite; sqlite-native-bridge-unavailable; android-sqlite"
  );
  assert(unavailableNativeStore.ok === false, "Android login smoke must not accept stale unavailable native store evidence");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("Android WebView login smoke dry-run ok");
