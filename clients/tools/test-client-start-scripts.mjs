import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import assert from "node:assert/strict";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));

const backendScript = readFileSync(join(clientsRoot, "start-local-backend.ps1"), "utf8");
const webScript = readFileSync(join(clientsRoot, "start-local-web.ps1"), "utf8");

assert(backendScript.includes("run-local-dev.ps1"), "backend script must delegate to clientweb local backend runner");
assert(backendScript.includes("BindHost"), "backend script must expose BindHost");
assert(backendScript.includes("ClientHost"), "backend script must expose ClientHost");

assert(webScript.includes("VITE_NEXUSIM_API_BASE"), "web script must set API endpoint");
assert(webScript.includes("VITE_NEXUSIM_WS_URL"), "web script must set WebSocket endpoint");
assert(webScript.includes("npm --prefix"), "web script must start the shared clients workspace dev server");
assert(!webScript.includes("run-local-dev.ps1"), "web script must not hide backend startup behind Web launch");

console.log("client start scripts contract ok");
