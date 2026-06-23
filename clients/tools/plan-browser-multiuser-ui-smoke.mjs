import { readFileSync, writeFileSync } from "node:fs";
import { basename, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const schemaVersion = "nexusim.browser-multiuser-ui-smoke-plan.v1";

function main() {
  const args = parseArgs(process.argv.slice(2));
  const plan = buildBrowserMultiUserUISmokePlan(args);
  const payload = `${JSON.stringify(plan, null, 2)}\n`;
  if (args.output) {
    writeFileSync(args.output, payload, "utf8");
    return;
  }
  process.stdout.write(payload);
}

export function buildBrowserMultiUserUISmokePlan(options = {}) {
  const summaryPath = resolveSummaryPath(options);
  const summary = options.summary ?? readClientWebSummary(summaryPath);
  assertSummary(summary);
  return {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    executionPolicy: executionPolicy(),
    source: {
      clientWebSummaryFile: summaryPath ? basename(summaryPath) : "client-web-summary.json",
      clientWebSummaryVerified: Boolean(summary.success),
      commit: summary.commit ?? "",
      gitDirty: Boolean(summary.git_dirty)
    },
    endpoints: {
      bffBaseURL: requiredString(summary.bff_base_url, "summary.bff_base_url"),
      pushWebSocketURL: requiredString(summary.push_url, "summary.push_url")
    },
    actors: {
      senderUserID: requiredString(summary.sender_user_id, "summary.sender_user_id"),
      receiverUserID: requiredString(summary.receiver_user_id, "summary.receiver_user_id"),
      receiverDeviceID: requiredString(summary.receiver_device_id, "summary.receiver_device_id")
    },
    prerequisites: prerequisites(summary),
    selectorContract: selectorContract(),
    scenarios: {
      directChat: directScenario(summary),
      groupChat: groupScenario(summary),
      groupSettings: groupSettingsScenario(summary),
      conversationManagement: conversationManagementScenario(summary)
    },
    sensitiveInputPolicy: {
      persistsLoginInput: false,
      persistsGatewaySessionMaterial: false,
      persistsRealtimeSessionMaterial: false,
      persistsRefreshSessionMaterial: false,
      operatorSuppliesLoginInputAtRuntime: true,
      note: "This plan is derived from real clientweb smoke evidence, but UI automation or manual runs must enter login proof at runtime."
    },
    focusedGate: {
      command: "npm --prefix clients run check:no-toolchain",
      startsServices: false,
      launchesBrowsers: false,
      downloadsToolchain: false,
      installsArtifacts: false
    }
  };
}

export function parseArgs(argv) {
  const args = {};
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--clientweb-summary" || arg === "--summary") {
      args.clientwebSummary = takeValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--result-dir") {
      args.resultDir = takeValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--output") {
      args.output = takeValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return args;
}

function resolveSummaryPath(options) {
  if (options.clientwebSummary) {
    return resolve(options.clientwebSummary);
  }
  if (options.resultDir) {
    return resolve(options.resultDir, "client-web-summary.json");
  }
  return "";
}

function readClientWebSummary(summaryPath) {
  if (!summaryPath) {
    throw new Error("missing --clientweb-summary or --result-dir");
  }
  return JSON.parse(readFileSync(summaryPath, "utf8"));
}

function executionPolicy() {
  return {
    planOnly: true,
    consumesClientWebSummary: true,
    startsServices: false,
    launchesBrowsers: false,
    launchesDesktopShell: false,
    contactsBFF: false,
    opensPushWebSocket: false,
    sendsMessages: false,
    writesProtectedMaterial: false,
    downloadsToolchain: false
  };
}

function prerequisites(summary) {
  return [
    {
      step: "run-real-clientweb-smoke",
      command: ".\\loadtest\\clientweb\\run-local-smoke.ps1 -BindHost 127.0.0.1 -ClientHost 127.0.0.1",
      evidence: "client-web-summary.json success=true with direct_chat and group_chat evidence"
    },
    {
      step: "keep-local-stack-online-for-ui",
      command: ".\\loadtest\\clientweb\\run-local-smoke.ps1 -KeepAlive -BindHost 127.0.0.1 -ClientHost 127.0.0.1",
      evidence: "BFF and push endpoints remain available for browser / PC UI verification"
    },
    {
      step: "open-web-shell",
      command: "npm --prefix clients run dev:web",
      evidence: `operator signs in sender=${summary.sender_user_id} and receiver=${summary.receiver_user_id} in separate browser or PC shell sessions`
    }
  ];
}

function selectorContract() {
  return [
    "login-user",
    "login-password",
    "login-submit",
    "conversation-item",
    "friend-conversation-item",
    "message-list",
    "message-item",
    "message-status",
    "message-composer",
    "send-message",
    "conversation-tag-filter",
    "conversation-include-archived",
    "conversation-archived-only",
    "conversation-draft-only",
    "active-conversation-actions",
    "active-conversation-archive-toggle",
    "conversation-archive-toggle",
    "conversation-tags-input",
    "conversation-tags-save",
    "conversation-draft-input",
    "conversation-draft-save",
    "conversation-draft-clear",
    "group-settings-tabs",
    "group-settings-profile-tab",
    "group-settings-members-tab",
    "group-settings-actions-tab",
    "group-profile-title-input",
    "group-profile-save",
    "group-member-item",
    "group-invite-user",
    "group-invite-submit",
    "group-member-search",
    "group-member-role-filter",
    "group-member-filter-submit"
  ];
}

function directScenario(summary) {
  const direct = requiredObject(summary.direct_chat, "summary.direct_chat");
  return {
    conversationID: requiredString(direct.conversation_id, "summary.direct_chat.conversation_id"),
    conversationType: requiredString(direct.conversation_type, "summary.direct_chat.conversation_type"),
    expectedMessageID: requiredString(direct.send_message?.message_id, "summary.direct_chat.send_message.message_id"),
    expectedConversationSeq: requiredPositiveNumber(direct.send_message?.conversation_seq, "summary.direct_chat.send_message.conversation_seq"),
    expectedReceiverPullMaxSeq: requiredPositiveNumber(direct.pull_inbox?.max_seq, "summary.direct_chat.pull_inbox.max_seq"),
    expectedAckSeq: requiredPositiveNumber(direct.ack_delivery?.last_received_seq, "summary.direct_chat.ack_delivery.last_received_seq"),
    uiFlow: [
      "sign in sender and receiver with operator-provided auth proofs",
      "sender clicks the receiver in the friend list to open direct chat",
      "sender sends a text message through message-composer",
      "receiver syncs or receives push wakeup and sees the message in message-list",
      "receiver AckDelivery advances to the expected direct chat sequence"
    ]
  };
}

function groupScenario(summary) {
  const group = requiredObject(summary.group_chat, "summary.group_chat");
  return {
    conversationID: requiredString(group.conversation_id, "summary.group_chat.conversation_id"),
    conversationType: requiredString(group.conversation_type, "summary.group_chat.conversation_type"),
    expectedMessageID: requiredString(group.send_message?.message_id, "summary.group_chat.send_message.message_id"),
    expectedConversationSeq: requiredPositiveNumber(group.send_message?.conversation_seq, "summary.group_chat.send_message.conversation_seq"),
    expectedReceiverPullMaxSeq: requiredPositiveNumber(group.pull_inbox?.max_seq, "summary.group_chat.pull_inbox.max_seq"),
    expectedAckSeq: requiredPositiveNumber(group.ack_delivery?.last_received_seq, "summary.group_chat.ack_delivery.last_received_seq"),
    uiFlow: [
      "sender opens the group from the group conversation list",
      "sender sends a text message through message-composer",
      "receiver opens the same group and verifies PullInbox message visibility",
      "receiver AckDelivery advances to the expected group chat sequence"
    ]
  };
}

function groupSettingsScenario(summary) {
  const profile = requiredObject(summary.group_profile, "summary.group_profile");
  const members = requiredObject(summary.group_member_actions, "summary.group_member_actions");
  return {
    conversationID: requiredString(summary.group_conversation_id, "summary.group_conversation_id"),
    expectedProfileTitle: requiredString(profile.updated?.title, "summary.group_profile.updated.title"),
    expectedAvatarURI: requiredString(profile.updated?.avatar_uri, "summary.group_profile.updated.avatar_uri"),
    expectedFinalOwnerUserID: requiredString(members.final?.members?.find(member => member.role === "MEMBER_ROLE_OWNER")?.user_id, "summary.group_member_actions.final owner"),
    expectedRemovedUserID: requiredString(members.remove_member?.target_user_id, "summary.group_member_actions.remove_member.target_user_id"),
    uiFlow: [
      "open group settings profile tab and verify title / avatar URI from BFF profile",
      "open group settings members tab and verify real BFF member list",
      "use search, role filter and pagination controls without local fake members",
      "open group settings actions tab for invite, leave and owner-sensitive operations",
      "verify removed member is absent in final BFF member list evidence"
    ]
  };
}

function conversationManagementScenario(summary) {
  const direct = requiredObject(summary.direct_chat, "summary.direct_chat");
  const group = requiredObject(summary.group_chat, "summary.group_chat");
  return {
    directConversationID: requiredString(direct.conversation_id, "summary.direct_chat.conversation_id"),
    groupConversationID: requiredString(group.conversation_id, "summary.group_chat.conversation_id"),
    expectedTag: "ui-smoke",
    expectedDraftText: "NexusIM UI smoke draft",
    uiFlow: [
      "select a real BFF conversation summary and open the active conversation management panel",
      "save conversation tags through api-gateway BFF and verify the list can be filtered by that tag",
      "save and clear a conversation draft through api-gateway BFF without treating local state as fact",
      "archive the conversation, reload archived-only summaries, then unarchive it through the same BFF-backed control"
    ]
  };
}

function assertSummary(summary) {
  if (!summary || typeof summary !== "object") {
    throw new Error("clientweb summary must be an object");
  }
  if (summary.success !== true) {
    throw new Error("clientweb summary must have success=true before deriving a UI smoke plan");
  }
  for (const key of [
    "bff_base_url",
    "push_url",
    "tenant_id",
    "sender_user_id",
    "receiver_user_id",
    "receiver_device_id",
    "direct_chat",
    "group_chat",
    "group_profile",
    "group_member_actions"
  ]) {
    if (summary[key] === undefined || summary[key] === null || summary[key] === "") {
      throw new Error(`clientweb summary missing ${key}`);
    }
  }
}

function requiredString(value, name) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`missing required string: ${name}`);
  }
  return value;
}

function requiredPositiveNumber(value, name) {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) {
    throw new Error(`missing required positive number: ${name}`);
  }
  return value;
}

function requiredObject(value, name) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`missing required object: ${name}`);
  }
  return value;
}

function takeValue(argv, index, arg) {
  const value = argv[index + 1];
  if (!value) {
    throw new Error(`${arg} requires a value`);
  }
  return value;
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
