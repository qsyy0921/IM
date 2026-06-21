import { createHash, randomUUID } from "node:crypto";
import { createServer } from "node:net";
import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.desktop-webview-login-smoke.v1";
const toolsDir = dirname(fileURLToPath(import.meta.url));
const buildDesktopArtifactScript = join(toolsDir, "build-desktop-artifact.mjs");
const desktopArtifactPath = join(workspaceRoot, "desktop", "src-tauri", "target", "release", "nexusim-desktop.exe");

async function main(argv) {
  const options = parseArgs(argv);
  const runID = options.runID || `desktop-webview-login-${randomUUID()}`;
  const fixture = options.fixturePath ? readFixture(options.fixturePath) : undefined;
  const plan = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    dryRun: options.dryRun,
    runID,
    input: {
      source: options.fixturePath ? safeHint(options.fixturePath) : "not-provided",
      authInputRequired: true,
      externalMessageTrigger: Boolean(fixture?.senderUserID && fixture?.senderAuthProof)
    },
    build: {
      command: "npm --prefix clients run smoke:desktop-webview-login",
      shellConfig: "temporary-desktop-login",
      artifactHint: safeHint(desktopArtifactPath)
    },
    automation: {
      driver: "webview2-cdp",
      uiSelectorContract: "clients/web/src/App.tsx data-testid",
      requiredSelectors: [
        "login-submit",
        "native-store-readiness",
        "runtime-status",
        "push-status",
        "ack-status"
      ],
      lowSensitiveOutput: true
    },
    verdict: {
      loginLevelDesktopUISmoke: false,
      deliveryNotifyInWebView: false,
      pullInboxInWebView: false,
      ackDeliveryInWebView: false
    }
  };

  if (options.dryRun) {
    assertLowSensitive(plan);
    emitResult(plan, options);
    return;
  }

  if (process.platform !== "win32") {
    throw new Error("desktop WebView login smoke is supported on Windows only");
  }
  if (!fixture) {
    throw new Error("--fixture is required unless --dry-run is used");
  }
  if (!fixture.senderUserID || !fixture.senderAuthProof) {
    throw new Error("fixture must include senderUserID and senderAuthProof for delivery.notify verification");
  }

  const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-webview-login-"));
  let child;
  let cdp;
  try {
    const debugPort = await getFreePort();
    const shellConfigPath = join(tempRoot, "shell-config.json");
    writeShellConfig(shellConfigPath, fixture);
    buildDesktopArtifact(shellConfigPath, options);
    validateArtifactExists();
    child = launchDesktopArtifact(debugPort);
    cdp = await connectWebViewCDP(debugPort, options.holdMs);
    await driveLogin(cdp, fixture, options);
    const nativeStoreReadiness = await waitForNativeStoreReadiness(cdp, options.holdMs);
    const sent = await triggerExternalSenderMessage(fixture, runID);
    await waitForWebViewMessage(cdp, sent.text, sent.conversationSeq, options.holdMs);
    const ack = await waitForAck(cdp, sent.conversationSeq, options.holdMs);
    const terminated = terminateProcess(child.pid);
    await sleep(500);

    const result = {
      ...plan,
      dryRun: false,
      build: {
        ...plan.build,
        artifactReady: true
      },
      automation: {
        ...plan.automation,
        debugTargetFound: true
      },
      flow: {
        loginOK: true,
        pushConnected: true,
        openedConversation: true,
        nativeStoreReadinessDisplayed: true,
        nativeStoreReadiness,
        sentConversationSeq: sent.conversationSeq,
        observedMessage: true,
        ackSeq: ack.seq
      },
      process: {
        started: true,
        terminated
      },
      verdict: {
        loginLevelDesktopUISmoke: true,
        deliveryNotifyInWebView: true,
        pullInboxInWebView: true,
        ackDeliveryInWebView: ack.seq >= sent.conversationSeq
      },
      caveats: [
        "This smoke drives the rendered Tauri WebView through the public Web UI using WebView2 remote debugging.",
        "It uses local smoke-only auth input from an external fixture file and never writes that input into shell config or output.",
        "It assumes the clientweb local stack has already created the tenant, users, membership and public BFF/push endpoints."
      ]
    };
    assertLowSensitive(result);
    emitResult(result, options);
  } finally {
    if (cdp) {
      cdp.close();
    }
    if (child?.pid) {
      terminateProcess(child.pid);
    }
    rmSync(tempRoot, { recursive: true, force: true });
  }
}

