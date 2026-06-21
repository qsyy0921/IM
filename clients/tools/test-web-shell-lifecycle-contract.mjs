import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

const root = fileURLToPath(new URL("..", import.meta.url));
const appSource = readFileSync(join(root, "web/src/App.tsx"), "utf8");

assertIncludes(
  appSource,
  "createClientShellActions(runtime)",
  "web shell must construct shared ClientShellActions"
);

for (const action of ["login", "refresh", "restoreSession", "logout"]) {
  assertIncludes(
    appSource,
    `shellActions.${action}(`,
    `web shell must call shared shellActions.${action}`
  );
  assertNotIncludes(
    appSource,
    `runtime.${action}(`,
    `web shell must not bypass shared shellActions.${action}`
  );
}

assertIncludes(
  appSource,
  "刷新登录态",
  "web shell must expose a user-visible auth refresh action"
);

console.log("web shell lifecycle contract ok");

function assertIncludes(source, expected, message) {
  if (!source.includes(expected)) {
    throw new Error(`${message}: missing ${expected}`);
  }
}

function assertNotIncludes(source, unexpected, message) {
  if (source.includes(unexpected)) {
    throw new Error(`${message}: found ${unexpected}`);
  }
}
