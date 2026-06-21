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
  "conversation-id-input",
  "open-conversation",
  "refresh-conversations",
  "conversation-item",
  "runtime-status",
  "push-status",
  "ack-status",
  "native-store-readiness",
  "error-banner",
  "message-list",
  "message-item",
  "message-composer",
  "send-message"
];

for (const testID of requiredTestIDs) {
  assertIncludes(appSource, `data-testid="${testID}"`, `web shell automation selector missing: ${testID}`);
}

assertIncludes(appSource, "setLastAck({ conversationID, seq: maxSeq })", "web shell must expose AckDelivery progress after sync");
assertIncludes(appSource, "runtime.ackQueue.flush(currentSession)", "web shell must keep AckDelivery inside shared runtime path");
assertIncludes(appSource, "nativeMetadata?.capabilities?.localStore", "web shell must display native local-store readiness when available");
assertIncludes(appSource, "nativeLocalStoreStatus", "web shell must keep local-store readiness formatting explicit");

console.log("web shell automation contract ok");

function assertIncludes(source, expected, message) {
  if (!source.includes(expected)) {
    throw new Error(`${message}: missing ${expected}`);
  }
}