function parseArgs(argv) {
  const options = {
    dryRun: false,
    fixturePath: "",
    holdMs: 60000,
    runID: "",
    outputPath: "",
    skipWebBuild: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--dry-run") {
      options.dryRun = true;
      continue;
    }
    if (arg === "--fixture") {
      options.fixturePath = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--hold-ms") {
      const value = Number.parseInt(requiredValue(argv, index, arg), 10);
      if (!Number.isInteger(value) || value < 10000 || value > 120000) {
        throw new Error("--hold-ms must be between 10000 and 120000");
      }
      options.holdMs = value;
      index += 1;
      continue;
    }
    if (arg === "--run-id") {
      options.runID = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--output") {
      options.outputPath = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--skip-web-build") {
      options.skipWebBuild = true;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return options;
}

function emitResult(result, options) {
  const payload = `${JSON.stringify(result, null, 2)}\n`;
  if (options.outputPath) {
    writeFileSync(options.outputPath, payload, "utf8");
    return;
  }
  process.stdout.write(payload);
}

function readFixture(path) {
  const raw = readFileSync(path, "utf8");
  const parsed = JSON.parse(raw);
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error("fixture must be a JSON object");
  }
  const fixture = {
    apiBaseURL: requiredString(parsed.apiBaseURL, "apiBaseURL"),
    pushWebSocketURL: requiredString(parsed.pushWebSocketURL, "pushWebSocketURL"),
    tenantID: requiredString(parsed.tenantID, "tenantID"),
    userID: requiredString(parsed.userID, "userID"),
    authProof: requiredString(parsed.authProof, "authProof"),
    deviceID: optionalString(parsed.deviceID) || "desktop-webview-login-device",
    conversationID: requiredString(parsed.conversationID, "conversationID"),
    senderUserID: optionalString(parsed.senderUserID),
    senderAuthProof: optionalString(parsed.senderAuthProof),
    senderDeviceID: optionalString(parsed.senderDeviceID) || "desktop-webview-login-sender",
    messageText: optionalString(parsed.messageText) || `NexusIM desktop WebView smoke ${Date.now()}`
  };
  assertURL(fixture.apiBaseURL, ["http:", "https:"], "apiBaseURL");
  assertURL(fixture.pushWebSocketURL, ["ws:", "wss:"], "pushWebSocketURL");
  return fixture;
}

function writeShellConfig(path, fixture) {
  writeFileSync(path, `${JSON.stringify({
    target: "windows-desktop",
    apiBaseURL: fixture.apiBaseURL,
    pushWebSocketURL: fixture.pushWebSocketURL,
    deviceID: fixture.deviceID,
    installationID: `desktop-webview-login-${sha256Text(fixture.tenantID).slice(0, 12)}`,
    appVersion: "0.1.0-smoke",
    sessionKey: `desktop-webview-login-${sha256Text(fixture.userID).slice(0, 12)}`
  }, null, 2)}\n`, "utf8");
}

function buildDesktopArtifact(shellConfigPath, options) {
  const args = [buildDesktopArtifactScript, "--shell-config", shellConfigPath];
  if (options.skipWebBuild) {
    args.push("--skip-web-build");
  }
  const completed = spawnSync(process.execPath, args, {
    cwd: resolve(workspaceRoot, ".."),
    encoding: "utf8",
    stdio: "inherit",
    windowsHide: true
  });
  if (completed.status !== 0) {
    throw new Error(`desktop artifact build failed with exit code ${completed.status ?? "unknown"}`);
  }
}

function validateArtifactExists() {
  if (!existsSync(desktopArtifactPath)) {
    throw new Error("desktop artifact was not built");
  }
}

