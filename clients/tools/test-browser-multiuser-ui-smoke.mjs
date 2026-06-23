import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const smokeScript = join(toolsDir, "smoke-browser-multiuser-ui.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const tempRoot = mkdtempSync(join(tmpdir(), "browser-multiuser-ui-smoke-test-"));
try {
  const fixturePath = join(tempRoot, "fixture.json");
  writeFileSync(fixturePath, `${JSON.stringify({
    apiBaseURL: "http://127.0.0.1:18080",
    pushWebSocketURL: "ws://127.0.0.1:18088/ws",
    tenantID: "tenant-browser-ui-test",
    senderUserID: "sender-browser-ui-test",
    senderLoginInput: "SenderPassw0rd!",
    receiverUserID: "receiver-browser-ui-test",
    receiverLoginInput: "ReceiverPassw0rd!"
  }, null, 2)}\n`, "utf8");

  const output = execFileSync(process.execPath, [
    smokeScript,
    "--dry-run",
    "--fixture",
    fixturePath,
    "--run-id",
    "browser-multiuser-ui-test"
  ], {
    encoding: "utf8"
  });
  const result = JSON.parse(output);
  const serialized = JSON.stringify(result);

  assert(result.schemaVersion === "nexusim.browser-multiuser-ui-smoke.v1", "schema mismatch");
  assert(result.dryRun === true, "dry-run flag missing");
  assert(result.runID === "browser-multiuser-ui-test", "run id mismatch");
  assert(result.input.loginInputRequired === true, "login input marker missing");
  assert(result.web.url === "http://127.0.0.1:5173", "default web URL mismatch");
  assertDryRunExecutionPolicy(result.executionPolicy);
  assert(result.automation.driver === "chromium-cdp", "driver mismatch");
  assert(result.automation.requiredSelectors.includes("friend-conversation-item"), "friend selector missing");
  assert(result.automation.requiredSelectors.includes("group-invite-submit"), "group invite selector missing");
  assert(result.automation.requiredSelectors.includes("message-composer"), "message composer selector missing");
  assert(result.automation.requiredSelectors.includes("conversation-tags-input"), "conversation tags selector missing");
  assert(result.automation.requiredSelectors.includes("conversation-draft-input"), "conversation draft selector missing");
  assert(result.automation.requiredSelectors.includes("active-conversation-archive-toggle"), "conversation archive selector missing");
  assert(result.automation.requiredSelectors.includes("conversation-archived-only"), "archived filter selector missing");
  assert(result.verdict.browserMultiUserUISmoke === false, "dry-run must not claim real smoke");
  assert(result.verdict.directChatThroughUI === false, "dry-run must not claim direct UI smoke");
  assert(result.verdict.groupChatThroughUI === false, "dry-run must not claim group UI smoke");
  assert(result.verdict.conversationManagementThroughUI === false, "dry-run must not claim conversation management smoke");
  assert(!serialized.includes("SenderPassw0rd") && !serialized.includes("ReceiverPassw0rd"), "dry-run leaked runtime login input");
  assert(!serialized.match(/[A-Z]:\\\\/), "dry-run leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "dry-run leaked extended Windows path");
  assert(!serialized.match(/token|secret|credential|private/i), "dry-run leaked sensitive field name");
  assert(!serialized.match(/fallback/i), "dry-run must not describe fallback behavior");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("browser multi-user UI smoke dry-run ok");

function assertDryRunExecutionPolicy(policy) {
  assert(policy?.planOnly === true, "dry-run should be plan-only");
  assert(policy.executesPlannedCommands === false, "dry-run should not execute planned commands");
  assert(policy.launchesBrowser === false, "dry-run should not launch browser");
  assert(policy.startsWebDevServer === false, "dry-run should not start Web dev server");
  assert(policy.usesBrowserAutomation === false, "dry-run should not automate browser");
  assert(policy.contactsBFF === false, "dry-run should not contact BFF");
  assert(policy.opensPushWebSocket === false, "dry-run should not open push WebSocket");
  assert(policy.sendsMessages === false, "dry-run should not send messages");
  assert(policy.writesProtectedMaterial === false, "dry-run should not write protected material");
  assert(policy.downloadsToolchain === false, "dry-run should not download toolchains");
}
