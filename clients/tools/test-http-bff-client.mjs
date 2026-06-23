import assert from "node:assert/strict";
import { Buffer } from "node:buffer";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const root = fileURLToPath(new URL("..", import.meta.url));

async function main() {
  const sourcePath = join(root, "packages/client-core/src/http-bff-client.ts");
  const source = readFileSync(sourcePath, "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022
    }
  }).outputText;
  const patched = compiled.replace(
    /import\s+\{[\s\S]*?CLIENT_API_ENDPOINTS[\s\S]*?\}\s+from\s+"@nexusim\/protocol";/,
    `const CLIENT_API_ENDPOINTS = {
      register: "/api/auth/register",
      conversations: "/api/conversations",
      createConversation: "/api/conversations/create",
      directConversation: "/api/conversations/direct",
      pinConversation: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/pin",
      muteConversation: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/mute",
      archiveConversation: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/archive",
      setConversationTags: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/tags",
      setConversationDraft: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/draft",
      conversationProfile: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/profile",
      conversationMembers: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/members",
      inviteConversationMember: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/members/invite",
      leaveConversation: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/members/leave",
      removeConversationMember: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/members/remove",
      updateConversationMemberRole: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/members/role",
      transferConversationOwner: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/owner/transfer",
      conversationMessages: conversationID => "/api/conversations/" + encodeURIComponent(conversationID) + "/messages"
    };`
  );
  assert(!patched.includes("@nexusim/protocol"), "test module must not depend on workspace package resolution");

  const moduleURL = `data:text/javascript;base64,${Buffer.from(patched).toString("base64")}`;
  const { BFFClient } = await import(moduleURL);
  const originalFetch = globalThis.fetch;
  const expectedText = "PC端发中文不应在网页端乱码";
  const calls = [];

  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    const url = String(input);
    if (url.endsWith("/api/auth/register")) {
      assert.equal(init?.method, "POST");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        tenant_id: "tenant-client-local",
        user_id: "user-new",
        password: "pw"
      });
      return new Response(
        JSON.stringify({
          tenant_id: "tenant-client-local",
          user_id: "user-new",
          status: "USER_STATUS_ACTIVE",
          created_at_unix_ms: "1782112000000"
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/create")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        conversation_id: "group-client-local",
        conversation_type: "CONVERSATION_TYPE_GROUP",
        idempotency_key: "idem-group"
      });
      return new Response(
        JSON.stringify({
          tenant_id: "tenant-client-local",
          conversation_id: "group-client-local",
          conversation_type: "CONVERSATION_TYPE_GROUP",
          boundary_seq: "1",
          member_version: "1",
          permission_version: "1"
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/direct")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      const body = JSON.parse(String(init?.body));
      assert.equal(body.peer_user_id, "user-b");
      assert.match(body.idempotency_key, /^direct-user-b-[a-z0-9-]+$/);
      return new Response(
        JSON.stringify({
          tenant_id: "tenant-client-local",
          conversation_id: "direct-user-a-user-b",
          conversation_type: "CONVERSATION_TYPE_DIRECT",
          direct_peer_user_id: "user-b",
          boundary_seq: "2",
          member_version: "2",
          permission_version: "2"
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url === "http://bff.local/api/conversations?limit=20") {
      assert.equal(init?.method, "GET");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      return new Response(
        JSON.stringify({
          items: [
            {
              conversation_id: "direct-user-a-user-b",
              conversation_type: "CONVERSATION_TYPE_DIRECT",
              title: "user-b",
              last_visible_seq: "12",
              unread_count: "2"
            },
            {
              conversation_id: "receipt-only-conversation",
              last_visible_seq: "1",
              unread_count: "0"
            }
          ]
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (
      url ===
      "http://bff.local/api/conversations?limit=50&include_archived=true&archived_only=true&draft_only=true&tag_filter=ui-smoke&tag_filters=urgent&tag_filters=later"
    ) {
      assert.equal(init?.method, "GET");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      return new Response(
        JSON.stringify({
          items: [
            {
              conversation_id: "group-client-local",
              conversation_type: "CONVERSATION_TYPE_GROUP",
              title: "研发二群",
              last_visible_seq: "14",
              unread_count: "0",
              archived: true,
              pinned: true,
              muted: true,
              tags: ["ui-smoke", "urgent", "ui-smoke"],
              draft_text: "待发送中文草稿",
              draft_updated_at_unix_ms: "1782112000400"
            }
          ]
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/archive")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        archived: true
      });
      return new Response(
        JSON.stringify({
          conversation: {
            conversation_id: "group-client-local",
            conversation_type: "CONVERSATION_TYPE_GROUP",
            title: "研发二群",
            last_visible_seq: "14",
            archived: true,
            tags: ["ui-smoke"],
            draft_text: "待发送中文草稿"
          }
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/tags")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        tags: ["ui-smoke", "urgent"]
      });
      return new Response(
        JSON.stringify({
          conversation: {
            conversation_id: "group-client-local",
            conversation_type: "CONVERSATION_TYPE_GROUP",
            title: "研发二群",
            last_visible_seq: "14",
            tags: ["ui-smoke", "urgent"]
          }
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/draft")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        draft_text: "待发送中文草稿"
      });
      return new Response(
        JSON.stringify({
          conversation: {
            conversation_id: "group-client-local",
            conversation_type: "CONVERSATION_TYPE_GROUP",
            title: "研发二群",
            last_visible_seq: "14",
            draft_text: "待发送中文草稿",
            draft_updated_at_unix_ms: "1782112000500"
          }
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/profile") && init?.method === "GET") {
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      return new Response(
        JSON.stringify({
          profile: {
            tenant_id: "tenant-client-local",
            conversation_id: "group-client-local",
            conversation_type: "CONVERSATION_TYPE_GROUP",
            title: "研发群",
            avatar_uri: "media://avatar/group-client-local",
            profile_version: "7",
            member_version: "3",
            permission_version: "2",
            updated_at_unix_ms: "1782112000200"
          }
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/profile") && init?.method === "POST") {
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        title: "研发二群",
        avatar_uri: "media://avatar/group-client-local-v2",
        expected_profile_version: 7
      });
      return new Response(
        JSON.stringify({
          profile: {
            tenant_id: "tenant-client-local",
            conversation_id: "group-client-local",
            conversation_type: "CONVERSATION_TYPE_GROUP",
            title: "研发二群",
            avatar_uri: "media://avatar/group-client-local-v2",
            profile_version: "8",
            member_version: "3",
            permission_version: "2",
            updated_at_unix_ms: "1782112000300"
          }
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/members/invite")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        target_user_id: "user-c",
        expected_member_version: 2,
        idempotency_key: "idem-invite",
        reason: "client invite"
      });
      return new Response(
        JSON.stringify({
          change_id: "change-invite-1",
          tenant_id: "tenant-client-local",
          conversation_id: "group-client-local",
          target_user_id: "user-c",
          change_type: "MEMBER_CHANGE_TYPE_JOIN",
          status: "MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED",
          boundary_seq: "3",
          member_version: "3",
          permission_version: "2"
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/members?page_size=100")) {
      assert.equal(init?.method, "GET");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      return new Response(
        JSON.stringify({
          tenant_id: "tenant-client-local",
          conversation_id: "group-client-local",
          member_version: "3",
          permission_version: "2",
          members: [
            {
              user_id: "user-a",
              role: "MEMBER_ROLE_OWNER",
              status: "MEMBER_STATUS_ACTIVE",
              join_seq: "1",
              member_version: "3",
              permission_version: "2",
              updated_at_unix_ms: "1782112000000"
            },
            {
              user_id: "user-c",
              role: "MEMBER_ROLE_MEMBER",
              status: "MEMBER_STATUS_ACTIVE",
              join_seq: "3",
              member_version: "3",
              permission_version: "2",
              updated_at_unix_ms: "1782112000001"
            }
          ],
          next_page_token: ""
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/members/remove")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        target_user_id: "user-c",
        expected_member_version: 3,
        idempotency_key: "idem-remove",
        reason: "client remove"
      });
      return new Response(
        JSON.stringify({
          change_id: "change-remove-1",
          tenant_id: "tenant-client-local",
          conversation_id: "group-client-local",
          target_user_id: "user-c",
          change_type: "MEMBER_CHANGE_TYPE_REMOVE",
          status: "MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED",
          boundary_seq: "4",
          member_version: "4",
          permission_version: "2"
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/members/role")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        target_user_id: "user-c",
        target_role: "MEMBER_ROLE_ADMIN",
        expected_member_version: 4,
        idempotency_key: "idem-role",
        reason: "client role"
      });
      return new Response(
        JSON.stringify({
          change_id: "change-role-1",
          tenant_id: "tenant-client-local",
          conversation_id: "group-client-local",
          target_user_id: "user-c",
          change_type: "MEMBER_CHANGE_TYPE_ROLE_CHANGED",
          status: "MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED",
          boundary_seq: "5",
          member_version: "5",
          permission_version: "2"
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/owner/transfer")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        new_owner_user_id: "user-c",
        expected_member_version: 5,
        idempotency_key: "idem-transfer",
        reason: "client owner transfer"
      });
      return new Response(
        JSON.stringify({
          change_id: "change-transfer-1",
          tenant_id: "tenant-client-local",
          conversation_id: "group-client-local",
          previous_owner_user_id: "user-a",
          new_owner_user_id: "user-c",
          status: "MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED",
          boundary_seq: "6",
          member_version: "6",
          permission_version: "2"
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    if (url.endsWith("/api/conversations/group-client-local/members/leave")) {
      assert.equal(init?.method, "POST");
      assert.equal(init?.headers?.Authorization, "Bearer gateway-token");
      assert.deepEqual(JSON.parse(String(init?.body)), {
        expected_member_version: 6,
        idempotency_key: "idem-leave",
        reason: "client leave"
      });
      return new Response(
        JSON.stringify({
          change_id: "change-leave-1",
          tenant_id: "tenant-client-local",
          conversation_id: "group-client-local",
          target_user_id: "user-a",
          change_type: "MEMBER_CHANGE_TYPE_LEAVE",
          status: "MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED",
          boundary_seq: "4",
          member_version: "4",
          permission_version: "2"
        }),
        { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
      );
    }
    assert.equal(url, "http://bff.local/api/conversations/conv-client-local/messages?after_seq=0&limit=10");
    assert.equal(init?.method, "GET");
    return new Response(
      JSON.stringify({
        items: [
          {
            conversation_id: "conv-client-local",
            conversation_seq: "11",
            event_id: "evt-11",
            message_id: "msg-11",
            sender_id: "user-a",
            payload_json: Buffer.from(JSON.stringify({ text: expectedText }), "utf8").toString("base64"),
            created_at_unix_ms: "1782112000000"
          }
        ],
        next_seq: "11"
      }),
      { status: 200, headers: { "content-type": "application/json; charset=utf-8" } }
    );
  };

  try {
    const client = new BFFClient("http://bff.local");
    const register = await client.register({
      tenantID: "tenant-client-local",
      userID: "user-new",
      password: "pw"
    });
    assert.equal(register.status, "ACTIVE");
    const created = await client.createConversation(
      {
        tenantID: "tenant-client-local",
        conversationID: "group-client-local",
        type: "GROUP",
        idempotencyKey: "idem-group"
      },
      session()
    );
    assert.equal(created.conversationID, "group-client-local");
    assert.equal(created.boundarySeq, 1);
    const direct = await client.openDirectConversation(
      {
        peerUserID: "user-b"
      },
      session()
    );
    assert.equal(direct.conversationID, "direct-user-a-user-b");
    assert.equal(direct.type, "DIRECT");
    assert.equal(direct.directPeerUserID, "user-b");
    assert.equal(direct.boundarySeq, 2);
    const conversationList = await client.listConversations(session(), { limit: 20 });
    assert.equal(conversationList[0]?.type, "DIRECT");
    assert.equal(conversationList[0]?.title, "user-b");
    assert.equal(conversationList[0]?.lastSeq, 12);
    assert.equal(conversationList[0]?.unreadCount, 2);
    assert.equal(conversationList[1]?.type, "UNKNOWN");
    assert.equal(conversationList[1]?.title, "Conversation receipt-only-conversation");
    const managedList = await client.listConversations(session(), {
      limit: 50,
      includeArchived: true,
      archivedOnly: true,
      draftOnly: true,
      tagFilter: "ui-smoke",
      tagFilters: ["urgent", "later"]
    });
    assert.equal(managedList[0]?.conversationID, "group-client-local");
    assert.equal(managedList[0]?.archived, true);
    assert.equal(managedList[0]?.pinned, true);
    assert.equal(managedList[0]?.muted, true);
    assert.deepEqual(managedList[0]?.tags, ["ui-smoke", "urgent"]);
    assert.equal(managedList[0]?.draftText, "待发送中文草稿");
    assert.equal(managedList[0]?.draftUpdatedAtMs, 1782112000400);
    const archivedConversation = await client.archiveConversation(
      {
        conversationID: "group-client-local",
        archived: true
      },
      session()
    );
    assert.equal(archivedConversation.archived, true);
    const taggedConversation = await client.setConversationTags(
      {
        conversationID: "group-client-local",
        tags: ["ui-smoke", "urgent"]
      },
      session()
    );
    assert.deepEqual(taggedConversation.tags, ["ui-smoke", "urgent"]);
    const draftConversation = await client.setConversationDraft(
      {
        conversationID: "group-client-local",
        draftText: "待发送中文草稿"
      },
      session()
    );
    assert.equal(draftConversation.draftText, "待发送中文草稿");
    const profile = await client.getConversationProfile("group-client-local", session());
    assert.equal(profile.title, "研发群");
    assert.equal(profile.avatarURI, "media://avatar/group-client-local");
    assert.equal(profile.profileVersion, 7);
    const updatedProfile = await client.updateConversationProfile(
      {
        conversationID: "group-client-local",
        title: "研发二群",
        avatarURI: "media://avatar/group-client-local-v2",
        expectedProfileVersion: 7
      },
      session()
    );
    assert.equal(updatedProfile.title, "研发二群");
    assert.equal(updatedProfile.profileVersion, 8);
    const invited = await client.inviteConversationMember(
      {
        conversationID: "group-client-local",
        targetUserID: "user-c",
        expectedMemberVersion: 2,
        idempotencyKey: "idem-invite",
        reason: "client invite"
      },
      session()
    );
    assert.equal(invited.changeType, "JOIN");
    assert.equal(invited.memberVersion, 3);
    const members = await client.listConversationMembers(
      {
        conversationID: "group-client-local",
        pageSize: 100
      },
      session()
    );
    assert.equal(members.members.length, 2);
    assert.equal(members.members[0]?.role, "OWNER");
    const removed = await client.removeConversationMember(
      {
        conversationID: "group-client-local",
        targetUserID: "user-c",
        expectedMemberVersion: 3,
        idempotencyKey: "idem-remove",
        reason: "client remove"
      },
      session()
    );
    assert.equal(removed.changeType, "REMOVE");
    assert.equal(removed.memberVersion, 4);
    const roleChanged = await client.updateConversationMemberRole(
      {
        conversationID: "group-client-local",
        targetUserID: "user-c",
        targetRole: "ADMIN",
        expectedMemberVersion: 4,
        idempotencyKey: "idem-role",
        reason: "client role"
      },
      session()
    );
    assert.equal(roleChanged.changeType, "ROLE_CHANGED");
    assert.equal(roleChanged.memberVersion, 5);
    const transferred = await client.transferConversationOwner(
      {
        conversationID: "group-client-local",
        newOwnerUserID: "user-c",
        expectedMemberVersion: 5,
        idempotencyKey: "idem-transfer",
        reason: "client owner transfer"
      },
      session()
    );
    assert.equal(transferred.newOwnerUserID, "user-c");
    assert.equal(transferred.memberVersion, 6);
    const left = await client.leaveConversation(
      {
        conversationID: "group-client-local",
        expectedMemberVersion: 6,
        idempotencyKey: "idem-leave",
        reason: "client leave"
      },
      session()
    );
    assert.equal(left.changeType, "LEAVE");
    assert.equal(left.targetUserID, "user-a");
    const response = await client.pullInbox(
      { conversationID: "conv-client-local", afterSeq: 0, limit: 10 },
      session()
    );
    assert.equal(response.items[0]?.text, expectedText);
    assert.equal(response.nextSeq, 11);
    assert.equal(calls.length, 17);
  } finally {
    globalThis.fetch = originalFetch;
  }

  console.log("http bff client utf8 payload ok");
}

function session() {
  return {
    tenantID: "tenant-client-local",
    userID: "user-a",
    deviceID: "web-local-device",
    sessionID: "session-local",
    accessToken: "gateway-token"
  };
}

await main();
