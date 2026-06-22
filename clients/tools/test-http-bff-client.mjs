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
      createConversation: "/api/conversations/create",
      directConversation: "/api/conversations/direct",
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
      assert.deepEqual(JSON.parse(String(init?.body)), {
        peer_user_id: "user-b",
        idempotency_key: "idem-direct"
      });
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
        peerUserID: "user-b",
        idempotencyKey: "idem-direct"
      },
      session()
    );
    assert.equal(direct.conversationID, "direct-user-a-user-b");
    assert.equal(direct.type, "DIRECT");
    assert.equal(direct.directPeerUserID, "user-b");
    assert.equal(direct.boundarySeq, 2);
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
    assert.equal(calls.length, 10);
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
