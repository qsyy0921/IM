import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const root = fileURLToPath(new URL("..", import.meta.url));
const appSource = readFileSync(join(root, "web/src/App.tsx"), "utf8");

const clearSessionBlock = functionBlock(appSource, "clearSessionViewState");
for (const expected of [
  "pushConnectionRef.current?.close();",
  "pushConnectionRef.current = null;",
  "sessionRef.current = null;",
  "activeConversationRef.current = \"\";",
  "setSession(null);",
  "setConversations([]);",
  "setContacts([]);",
  "setIncomingRequests([]);",
  "setOutgoingRequests([]);",
  "setContactsError(\"\");",
  "setActiveConversationID(\"\");",
  "setMessages([]);",
  "clearGroupMemberState();",
  "clearGroupProfileState();",
  "clearConversationManagementState();",
  "setGroupSettingsTab(\"profile\");",
  "setLastAck(null);",
  "setPushStatus(\"disconnected\");"
]) {
  assertIncludes(clearSessionBlock, expected, `clearSessionViewState must clear ${expected}`);
}

const expiredBlock = asyncFunctionBlock(appSource, "clearExpiredSession");
for (const expected of [
  "await shellActions.logout().catch(() => undefined);",
  "clearSessionViewState();",
  "setComposerText(\"\");",
  "setStatus(\"login expired\");",
  "setError(`登录态已过期，请重新登录。${errorMessage(caught)}`);"
]) {
  assertIncludes(expiredBlock, expected, `clearExpiredSession must run ${expected}`);
}

assertIncludes(
  appSource,
  "await clearExpiredSession(caught);\n        setStatus(\"restore session failed\");",
  "auto restore failure must reuse expired-session cleanup before reporting restore failure"
);
assertIncludes(
  appSource,
  "if (isUnauthenticated(caught) && sessionRef.current) {\n        await clearExpiredSession(caught);\n        return;\n      }",
  "authenticated task failures must clear expired session state"
);
assertIncludes(
  appSource,
  "await store.clear();",
  "explicit logout must still clear local message cache"
);

console.log("web shell expired session cleanup contract ok");

function functionBlock(source, name) {
  return namedFunctionBlock(source, `function ${name}`);
}

function asyncFunctionBlock(source, name) {
  return namedFunctionBlock(source, `async function ${name}`);
}

function namedFunctionBlock(source, signature) {
  const start = source.indexOf(signature);
  if (start < 0) {
    throw new Error(`missing ${signature}`);
  }
  const bodyStart = source.indexOf("{", start);
  if (bodyStart < 0) {
    throw new Error(`missing body for ${signature}`);
  }
  let depth = 0;
  for (let index = bodyStart; index < source.length; index += 1) {
    const char = source[index];
    if (char === "{") {
      depth += 1;
    }
    if (char === "}") {
      depth -= 1;
      if (depth === 0) {
        return source.slice(bodyStart, index + 1);
      }
    }
  }
  throw new Error(`unterminated body for ${signature}`);
}

function assertIncludes(source, expected, message) {
  if (!source.includes(expected)) {
    throw new Error(`${message}: missing ${expected}`);
  }
}