function launchDesktopArtifact(debugPort) {
  const existingArgs = process.env.WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS ?? "";
  const remoteDebugArg = `--remote-debugging-port=${debugPort}`;
  const child = spawn(desktopArtifactPath, [], {
    cwd: dirname(desktopArtifactPath),
    windowsHide: true,
    stdio: "ignore",
    env: {
      ...process.env,
      WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS: [existingArgs, remoteDebugArg].filter(Boolean).join(" ")
    }
  });
  child.unref();
  return child;
}

async function connectWebViewCDP(port, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const targets = await fetchJSON(`http://127.0.0.1:${port}/json`);
      const page = Array.isArray(targets)
        ? targets.find(target => target.type === "page" && typeof target.webSocketDebuggerUrl === "string")
        : undefined;
      if (page) {
        const client = await CDPClient.connect(page.webSocketDebuggerUrl);
        await client.send("Runtime.enable");
        return client;
      }
    } catch (error) {
      lastError = error;
    }
    await sleep(300);
  }
  throw new Error(`timed out waiting for WebView2 debug target${lastError ? `: ${errorMessage(lastError)}` : ""}`);
}

async function driveLogin(cdp, fixture, options) {
  await waitForSelector(cdp, "login-submit", options.holdMs);
  await setInput(cdp, "login-tenant", fixture.tenantID);
  await setInput(cdp, "login-user", fixture.userID);
  await setInput(cdp, "login-password", fixture.authProof);
  await click(cdp, "login-submit");
  await waitForText(cdp, "runtime-status", value => value === "login ok", "login ok", options.holdMs);
  await waitForText(cdp, "push-status", value => value.includes("connected"), "push connected", options.holdMs);
  await setInput(cdp, "conversation-id-input", fixture.conversationID);
  await click(cdp, "open-conversation");
  await waitForText(cdp, "runtime-status", value => value === "open conversation ok", "open conversation ok", options.holdMs);
}

async function waitForNativeStoreReadiness(cdp, timeoutMs) {
  return waitForEval(cdp, value => {
    const text = String(value?.text ?? "");
    const ok = text.includes("local-storage -> sqlite") &&
      text.includes("sqlite-native-bridge-unavailable") &&
      text.includes("tauri-sqlite");
    return ok
      ? {
          ok: true,
          currentDefault: "local-storage",
          productionTarget: "sqlite",
          nativeStoreReady: false,
          nativeStoreReason: "sqlite-native-bridge-unavailable",
          nativeStoreBridge: "tauri-sqlite"
        }
      : { ok: false };
  }, {
    label: "native store readiness in WebView",
    timeoutMs,
    expression: `(() => {
      const text = document.querySelector('[data-testid="native-store-readiness"]')?.textContent || "";
      return { ok: false, text };
    })()`
  });
}

async function triggerExternalSenderMessage(fixture, runID) {
  const sender = await bffJSON(`${fixture.apiBaseURL}/api/auth/login`, {
    method: "POST",
    body: {
      tenant_id: fixture.tenantID,
      user_id: fixture.senderUserID,
      password: fixture.senderAuthProof,
      device_id: fixture.senderDeviceID,
      audience: "api-gateway",
      trace_id: "desktop-webview-login-smoke",
      request_id: `desktop-webview-login-${runID}`
    }
  });
  const gatewayToken = stringField(sender.gateway_token, "gateway_token");
  const message = await bffJSON(`${fixture.apiBaseURL}/api/messages/send`, {
    method: "POST",
    bearer: gatewayToken,
    body: {
      conversation_id: fixture.conversationID,
      client_msg_id: `desktop-webview-login-${runID}-${Date.now()}`,
      message_type: "TEXT",
      payload: { text: fixture.messageText },
      attachment_ids: []
    }
  });
  return {
    conversationSeq: numberField(message.conversation_seq, "conversation_seq"),
    text: fixture.messageText
  };
}

async function waitForWebViewMessage(cdp, expectedText, minSeq, timeoutMs) {
  await waitForEval(cdp, () => "", {
    label: "message visible",
    timeoutMs,
    expression: `(() => {
      const list = document.querySelector('[data-testid="message-list"]');
      const text = list?.textContent || "";
      const items = Array.from(document.querySelectorAll('[data-testid="message-item"]')).map(item => item.textContent || "");
      const seqVisible = items.some(item => item.includes("#${minSeq}"));
      return { ok: text.includes(${JSON.stringify(expectedText)}) && seqVisible, text };
    })()`
  });
}

