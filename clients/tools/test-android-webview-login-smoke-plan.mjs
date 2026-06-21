import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const planScript = join(toolsDir, "plan-android-webview-login-smoke.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const output = execFileSync(process.execPath, [
  planScript,
  "--run-id",
  "android-webview-login-plan-test"
], {
  encoding: "utf8"
});
const result = JSON.parse(output);
const serialized = JSON.stringify(result);

assert(result.schemaVersion === "nexusim.android-webview-login-smoke-plan.v1", "schema mismatch");
assert(result.runID === "android-webview-login-plan-test", "run id mismatch");
assert(result.target === "android", "target mismatch");
assert(result.verdict.planOnly === true, "plan-only marker missing");
assert(result.verdict.loginLevelAndroidUISmoke === false, "plan must not claim login smoke");
assert(result.verdict.deliveryNotifyInWebView === false, "plan must not claim notify smoke");
assert(result.verdict.pullInboxInWebView === false, "plan must not claim PullInbox smoke");
assert(result.verdict.ackDeliveryInWebView === false, "plan must not claim AckDelivery smoke");
assert(result.prerequisites.some(item => item.name === "debuggable-apk"), "debuggable APK prerequisite missing");
assert(result.prerequisites.some(item => item.name === "webview-devtools-socket"), "WebView devtools prerequisite missing");
assert(result.commands.devtoolsDiscovery.includes("report:android-webview-devtools-readiness"), "WebView devtools readiness command missing");
assert(result.commands.runner.includes("smoke:android-webview-login"), "runner command missing");
assert(result.selectorContract.required.includes("native-store-readiness"), "native store readiness selector missing");
assert(result.selectorContract.required.includes("ack-status"), "ack selector contract missing");
assert(!serialized.match(/[A-Z]:\\\\/), "plan leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "plan leaked extended Windows path");
assert(!serialized.match(/token|secret|password|credential|private/i), "plan leaked sensitive field name");

console.log("Android WebView login smoke plan ok");
