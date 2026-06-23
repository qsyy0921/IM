import { readFileSync } from "node:fs";
import { Buffer } from "node:buffer";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import ts from "typescript";

const root = fileURLToPath(new URL("..", import.meta.url));

async function main() {
  const sourcePath = join(root, "packages/client-core/src/send-queue.ts");
  const source = readFileSync(sourcePath, "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022
    }
  }).outputText;

  const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`;
  const { MessageSendQueue } = await import(moduleURL);

  const calls = [];
  const store = {
    async markPending(message) {
      calls.push(`store:${message.status}:${message.clientMessageID}`);
    },
    async markSendAccepted(localID, response) {
      calls.push(`accepted:${localID}:${response.conversationSeq}`);
    },
    async markSendFailed(localID) {
      calls.push(`failed:${localID}`);
    }
  };
  const api = {
    async sendMessage(request) {
      calls.push(`api:${request.clientMessageID}`);
      return {
        messageID: "msg-1",
        conversationID: request.conversationID,
        conversationSeq: 7
      };
    }
  };
  const queue = new MessageSendQueue({
    messagingAPI: api,
    store,
    idempotencyKeyFactory: () => "idem-1",
    clientMessageIDFactory: () => "local-1",
    nowMs: () => 123
  });

  const sent = await queue.sendText({
    session: {
      tenantID: "tenant-1",
      userID: "user-a",
      deviceID: "device-a",
      accessToken: "access",
      refreshToken: "refresh",
      expiresAtMs: 9999999999999
    },
    conversationID: "conv-1",
    text: "hello",
    onPendingStored: message => {
      calls.push(`pending-hook:${message.status}:${message.clientMessageID}`);
    }
  });

  assertDeepEqual(
    calls,
    ["store:PENDING:local-1", "pending-hook:PENDING:local-1", "api:local-1", "accepted:local-1:7"],
    "pending hook must run after store write and before network send"
  );
  assertEqual(sent.status, "SENT", "send queue returns accepted message status");
  assertEqual(sent.conversationSeq, 7, "send queue returns accepted seq");

  console.log("message send queue pending visibility ok");
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
