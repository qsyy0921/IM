import { readFileSync } from "node:fs";
import { Buffer } from "node:buffer";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import ts from "typescript";

const root = fileURLToPath(new URL("..", import.meta.url));

async function main() {
  const sourcePath = join(root, "packages/client-core/src/development-adapters.ts");
  const source = readFileSync(sourcePath, "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022
    }
  }).outputText;

  const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`;
  const { MemoryMessageStore } = await import(moduleURL);
  const store = new MemoryMessageStore();

  assertDeepEqual(await store.listMessages("conv-1"), [], "new store starts without cached messages");
  assertEqual(await store.getLastReceivedSeq("conv-1"), 0, "new store cursor starts at 0");

  await store.upsertMessages([
    message("conv-1", "msg-2", 2),
    message("conv-1", "msg-1", 1),
    message("conv-2", "msg-5", 5)
  ]);

  assertDeepEqual(
    (await store.listMessages("conv-1")).map(item => item.messageID),
    ["msg-1", "msg-2"],
    "messages are listed by conversation seq"
  );
  assertEqual(await store.getLastReceivedSeq("conv-1"), 2, "upsert records max received seq");
  assertDeepEqual(await store.listConversationsNeedingSync(), ["conv-1", "conv-2"], "store lists conversations needing sync");

  await store.markPending({
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
  assertEqual(
    (await store.listMessages("conv-1")).find(item => item.clientMessageID === "local-1")?.status,
    "PENDING",
    "pending send is readable from shared store port"
  );

  await store.markSendAccepted("local-1", {
    messageID: "msg-3",
    conversationID: "conv-1",
    conversationSeq: 3
  });
  await store.upsertMessages([message("conv-1", "msg-3", 3)]);
  assertDeepEqual(
    (await store.listMessages("conv-1")).map(item => item.messageID),
    ["msg-1", "msg-2", "msg-3"],
    "accepted send migrates pending key without duplicate after replay"
  );

  await store.clear();
  assertDeepEqual(await store.listMessages("conv-1"), [], "clear removes cached messages");
  assertEqual(await store.getLastReceivedSeq("conv-1"), 0, "clear removes cursor");

  console.log("local message store contract ok");
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
