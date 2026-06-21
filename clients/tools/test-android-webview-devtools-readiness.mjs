import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { buildAndroidWebViewDevtoolsReadinessReport } from "./report-android-webview-devtools-readiness.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const reportScript = join(toolsDir, "report-android-webview-devtools-readiness.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const procNetUnix = [
  "Num       RefCount Protocol Flags    Type St Inode Path",
  "00000000: 00000002 00000000 00010000 0001 01 12345 /dev/socket/logdw",
  "00000000: 00000002 00000000 00010000 0001 01 23456 @webview_devtools_remote_31841"
].join("\n");

const ready = buildAndroidWebViewDevtoolsReadinessReport({
  procNetUnixOutput: procNetUnix
});
const readyText = JSON.stringify(ready);
assert(ready.schemaVersion === "nexusim.android-webview-devtools-readiness.v1", "schema mismatch");
assert(ready.inputSource === "fixture", "input source mismatch");
assert(ready.adbAvailable === true, "fixture should count as available input");
assert(ready.procNetUnixReadable === true, "fixture should be readable");
assert(ready.readyForWebViewAutomation === true, "socket should make WebView automation ready");
assert(ready.counts.webViewDevtoolsSockets === 1, "socket count mismatch");
assert(ready.sockets[0].socketHash.length === 16, "socket hash should be short");
assert(ready.nextActions.some(action => action.action === "run-android-webview-smoke"), "ready next action missing");
assert(!readyText.includes("webview_devtools_remote_31841"), "report leaked raw socket");
assert(!readyText.match(/[A-Z]:\\\\/), "report leaked Windows absolute path");
assert(!readyText.match(/token|secret|password|credential|private/i), "report leaked sensitive field name");

const missingSocket = buildAndroidWebViewDevtoolsReadinessReport({
  procNetUnixOutput: "Num RefCount Protocol Flags Type St Inode Path\n@chrome_devtools_remote\n"
});
assert(missingSocket.readyForWebViewAutomation === false, "missing WebView socket should not be automation-ready");
assert(missingSocket.counts.webViewDevtoolsSockets === 0, "missing socket count mismatch");
assert(missingSocket.nextActions.some(action => action.action === "launch-debuggable-android-shell"), "launch shell next action missing");

const tempRoot = mkdtempSync(join(tmpdir(), "android-webview-devtools-readiness-test-"));
try {
  const inputPath = join(tempRoot, "proc-net-unix.txt");
  writeFileSync(inputPath, `${procNetUnix}\n`, "utf8");
  const output = execFileSync(process.execPath, [
    reportScript,
    "--input",
    inputPath
  ], {
    encoding: "utf8"
  });
  const cli = JSON.parse(output);
  const cliText = JSON.stringify(cli);
  assert(cli.readyForWebViewAutomation === true, "CLI report should be automation-ready");
  assert(!cliText.includes("webview_devtools_remote_31841"), "CLI leaked raw socket");
  assert(!cliText.includes(inputPath), "CLI leaked input path");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("Android WebView devtools readiness report ok");
