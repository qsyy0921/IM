import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { buildBrowserMultiUserUISmokePlan } from "./plan-browser-multiuser-ui-smoke.mjs";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const planScript = join(toolsDir, "plan-browser-multiuser-ui-smoke.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const sampleSummary = buildSampleSummary();
const plan = buildBrowserMultiUserUISmokePlan({ summary: sampleSummary });
assertPlan(plan);

const tempRoot = mkdtempSync(join(tmpdir(), "browser-ui-smoke-plan-test-"));
try {
  const resultDir = join(tempRoot, "run");
  const summaryPath = join(resultDir, "client-web-summary.json");
  const outputPath = join(resultDir, "browser-ui-smoke-plan.json");
  mkdirSync(resultDir, { recursive: true });
  writeFileSync(summaryPath, `${JSON.stringify(sampleSummary, null, 2)}\n`, "utf8");
  const output = execFileSync(process.execPath, [
    planScript,
    "--clientweb-summary",
    summaryPath
  ], {
    encoding: "utf8"
  });
  assertPlan(JSON.parse(output));

  execFileSync(process.execPath, [
    planScript,
    "--result-dir",
    resultDir,
    "--output",
    outputPath
  ]);
  assertPlan(JSON.parse(readFileSync(outputPath, "utf8")));
  const outputPlan = JSON.parse(execFileSync(process.execPath, [
    planScript,
    "--clientweb-summary",
    summaryPath
  ], {
    encoding: "utf8"
  }));
  assertPlan(outputPlan);
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

const failedSummary = { ...sampleSummary, success: false };
assertThrows(() => buildBrowserMultiUserUISmokePlan({ summary: failedSummary }), "success=true");

console.log("browser multi-user UI smoke plan ok");

function assertPlan(plan) {
  const serialized = JSON.stringify(plan);
  assert(plan.schemaVersion === "nexusim.browser-multiuser-ui-smoke-plan.v1", "schema mismatch");
  assert(plan.executionPolicy.planOnly === true, "plan must be plan-only");
  assert(plan.executionPolicy.startsServices === false, "plan must not start services");
  assert(plan.executionPolicy.launchesBrowsers === false, "plan must not launch browsers");
  assert(plan.executionPolicy.contactsBFF === false, "plan must not contact BFF");
  assert(plan.executionPolicy.sendsMessages === false, "plan must not send messages");
  assert(plan.executionPolicy.writesProtectedMaterial === false, "plan must not write protected material");
  assert(plan.executionPolicy.downloadsToolchain === false, "plan must not download toolchains");
  assert(plan.source.clientWebSummaryVerified === true, "clientweb summary evidence missing");
  assert(plan.source.clientWebSummaryFile === "client-web-summary.json", "summary filename should be low-sensitive");
  assert(plan.endpoints.bffBaseURL === "http://127.0.0.1:18080", "BFF endpoint mismatch");
  assert(plan.endpoints.pushWebSocketURL === "ws://127.0.0.1:18088/ws", "push endpoint mismatch");
  assert(plan.actors.senderUserID === "sender-ui-smoke", "sender mismatch");
  assert(plan.actors.receiverUserID === "receiver-ui-smoke", "receiver mismatch");
  assert(plan.sensitiveInputPolicy.persistsLoginInput === false, "login proof must not be stored");
  assert(plan.sensitiveInputPolicy.persistsGatewaySessionMaterial === false, "gateway session material must not be stored");
  assert(plan.sensitiveInputPolicy.operatorSuppliesLoginInputAtRuntime === true, "runtime login input marker missing");
  assert(plan.prerequisites.some(step => step.step === "run-real-clientweb-smoke"), "real smoke prerequisite missing");
  assert(plan.prerequisites.some(step => step.step === "keep-local-stack-online-for-ui"), "keep-alive prerequisite missing");
  assert(plan.selectorContract.includes("friend-conversation-item"), "friend selector missing");
  assert(plan.selectorContract.includes("group-settings-members-tab"), "group member selector missing");
  assert(plan.selectorContract.includes("message-composer"), "message composer selector missing");
  assert(plan.selectorContract.includes("conversation-tags-input"), "conversation tags selector missing");
  assert(plan.selectorContract.includes("conversation-draft-input"), "conversation draft selector missing");
  assert(plan.selectorContract.includes("active-conversation-archive-toggle"), "conversation archive selector missing");
  assert(plan.selectorContract.includes("conversation-archived-only"), "archived-only filter selector missing");
  assert(plan.scenarios.directChat.conversationID === "direct-ui-smoke", "direct conversation mismatch");
  assert(plan.scenarios.directChat.expectedConversationSeq === 2, "direct seq mismatch");
  assert(plan.scenarios.directChat.uiFlow.some(step => step.includes("friend list")), "direct friend-click flow missing");
  assert(plan.scenarios.groupChat.conversationID === "group-ui-smoke", "group conversation mismatch");
  assert(plan.scenarios.groupChat.expectedAckSeq === 8, "group ack mismatch");
  assert(plan.scenarios.groupSettings.expectedProfileTitle === "NexusIM UI Smoke Group", "group profile title mismatch");
  assert(plan.scenarios.groupSettings.expectedFinalOwnerUserID === "receiver-ui-smoke", "final owner mismatch");
  assert(plan.scenarios.groupSettings.expectedRemovedUserID === "sender-ui-smoke", "removed member mismatch");
  assert(plan.scenarios.groupSettings.uiFlow.some(step => step.includes("without local fake members")), "group settings must reject fake members");
  assert(plan.scenarios.conversationManagement.directConversationID === "direct-ui-smoke", "conversation management direct mismatch");
  assert(plan.scenarios.conversationManagement.groupConversationID === "group-ui-smoke", "conversation management group mismatch");
  assert(plan.scenarios.conversationManagement.expectedTag === "ui-smoke", "conversation management tag mismatch");
  assert(plan.scenarios.conversationManagement.expectedDraftText === "NexusIM UI smoke draft", "conversation management draft mismatch");
  assert(plan.scenarios.conversationManagement.uiFlow.some(step => step.includes("archived-only")), "conversation management archive flow missing");
  assert(plan.focusedGate.command === "npm --prefix clients run check:no-toolchain", "focused gate command mismatch");
  assert(plan.focusedGate.startsServices === false, "focused gate must not start services");
  assert(!serialized.match(/authProof|gatewayToken|pushToken|refreshToken|secret|credential|private/i), "plan leaked sensitive field names");
  assert(!serialized.includes("SenderPassw0rd") && !serialized.includes("ReceiverPassw0rd"), "plan leaked runtime login input");
  assert(!serialized.match(/[A-Z]:\\\\/), "plan leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "plan leaked extended Windows path");
  assert(!serialized.match(/fallback/i), "plan must not describe fallback behavior");
}

function buildSampleSummary() {
  return {
    commit: "abc1234",
    git_dirty: false,
    success: true,
    bff_base_url: "http://127.0.0.1:18080",
    push_url: "ws://127.0.0.1:18088/ws",
    tenant_id: "tenant-ui-smoke",
    group_conversation_id: "group-ui-smoke",
    sender_user_id: "sender-ui-smoke",
    receiver_user_id: "receiver-ui-smoke",
    receiver_device_id: "receiver-device-ui-smoke",
    direct_chat: {
      conversation_id: "direct-ui-smoke",
      conversation_type: "DIRECT",
      send_message: {
        message_id: "msg-direct-ui-smoke",
        conversation_seq: 2
      },
      pull_inbox: {
        max_seq: 2
      },
      ack_delivery: {
        last_received_seq: 2
      }
    },
    group_chat: {
      conversation_id: "group-ui-smoke",
      conversation_type: "GROUP",
      send_message: {
        message_id: "msg-group-ui-smoke",
        conversation_seq: 8
      },
      pull_inbox: {
        max_seq: 8
      },
      ack_delivery: {
        last_received_seq: 8
      }
    },
    group_profile: {
      updated: {
        title: "NexusIM UI Smoke Group",
        avatar_uri: "nexusim://client-web-smoke/group-avatar.png"
      }
    },
    group_member_actions: {
      remove_member: {
        target_user_id: "sender-ui-smoke"
      },
      final: {
        members: [
          {
            user_id: "receiver-ui-smoke",
            role: "MEMBER_ROLE_OWNER"
          }
        ]
      }
    }
  };
}

function assertThrows(callback, expected) {
  try {
    callback();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    assert(message.includes(expected), `expected error to include ${expected}, got ${message}`);
    return;
  }
  throw new Error(`expected callback to throw ${expected}`);
}
