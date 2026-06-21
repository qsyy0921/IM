import { readFileSync } from "node:fs";
import { Buffer } from "node:buffer";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import ts from "typescript";

const root = fileURLToPath(new URL("..", import.meta.url));

async function main() {
  const sourcePath = join(root, "packages/client-core/src/key-value-message-store.ts");
  const source = readFileSync(sourcePath, "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022
    }
  }).outputText;

  const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`;
  const { KeyValueMessageStore, MemoryStringKeyValueStorage } = await import(moduleURL);
  const storage = new MemoryStringKeyValueStorage();

  const first = new KeyValueMessageStore(storage, { namespace: "desktop-replay-test" });
  assertEqual(await first.getLastReceivedSeq("conv-1"), 0, "new persistent store cursor starts at 0");

  await first.upsertMessages([
    message("conv-1", "msg-1", 1),
    message("conv-1", "msg-2", 2)
  ]);

  const reopened = new KeyValueMessageStore(storage, { namespace: "desktop-replay-test" });
  assertEqual(await reopened.getLastReceivedSeq("conv-1"), 2, "cursor persists across store instances");
  assertDeepEqual(
    (await reopened.listMessages("conv-1")).map(item => item.messageID),
    ["msg-1", "msg-2"],
    "messages persist across store instances"
  );

  await reopened.markPending({
    tenantID: "tenant-1",
    conversationID: "conv-1",
    messageID: "",
    senderUserID: "user-1",
    conversationSeq: 0,
    contentType: "TEXT",
    text: "pending",
    clientMessageID: "local-1",
    status: "PENDING",
    createdAtMs: 3
  });

  const afterPendingRestart = new KeyValueMessageStore(storage, { namespace: "desktop-replay-test" });
  assertEqual(
    (await afterPendingRestart.listMessages("conv-1")).find(item => item.clientMessageID === "local-1")?.status,
    "PENDING",
    "pending send survives restart"
  );

  await afterPendingRestart.markSendAccepted("local-1", {
    messageID: "msg-3",
    conversationID: "conv-1",
    conversationSeq: 3
  });

  const afterAcceptRestart = new KeyValueMessageStore(storage, { namespace: "desktop-replay-test" });
  await afterAcceptRestart.upsertMessages([message("conv-1", "msg-3", 3)]);
  assertEqual(await afterAcceptRestart.getLastReceivedSeq("conv-1"), 3, "accepted send cursor persists");
  assertDeepEqual(
    (await afterAcceptRestart.listMessages("conv-1")).map(item => item.messageID),
    ["msg-1", "msg-2", "msg-3"],
    "accepted send migrates pending key without replay duplicate after restart"
  );

  await afterAcceptRestart.markPending({
    tenantID: "tenant-1",
    conversationID: "conv-1",
    messageID: "",
    senderUserID: "user-1",
    conversationSeq: 0,
    contentType: "TEXT",
    text: "will fail",
    clientMessageID: "local-2",
    status: "PENDING",
    createdAtMs: 4
  });
  await afterAcceptRestart.markSendFailed("local-2", "network failed");

  const afterFailureRestart = new KeyValueMessageStore(storage, { namespace: "desktop-replay-test" });
  assertEqual(
    (await afterFailureRestart.listMessages("conv-1")).find(item => item.clientMessageID === "local-2")?.status,
    "FAILED",
    "failed send status survives restart"
  );

  assertDeepEqual(
    await afterFailureRestart.listConversationsNeedingSync(),
    ["conv-1"],
    "persistent cursor store lists conversations needing sync"
  );

  console.log("key-value message store persistence ok");
}

function message(conversationID, messageID, seq) {
  return {
    tenantID: "tenant-1",
    conversationID,
    messageID,
    senderUserID: "user-1",
    conversationSeq: seq,
    contentType: "TEXT",
    text: `message ${seq}`,
    status: "SENT",
    createdAtMs: seq
  };
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: expected ${expected}, got ${actual}`);
  }
}

function assertDeepEqual(actual, expected, message) {
  const actualJSON = JSON.stringify(actual);
  const expectedJSON = JSON.stringify(expected);
  if (actualJSON !== expectedJSON) {
    throw new Error(`${message}: expected ${expectedJSON}, got ${actualJSON}`);
  }
}

await main();
