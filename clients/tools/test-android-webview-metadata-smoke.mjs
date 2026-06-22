import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const smokeScript = join(toolsDir, "smoke-android-webview-metadata.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const output = execFileSync(process.execPath, [
  smokeScript,
  "--dry-run",
  "--run-id",
  "android-webview-metadata-test"
], {
  encoding: "utf8"
});
const result = JSON.parse(output);
const serialized = JSON.stringify(result);

assert(result.schemaVersion === "nexusim.android-webview-metadata-smoke.v1", "schema mismatch");
assert(result.dryRun === true, "dry-run flag missing");
assertDryRunExecutionPolicy(result.executionPolicy);
assert(result.runID === "android-webview-metadata-test", "run id mismatch");
assert(result.build.shellConfig === "temporary-loopback-metadata", "temporary shell config marker missing");
assert(result.build.freshBuildRequired === true, "Android metadata smoke must require a fresh callback-config build");
assert(result.build.freshBuildReason.includes("callback URL"), "fresh-build reason should mention callback URL injection");
assert(result.adb.packageName === "com.nexusim.android", "Android package marker missing");
assert(result.adb.mainActivity === "com.nexusim.android/.MainActivity", "Android activity marker missing");
assert(result.adb.reverseLoopback === true, "adb reverse loopback marker missing");
assert(result.callback.loopbackOnly === true, "loopback callback marker missing");
assert(result.verdict.metadataWebViewSmoke === false, "dry-run must not claim metadata smoke");
assert(result.verdict.loginLevelAndroidUISmoke === false, "metadata smoke must not claim login smoke");
assert(!serialized.match(/[A-Z]:\\\\/), "dry-run leaked Windows absolute path");
assert(!serialized.includes("\\\\?"), "dry-run leaked extended Windows path");
assert(!serialized.match(/token|secret|password|credential|private/i), "dry-run leaked sensitive field name");

console.log("Android WebView metadata smoke dry-run ok");

function assertDryRunExecutionPolicy(policy) {
  assert(policy?.planOnly === true, "Android metadata dry-run should be marked plan-only");
  assert(policy.executesPlannedCommands === false, "Android metadata dry-run should not execute planned commands");
  assert(policy.buildsAPK === false, "Android metadata dry-run should not build APKs");
  assert(policy.installsAPK === false, "Android metadata dry-run should not install APKs");
  assert(policy.startsActivity === false, "Android metadata dry-run should not start activities");
  assert(policy.opensAdbReverse === false, "Android metadata dry-run should not open adb reverse");
  assert(policy.contactsDevice === false, "Android metadata dry-run should not contact devices");
  assert(policy.startsCallbackServer === false, "Android metadata dry-run should not start callback server");
  assert(policy.opensNetworkConnection === false, "Android metadata dry-run should not open network connections");
  assert(policy.downloadsToolchain === false, "Android metadata dry-run should not download toolchains");
}