async function waitForAck(cdp, minSeq, timeoutMs) {
  return waitForEval(cdp, value => {
    const match = String(value?.text ?? "").match(/#(\d+)/);
    const seq = match ? Number.parseInt(match[1], 10) : 0;
    return Number.isInteger(seq) && seq >= minSeq ? { ok: true, seq } : { ok: false };
  }, {
    label: "AckDelivery in WebView",
    timeoutMs,
    expression: `(() => {
      const text = document.querySelector('[data-testid="ack-status"]')?.textContent || "";
      return { ok: false, text };
    })()`
  });
}

async function waitForSelector(cdp, testID, timeoutMs) {
  await waitForEval(cdp, () => "", {
    label: `selector ${testID}`,
    timeoutMs,
    expression: `(() => ({ ok: Boolean(document.querySelector('[data-testid="${testID}"]')) }))()`
  });
}

async function setInput(cdp, testID, value) {
  await cdp.evaluate(`(() => {
    const element = document.querySelector('[data-testid="${testID}"]');
    if (!element) {
      throw new Error('missing input ${testID}');
    }
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
    setter.call(element, ${JSON.stringify(value)});
    element.dispatchEvent(new Event('input', { bubbles: true }));
    return true;
  })()`);
}

async function click(cdp, testID) {
  await cdp.evaluate(`(() => {
    const element = document.querySelector('[data-testid="${testID}"]');
    if (!element) {
      throw new Error('missing button ${testID}');
    }
    if (element.disabled) {
      throw new Error('button disabled ${testID}');
    }
    element.click();
    return true;
  })()`);
}

async function waitForText(cdp, testID, predicate, label, timeoutMs) {
  await waitForEval(cdp, value => {
    const text = String(value?.text ?? "");
    return predicate(text) ? { ok: true, text } : { ok: false };
  }, {
    label,
    timeoutMs,
    expression: `(() => {
      const text = document.querySelector('[data-testid="${testID}"]')?.textContent || "";
      return { ok: false, text };
    })()`
  });
}

async function waitForEval(cdp, success, options) {
  const deadline = Date.now() + options.timeoutMs;
  let lastValue;
  while (Date.now() < deadline) {
    const value = await cdp.evaluate(options.expression);
    lastValue = value;
    const result = value?.ok === true ? { ok: true, ...value } : success(value);
    if (result?.ok) {
      return result;
    }
    await sleep(300);
  }
  const diagnostics = await pageDiagnostics(cdp);
  throw new Error(`timed out waiting for ${options.label}: ${JSON.stringify({ lastValue, diagnostics })}`);
}

async function bffJSON(url, input) {
  const headers = {
    accept: "application/json"
  };
  if (input.body !== undefined) {
    headers["content-type"] = "application/json";
  }
  if (input.bearer) {
    headers.authorization = `Bearer ${input.bearer}`;
  }
  const response = await fetch(url, {
    method: input.method,
    headers,
    body: input.body === undefined ? undefined : JSON.stringify(input.body)
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`BFF ${input.method} ${new URL(url).pathname} returned ${response.status}`);
  }
  return text.trim() === "" ? {} : JSON.parse(text);
}

class CDPClient {
  constructor(socket) {
    this.socket = socket;
    this.nextID = 1;
    this.pending = new Map();
    socket.addEventListener("message", event => this.onMessage(event.data));
    socket.addEventListener("error", () => this.rejectAll(new Error("WebView2 debug socket error")));
    socket.addEventListener("close", () => this.rejectAll(new Error("WebView2 debug socket closed")));
  }

  static connect(url) {
    if (typeof WebSocket !== "function") {
      throw new Error("Node.js WebSocket is required for desktop WebView smoke");
    }
    const socket = new WebSocket(url);
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        cleanup();
        reject(new Error("timed out connecting to WebView2 debug socket"));
      }, 10000);
      const cleanup = () => {
        clearTimeout(timeout);
        socket.removeEventListener("open", onOpen);
        socket.removeEventListener("error", onError);
      };
      const onOpen = () => {
        cleanup();
        resolve(new CDPClient(socket));
      };
      const onError = () => {
        cleanup();
        reject(new Error("failed connecting to WebView2 debug socket"));
      };
      socket.addEventListener("open", onOpen);
      socket.addEventListener("error", onError);
    });
  }

  send(method, params = {}) {
    const id = this.nextID++;
    const payload = JSON.stringify({ id, method, params });
    const response = new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
    });
    this.socket.send(payload);
    return response;
  }

  async evaluate(expression) {
    const response = await this.send("Runtime.evaluate", {
      expression,
      awaitPromise: true,
      returnByValue: true
    });
    if (response.exceptionDetails) {
      throw new Error(response.exceptionDetails.text || "WebView evaluation failed");
    }
    return response.result?.value;
  }

  onMessage(data) {
    const message = JSON.parse(String(data));
    if (!message.id) {
      return;
    }
    const pending = this.pending.get(message.id);
    if (!pending) {
      return;
    }
    this.pending.delete(message.id);
    if (message.error) {
      pending.reject(new Error(message.error.message || "WebView2 debug command failed"));
      return;
    }
    pending.resolve(message.result ?? {});
  }

  rejectAll(error) {
    for (const pending of this.pending.values()) {
      pending.reject(error);
    }
    this.pending.clear();
  }

  close() {
    try {
      this.socket.close();
    } catch {
      // best effort
    }
  }
}

