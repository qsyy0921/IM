import { createHash, randomUUID } from "node:crypto";
import { createServer } from "node:net";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { buildClientArtifactInstallPlan } from "./plan-client-artifact-install.mjs";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.android-webview-login-smoke.v1";
const toolsDir = dirname(fileURLToPath(import.meta.url));
const buildAndroidApkScript = join(toolsDir, "build-android-apk.mjs");
const packageName = "com.nexusim.android";
const mainActivity = "com.nexusim.android/.MainActivity";

async function main(argv) {
  const options = parseArgs(argv);
  const runID = options.runID || `android-webview-login-${randomUUID()}`;
  const fixture = options.fixturePath ? readFixture(options.fixturePath) : undefined;
  const plan = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    dryRun: options.dryRun,
    runID,
    input: {
      source: options.fixturePath ? "provided" : "not-provided",
      loginInputRequired: true,
      externalMessageTrigger: Boolean(fixture?.senderUserID && fixture?.senderAuthProof)
    },
    build: {
      command: "npm --prefix clients run smoke:android-webview-login",
      shellConfig: "temporary-android-login",
      freshBuildRequired: true,
      artifactHint: "clients/artifacts/<run-id>/nexusim-android-debug.apk"
    },
    adb: {
      packageName,
      mainActivity,
      installRequired: true,
      webviewDevtoolsForwardRequired: true
    },
    automation: {
      driver: "android-webview-cdp-via-adb-forward",
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
      loginLevelAndroidUISmoke: false,
      deliveryNotifyInWebView: false,
      pullInboxInWebView: false,
      ackDeliveryInWebView: false
    }
  };

  if (options.dryRun) {
    const dryRunPlan = {
      ...plan,
      executionPolicy: dryRunExecutionPolicy()
    };
    assertLowSensitive(dryRunPlan);
    emitResult(dryRunPlan, options);
    return;
  }

  if (!fixture) {
    throw new Error("--fixture is required unless --dry-run is used");
  }
  if (!fixture.senderUserID || !fixture.senderAuthProof) {
    throw new Error("fixture must include senderUserID and senderAuthProof for delivery.notify verification");
  }

  const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-android-webview-login-"));
  let cdp;
  let forwardedPort = 0;
  try {
    const shellConfigPath = join(tempRoot, "shell-config.json");
    writeShellConfig(shellConfigPath, fixture);
    buildAndroidApk(shellConfigPath, runID, options);
    const manifestPath = join(workspaceRoot, "artifacts", runID, "manifest.json");
    const installPlan = buildClientArtifactInstallPlan({ manifest: manifestPath });
    const androidPlan = installPlan.targets.android;
    if (!androidPlan?.artifactReady || !androidPlan.readyForInstall) {
      throw new Error("Android APK artifact or adb install prerequisites are not ready");
    }
    const apkPath = artifactPathFromManifest(manifestPath);

    installAndLaunch(apkPath);
    const devtoolsSocket = await waitForWebViewDevtoolsSocket(options.holdMs);
    forwardedPort = await getFreePort();
    forwardWebViewDevtools(forwardedPort, devtoolsSocket);
    cdp = await connectWebViewCDP(forwardedPort, options.holdMs);
    await driveLogin(cdp, fixture, options);
    const nativeStoreReadiness = await waitForNativeStoreReadiness(cdp, options.holdMs);
    const sent = await triggerExternalSenderMessage(fixture, runID);
    await waitForWebViewMessage(cdp, sent.text, sent.conversationSeq, options.holdMs);
    const ack = await waitForAck(cdp, sent.conversationSeq, options.holdMs);

    const result = {
      ...plan,
      dryRun: false,
      build: {
        ...plan.build,
        artifactReady: true
      },
      adb: {
        ...plan.adb,
        installed: true,
        launched: true,
        devtoolsForwarded: true
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
      verdict: {
        loginLevelAndroidUISmoke: true,
        deliveryNotifyInWebView: true,
        pullInboxInWebView: true,
        ackDeliveryInWebView: ack.seq >= sent.conversationSeq
      },
      caveats: [
        "This smoke drives a debuggable Android WebView through the public Web UI through CDP over adb forwarding.",
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
    if (forwardedPort > 0) {
      removeAdbForward(forwardedPort);
    }
    rmSync(tempRoot, { recursive: true, force: true });
  }
}

function dryRunExecutionPolicy() {
  return {
    planOnly: true,
    executesPlannedCommands: false,
    buildsAPK: false,
    collectsArtifacts: false,
    installsAPK: false,
    startsActivity: false,
    opensAdbForward: false,
    contactsDevice: false,
    usesWebViewAutomation: false,
    contactsBFF: false,
    sendsMessages: false,
    opensNetworkConnection: false,
    downloadsToolchain: false
  };
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
    deviceID: optionalString(parsed.deviceID) || "android-webview-login-device",
    conversationID: requiredString(parsed.conversationID, "conversationID"),
    senderUserID: optionalString(parsed.senderUserID),
    senderAuthProof: optionalString(parsed.senderAuthProof),
    senderDeviceID: optionalString(parsed.senderDeviceID) || "android-webview-login-sender",
    messageText: optionalString(parsed.messageText) || `NexusIM Android WebView smoke ${Date.now()}`
  };
  assertURL(fixture.apiBaseURL, ["http:", "https:"], "apiBaseURL");
  assertURL(fixture.pushWebSocketURL, ["ws:", "wss:"], "pushWebSocketURL");
  return fixture;
}

function writeShellConfig(path, fixture) {
  writeFileSync(path, `${JSON.stringify({
    target: "android",
    apiBaseURL: fixture.apiBaseURL,
    pushWebSocketURL: fixture.pushWebSocketURL,
    deviceID: fixture.deviceID,
    installationID: `android-webview-login-${sha256Text(fixture.tenantID).slice(0, 12)}`,
    appVersion: "0.1.0-smoke",
    sessionKey: `android-webview-login-${sha256Text(fixture.userID).slice(0, 12)}`
  }, null, 2)}\n`, "utf8");
}

function buildAndroidApk(shellConfigPath, runID, options) {
  const args = [
    buildAndroidApkScript,
    "--shell-config",
    shellConfigPath,
    "--collect",
    "--run-id",
    runID
  ];
  if (options.skipWebBuild) {
    args.push("--skip-web-build");
  }
  const completed = spawnSync(process.execPath, args, {
    cwd: resolve(workspaceRoot, ".."),
    encoding: "utf8",
    stdio: "inherit",
    windowsHide: true,
    env: process.env
  });
  if (completed.status !== 0) {
    throw new Error(`Android APK build failed with exit code ${completed.status ?? "unknown"}`);
  }
}

function artifactPathFromManifest(manifestPath) {
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  const artifact = manifest.artifacts?.find(entry => entry.target === "android");
  if (!artifact?.filename) {
    throw new Error("Android APK artifact missing from collector manifest");
  }
  return join(dirname(manifestPath), artifact.filename);
}

function installAndLaunch(apkPath) {
  runAdb(["install", "-r", apkPath], "adb install failed");
  runAdb(["shell", "am", "force-stop", packageName], "adb force-stop failed");
  runAdb(["shell", "am", "start", "-n", mainActivity], "adb activity launch failed");
}

async function waitForWebViewDevtoolsSocket(timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let lastOutput = "";
  while (Date.now() < deadline) {
    const output = runAdbCapture(["shell", "cat", "/proc/net/unix"]);
    lastOutput = output;
    const socket = parseWebViewDevtoolsSocket(output);
    if (socket) {
      return socket;
    }
    await sleep(300);
  }
  throw new Error(`timed out waiting for Android WebView devtools socket${lastOutput ? ": no webview_devtools_remote entry" : ""}`);
}

export function parseWebViewDevtoolsSocket(output) {
  return parseWebViewDevtoolsSockets(output)[0] ?? "";
}

export function parseWebViewDevtoolsSockets(output) {
  return output
    .split(/\r?\n/)
    .map(line => {
      const match = line.match(/@?(webview_devtools_remote[^\s]*)/);
      return match?.[1] ?? "";
    })
    .filter(Boolean);
}

function forwardWebViewDevtools(port, socketName) {
  runAdb(["forward", `tcp:${port}`, `localabstract:${socketName}`], "adb forward WebView devtools failed");
}

function removeAdbForward(port) {
  spawnSync("adb", ["forward", "--remove", `tcp:${port}`], {
    stdio: "ignore",
    windowsHide: true
  });
}

function runAdb(args, failureMessage) {
  const completed = spawnSync("adb", args, {
    encoding: "utf8",
    stdio: "inherit",
    windowsHide: true
  });
  if (completed.status !== 0) {
    throw new Error(failureMessage);
  }
}

function runAdbCapture(args) {
  const completed = spawnSync("adb", args, {
    encoding: "utf8",
    windowsHide: true
  });
  if (completed.status !== 0) {
    return "";
  }
  return completed.stdout || completed.stderr || "";
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
  throw new Error(`timed out waiting for Android WebView debug target${lastError ? `: ${errorMessage(lastError)}` : ""}`);
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
    return parseAndroidNativeStoreReadinessText(String(value?.text ?? ""));
  }, {
    label: "native store readiness in Android WebView",
    timeoutMs,
    expression: `(() => {
      const text = document.querySelector('[data-testid="native-store-readiness"]')?.textContent || "";
      return { ok: false, text };
    })()`
  });
}

