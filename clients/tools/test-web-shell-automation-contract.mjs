import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const root = fileURLToPath(new URL("..", import.meta.url));
const appSource = readFileSync(join(root, "web/src/App.tsx"), "utf8");

const requiredTestIDs = [
  "login-tenant",
  "login-user",
  "login-password",
  "login-submit",
  "logout-submit",
  "refresh-session",
  "restore-session",
  "register-submit",
  "conversation-id-input",
  "open-conversation",
  "refresh-conversations",
  "create-group",
  "conversation-item",
  "friend-conversation-item",
  "group-invite-user",
  "group-invite-submit",
  "group-invite-source",
  "group-leave-submit",
  "group-profile-card",
  "group-profile-avatar",
  "group-profile-title",
  "group-profile-subtitle",
  "group-profile-announcement",
  "group-settings-tabs",
  "group-settings-profile-tab",
  "group-settings-members-tab",
  "group-settings-actions-tab",
  "group-profile-id",
  "group-profile-member-count",
  "group-profile-title-input",
  "group-profile-avatar-input",
  "group-profile-announcement-input",
  "group-profile-save",
  "group-profile-error",
  "group-permission-status",
  "group-self-role",
  "group-members-refresh",
  "group-member-search",
  "group-member-role-filter",
  "group-member-filter-submit",
  "group-member-filter-reset",
  "group-member-page-status",
  "group-member-prev-page",
  "group-member-next-page",
  "group-settings-members-panel",
  "group-settings-actions-panel",
  "group-member-item",
  "group-invite-contact-picker",
  "group-invite-contact-query",
  "group-invite-contact-submit",
  "runtime-status",
  "push-status",
  "ack-status",
  "native-store-readiness",
  "error-banner",
  "message-list",
  "message-item",
  "message-status",
  "message-edit-failed",
  "message-resend-as-new",
  "message-composer",
  "send-message"
];

for (const testID of requiredTestIDs) {
  assertIncludes(appSource, `data-testid="${testID}"`, `web shell automation selector missing: ${testID}`);
}

assertIncludes(appSource, "setLastAck({ conversationID, seq: maxSeq })", "web shell must expose AckDelivery progress after sync");
assertIncludes(appSource, "runtime.ackQueue.flush(currentSession)", "web shell must keep AckDelivery inside shared runtime path");
assertIncludes(appSource, "openDirectConversation(contact)", "web shell must let users click a friend to open direct chat");
assertIncludes(appSource, "data-conversation-id={conversation.conversationID}", "web shell must expose low-sensitive conversation id metadata for UI smoke automation");
assertIncludes(appSource, "data-contact-user-id={contact.contactUserID}", "web shell must expose low-sensitive contact id metadata for UI smoke automation");
assertIncludes(appSource, "inviteGroupMember", "web shell must expose group member invite action");
assertIncludes(appSource, "leaveGroupConversation", "web shell must expose group leave action");
assertIncludes(appSource, "loadGroupMembers", "web shell must load real group members through BFF");
assertIncludes(appSource, "loadGroupProfile", "web shell must load group profile through BFF");
assertIncludes(appSource, "saveGroupProfile", "web shell must expose group profile update action");
assertIncludes(appSource, "updateConversationProfile", "web shell must update group profile through client-core BFF");
assertIncludes(appSource, "getConversationProfile", "web shell must read group profile through client-core BFF");
assertIncludes(appSource, "userIDPrefix", "web shell must use public BFF member search instead of local-only fake filtering");
assertIncludes(appSource, "pageToken", "web shell must use public BFF member pagination tokens");
assertIncludes(appSource, "roleFilter", "web shell must use public BFF member role filter");
assertIncludes(appSource, "removeGroupMember", "web shell must expose group member removal action");
assertIncludes(appSource, "updateGroupMemberRole", "web shell must expose group member role action");
assertIncludes(appSource, "transferGroupOwner", "web shell must expose owner transfer action");
assertIncludes(appSource, "requireActiveGroupConversation", "web shell must gate group actions to group conversations");
assertIncludes(appSource, "GroupSettingsTab", "web shell must keep group settings sections explicit");
assertIncludes(appSource, "setGroupSettingsTab(\"profile\")", "web shell must reset group settings to profile when selecting a group");
assertIncludes(appSource, "groupSettingsTab === \"members\"", "web shell must separate group member management from profile editing");
assertIncludes(appSource, "groupSettingsTab === \"actions\"", "web shell must separate explicit group actions from member browsing");
assertIncludes(appSource, "chooseActiveConversationID", "web shell must preserve selected conversation during refresh");
assertIncludes(appSource, "clearExpiredSession", "web shell must clear UI state when gateway token expires");
assertIncludes(appSource, "nativeMetadata?.capabilities?.localStore", "web shell must display native local-store readiness when available");
assertIncludes(appSource, "nativeLocalStoreStatus", "web shell must keep local-store readiness formatting explicit");
assertIncludes(appSource, "mergeConversationSummaries", "web shell must preserve local display titles across conversation refresh");
assertIncludes(appSource, "mergeConversationSummary", "web shell must merge server conversation summaries through one explicit helper");
assertIncludes(appSource, "incoming.type === \"UNKNOWN\" ? existing.type : incoming.type", "web shell must not let receipt-only summaries overwrite known direct/group types");
assertIncludes(appSource, "titleFromConversationID(activeConversationID, \"UNKNOWN\")", "web shell must not label unknown selected conversations as groups");
assertIncludes(appSource, "conversationDisplayTitle", "web shell must keep conversation title formatting explicit");
assertIncludes(appSource, "conversationStatusLabel", "web shell must keep conversation status formatting explicit");
assertIncludes(appSource, "messageStatusLabel", "web shell must keep message status formatting explicit");
assertIncludes(appSource, "restoreFailedMessageToComposer", "web shell must let users explicitly edit failed sends");
assertIncludes(appSource, "resendFailedMessageAsNew", "web shell must make failed-send resend a new explicit send");
assertIncludes(appSource, "message.status === \"FAILED\"", "web shell must gate failed-message actions on FAILED status");
assertIncludes(appSource, "emptyMessageState", "web shell must keep empty-state copy explicit");
assertIncludes(appSource, "publicErrorMessage", "web shell must map common public errors to user-facing copy");

console.log("web shell automation contract ok");

function assertIncludes(source, expected, message) {
  if (!source.includes(expected)) {
    throw new Error(`${message}: missing ${expected}`);
  }
}