async function fetchJSON(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`GET ${url} returned ${response.status}`);
  }
  return response.json();
}

async function getFreePort() {
  const server = createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  await new Promise(resolve => server.close(resolve));
  if (!address || typeof address === "string") {
    throw new Error("failed to allocate TCP port");
  }
  return address.port;
}

function terminateProcess(pid) {
  if (!pid) {
    return false;
  }
  if (process.platform === "win32") {
    const completed = spawnSync("taskkill", ["/PID", String(pid), "/T", "/F"], {
      encoding: "utf8",
      windowsHide: true
    });
    return completed.status === 0;
  }
  try {
    process.kill(pid, "SIGTERM");
    return true;
  } catch {
    return false;
  }
}

function requiredString(value, name) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`fixture ${name} is required`);
  }
  return value;
}

function optionalString(value) {
  return typeof value === "string" && value.trim() !== "" ? value : "";
}

function assertURL(value, protocols, name) {
  let parsed;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`fixture ${name} must be a valid URL`);
  }
  if (!protocols.includes(parsed.protocol)) {
    throw new Error(`fixture ${name} has unsupported protocol`);
  }
}

function stringField(value, name) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`BFF response ${name} missing`);
  }
  return value;
}

function numberField(value, name) {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number.parseInt(value, 10);
    if (Number.isInteger(parsed)) {
      return parsed;
    }
  }
  throw new Error(`BFF response ${name} missing`);
}

function safeHint(path) {
  const fullPath = resolve(path);
  const relativePath = relative(resolve(workspaceRoot, ".."), fullPath).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(fullPath).slice(0, 12)}`;
  }
  return relativePath;
}

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("desktop WebView login smoke leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop WebView login smoke leaked a sensitive field name");
  }
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
}

function requiredValue(argv, index, name) {
  const value = argv[index + 1];
  if (!value || value.startsWith("--")) {
    throw new Error(`${name} requires a value`);
  }
  return value;
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

async function pageDiagnostics(cdp) {
  try {
    return await cdp.evaluate(`(() => ({
      url: location.href,
      title: document.title,
      runtimeStatus: document.querySelector('[data-testid="runtime-status"]')?.textContent || "",
      pushStatus: document.querySelector('[data-testid="push-status"]')?.textContent || "",
      nativeStoreReadiness: document.querySelector('[data-testid="native-store-readiness"]')?.textContent || "",
      error: document.querySelector('[data-testid="error-banner"]')?.textContent || "",
      bodyTextPrefix: (document.body?.textContent || "").slice(0, 300)
    }))()`);
  } catch (error) {
    return { error: errorMessage(error) };
  }
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    await main(process.argv.slice(2));
  } catch (error) {
    console.error(errorMessage(error));
    process.exitCode = 2;
  }
}