export function parseAndroidNativeStoreReadinessText(text) {
  const value = String(text ?? "");
  const ok = value.includes("local-storage -> sqlite") &&
    value.includes("; ready;") &&
    value.includes("android-sqlite");
  return ok
    ? {
        ok: true,
        currentDefault: "local-storage",
        productionTarget: "sqlite",
        nativeStoreReady: true,
        nativeStoreReason: "",
        nativeStoreBridge: "android-sqlite"
      }
    : { ok: false };
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
      trace_id: "android-webview-login-smoke",
      request_id: `android-webview-login-${runID}`
    }
  });
  const gatewayAuth = stringField(sender.gateway_token, "gateway auth");
  const message = await bffJSON(`${fixture.apiBaseURL}/api/messages/send`, {
    method: "POST",
    bearer: gatewayAuth,
    body: {
      conversation_id: fixture.conversationID,
      client_msg_id: `android-webview-login-${runID}-${Date.now()}`,
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
    label: "AckDelivery in Android WebView",
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
    socket.addEventListener("error", () => this.rejectAll(new Error("Android WebView debug socket error")));
    socket.addEventListener("close", () => this.rejectAll(new Error("Android WebView debug socket closed")));
  }

  static connect(url) {
    if (typeof WebSocket !== "function") {
      throw new Error("Node.js WebSocket is required for Android WebView smoke");
    }
    const socket = new WebSocket(url);
    return new Promise((resolvePromise, rejectPromise) => {
      const timeout = setTimeout(() => {
        cleanup();
        rejectPromise(new Error("timed out connecting to Android WebView debug socket"));
      }, 10000);
      const cleanup = () => {
        clearTimeout(timeout);
        socket.removeEventListener("open", onOpen);
        socket.removeEventListener("error", onError);
      };
      const onOpen = () => {
        cleanup();
        resolvePromise(new CDPClient(socket));
      };
      const onError = () => {
        cleanup();
        rejectPromise(new Error("failed connecting to Android WebView debug socket"));
      };
      socket.addEventListener("open", onOpen);
      socket.addEventListener("error", onError);
    });
  }

  send(method, params = {}) {
    const id = this.nextID++;
    const payload = JSON.stringify({ id, method, params });
    const response = new Promise((resolvePromise, rejectPromise) => {
      this.pending.set(id, { resolve: resolvePromise, reject: rejectPromise });
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
      throw new Error(response.exceptionDetails.text || "Android WebView evaluation failed");
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
      pending.reject(new Error(message.error.message || "Android WebView debug command failed"));
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
  await new Promise((resolvePromise, rejectPromise) => {
    server.once("error", rejectPromise);
    server.listen(0, "127.0.0.1", resolvePromise);
  });
  const address = server.address();
  await new Promise(resolvePromise => server.close(resolvePromise));
  if (!address || typeof address === "string") {
    throw new Error("failed to allocate TCP port");
  }
  return address.port;
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

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("Android WebView login smoke leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("Android WebView login smoke leaked a sensitive field name");
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
