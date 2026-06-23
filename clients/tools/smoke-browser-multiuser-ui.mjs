import { createHash, randomUUID } from "node:crypto";
import { createServer } from "node:net";
import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.browser-multiuser-ui-smoke.v1";
const defaultWebURL = "http://127.0.0.1:5173";

async function main(argv) {
  const options = parseArgs(argv);
  const runID = options.runID || `browser-multiuser-ui-${randomUUID()}`;
  const fixture = options.fixturePath ? readFixture(options.fixturePath) : undefined;
  const plan = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    dryRun: options.dryRun,
    runID,
    input: {
      source: options.fixturePath ? safeHint(options.fixturePath) : "not-provided",
      loginInputRequired: true,
      externalBrowserRequired: false
    },
    web: {
      url: options.webURL,
      canStartDevServer: options.startWeb,
      devServerCommand: "npm --prefix clients run dev:web -- --host 127.0.0.1 --port 5173"
    },
    automation: {
      driver: "chromium-cdp",
      uiSelectorContract: "clients/web/src/App.tsx data-testid + low-sensitive data-* ids",
      runtimeEndpointSource: "fixture-shell-config",
      requiredSelectors: [
        "login-user",
        "login-submit",
        "friend-conversation-item",
        "conversation-item",
        "new-group-name",
        "create-group",
        "group-settings-actions-tab",
        "group-invite-user",
        "group-invite-submit",
        "message-composer",
        "send-message",
        "message-list",
        "ack-status",
        "conversation-tag-filter",
        "conversation-archived-only",
        "conversation-draft-only",
        "active-conversation-actions",
        "active-conversation-archive-toggle",
        "conversation-archive-toggle",
        "conversation-tags-input",
        "conversation-tags-save",
        "conversation-draft-input",
        "conversation-draft-save",
        "conversation-draft-clear"
      ],
      lowSensitiveOutput: true
    },
    verdict: {
      browserMultiUserUISmoke: false,
      directChatThroughUI: false,
      groupChatThroughUI: false,
      groupInviteThroughUI: false,
      conversationManagementThroughUI: false,
      receiverAckObserved: false
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

  const browserExecutable = options.browserExecutable || findBrowserExecutable();
  if (!browserExecutable) {
    throw new Error("no Chrome or Edge executable found; pass --browser-executable explicitly");
  }

  let webServer;
  const senderBrowser = { child: null, cdp: null, tempRoot: "" };
  const receiverBrowser = { child: null, cdp: null, tempRoot: "" };
  let stage = "init";
  let webServerStarted = false;
  try {
    if (options.startWeb) {
      stage = "start-web";
      webServer = startWebDevServer(options.webURL);
      webServerStarted = true;
      await waitForHTTP(options.webURL, options.holdMs);
    }
    stage = "launch-sender-browser";
    await launchBrowserSession(senderBrowser, browserExecutable, options.webURL, "sender", options.holdMs, fixture, runID);
    stage = "launch-receiver-browser";
    await launchBrowserSession(receiverBrowser, browserExecutable, options.webURL, "receiver", options.holdMs, fixture, runID);

    stage = "sender-login";
    await driveLogin(senderBrowser.cdp, fixture.tenantID, fixture.senderUserID, fixture.senderLoginInput, options.holdMs);
    stage = "receiver-login";
    await driveLogin(receiverBrowser.cdp, fixture.tenantID, fixture.receiverUserID, fixture.receiverLoginInput, options.holdMs);

    stage = "open-direct-from-friend-list";
    const directConversationID = await openDirectFromFriendList(senderBrowser.cdp, fixture.receiverUserID, options.holdMs);
    const directText = `NexusIM browser UI direct ${runID}`;
    stage = "send-direct-message";
    const directSeq = await sendText(senderBrowser.cdp, directText, options.holdMs);
    stage = "receiver-open-direct";
    await openKnownConversation(receiverBrowser.cdp, directConversationID, options.holdMs);
    stage = "receiver-direct-message-visible";
    await waitForMessage(receiverBrowser.cdp, directText, directSeq, options.holdMs);
    stage = "receiver-direct-ack";
    const directAck = await waitForAck(receiverBrowser.cdp, directSeq, options.holdMs);

    const groupName = `NexusIM UI ${runID.slice(0, 12)}`;
    stage = "create-group-through-ui";
    const groupConversationID = await createGroup(senderBrowser.cdp, groupName, options.holdMs);
    stage = "invite-group-member-through-ui";
    await inviteGroupMember(senderBrowser.cdp, fixture.receiverUserID, options.holdMs);
    stage = "receiver-open-group";
    await openKnownConversation(receiverBrowser.cdp, groupConversationID, options.holdMs);
    const groupText = `NexusIM browser UI group ${runID}`;
    stage = "send-group-message";
    const groupSeq = await sendText(senderBrowser.cdp, groupText, options.holdMs);
    stage = "receiver-group-message-visible";
    await waitForMessage(receiverBrowser.cdp, groupText, groupSeq, options.holdMs);
    stage = "receiver-group-ack";
    const groupAck = await waitForAck(receiverBrowser.cdp, groupSeq, options.holdMs);
    stage = "exercise-conversation-management";
    const conversationManagement = await exerciseConversationManagement(
      senderBrowser.cdp,
      groupConversationID,
      runID,
      options.holdMs
    );

    const result = {
      ...plan,
      dryRun: false,
      web: {
        ...plan.web,
        devServerStarted: webServerStarted
      },
      automation: {
        ...plan.automation,
        senderBrowserStarted: true,
        receiverBrowserStarted: true
      },
      flow: {
        senderLoginOK: true,
        receiverLoginOK: true,
        directConversationID,
        directMessageSeq: directSeq,
        directAckSeq: directAck.seq,
        groupConversationID,
        groupName,
        groupMessageSeq: groupSeq,
        groupAckSeq: groupAck.seq,
        conversationManagement
      },
      verdict: {
        browserMultiUserUISmoke: true,
        directChatThroughUI: directAck.seq >= directSeq,
        groupChatThroughUI: groupAck.seq >= groupSeq,
        groupInviteThroughUI: true,
        conversationManagementThroughUI: conversationManagement.completed === true,
        receiverAckObserved: directAck.seq >= directSeq && groupAck.seq >= groupSeq
      },
      caveats: [
        "This smoke drives two isolated Chromium profiles through the rendered Web shell over CDP.",
        "It uses local smoke-only login input from a temporary fixture and never writes that input to output.",
        "It assumes the clientweb local stack has already created the tenant, users and contact relationship."
      ]
    };
    assertLowSensitive(result);
    emitResult(result, options);
  } catch (error) {
    const result = {
      ...plan,
      dryRun: false,
      web: {
        ...plan.web,
        devServerStarted: webServerStarted
      },
      automation: {
        ...plan.automation,
        senderBrowserStarted: Boolean(senderBrowser.child?.pid),
        receiverBrowserStarted: Boolean(receiverBrowser.child?.pid)
      },
      flow: {
        stage
      },
      verdict: {
        browserMultiUserUISmoke: false,
        directChatThroughUI: false,
        groupChatThroughUI: false,
        groupInviteThroughUI: false,
        conversationManagementThroughUI: false,
        receiverAckObserved: false
      },
      failure: {
        stage,
        message: safeFailureMessage(error)
      },
      diagnostics: {
        sender: await browserDiagnostics(senderBrowser),
        receiver: await browserDiagnostics(receiverBrowser)
      }
    };
    assertLowSensitive(result);
    emitResult(result, options);
    process.exitCode = 1;
  } finally {
    for (const session of [senderBrowser, receiverBrowser]) {
      session.cdp?.close();
      if (session.child?.pid) {
        terminateProcess(session.child.pid);
      }
      if (session.tempRoot) {
        removeTempRoot(session.tempRoot);
      }
    }
    if (webServer?.pid) {
      terminateProcess(webServer.pid);
    }
  }
  process.exit(process.exitCode ?? 0);
}

function dryRunExecutionPolicy() {
  return {
    planOnly: true,
    executesPlannedCommands: false,
    launchesBrowser: false,
    startsWebDevServer: false,
    usesBrowserAutomation: false,
    contactsBFF: false,
    opensPushWebSocket: false,
    sendsMessages: false,
    writesProtectedMaterial: false,
    downloadsToolchain: false
  };
}

function parseArgs(argv) {
  const options = {
    dryRun: false,
    fixturePath: "",
    outputPath: "",
    runID: "",
    holdMs: 60000,
    webURL: defaultWebURL,
    startWeb: false,
    browserExecutable: ""
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
    if (arg === "--output") {
      options.outputPath = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--run-id") {
      options.runID = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--hold-ms") {
      const value = Number.parseInt(requiredValue(argv, index, arg), 10);
      if (!Number.isInteger(value) || value < 10000 || value > 180000) {
        throw new Error("--hold-ms must be between 10000 and 180000");
      }
      options.holdMs = value;
      index += 1;
      continue;
    }
    if (arg === "--web-url") {
      options.webURL = requiredValue(argv, index, arg);
      assertURL(options.webURL, ["http:", "https:"], "--web-url");
      index += 1;
      continue;
    }
    if (arg === "--start-web") {
      options.startWeb = true;
      continue;
    }
    if (arg === "--browser-executable") {
      options.browserExecutable = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return options;
}

function readFixture(path) {
  const parsed = JSON.parse(readFileSync(path, "utf8"));
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("fixture must be a JSON object");
  }
  const fixture = {
    apiBaseURL: requiredString(parsed.apiBaseURL, "apiBaseURL"),
    pushWebSocketURL: requiredString(parsed.pushWebSocketURL, "pushWebSocketURL"),
    tenantID: requiredString(parsed.tenantID, "tenantID"),
    senderUserID: requiredString(parsed.senderUserID, "senderUserID"),
    senderLoginInput: requiredString(parsed.senderLoginInput, "senderLoginInput"),
    receiverUserID: requiredString(parsed.receiverUserID, "receiverUserID"),
    receiverLoginInput: requiredString(parsed.receiverLoginInput, "receiverLoginInput")
  };
  assertURL(fixture.apiBaseURL, ["http:", "https:"], "apiBaseURL");
  assertURL(fixture.pushWebSocketURL, ["ws:", "wss:"], "pushWebSocketURL");
  return fixture;
}

function emitResult(result, options) {
  const payload = `${JSON.stringify(result, null, 2)}\n`;
  if (options.outputPath) {
    writeFileSync(options.outputPath, payload, "utf8");
    return;
  }
  process.stdout.write(payload);
}

async function launchBrowserSession(session, executable, webURL, label, timeoutMs, fixture, runID) {
  session.tempRoot = mkdtempSync(join(tmpdir(), `nexusim-browser-ui-${label}-`));
  const debugPort = await getFreePort();
  session.child = spawn(executable, [
    `--remote-debugging-port=${debugPort}`,
    `--user-data-dir=${session.tempRoot}`,
    "--no-first-run",
    "--disable-first-run-ui",
    "--disable-default-apps",
    "--disable-background-networking",
    "--new-window",
    "about:blank"
  ], {
    stdio: "ignore",
    windowsHide: true
  });
  session.child.unref();
  session.cdp = await connectBrowserCDP(debugPort, timeoutMs);
  await installClientShellConfig(session.cdp, fixture, label, runID);
  await session.cdp.send("Page.navigate", { url: webURL });
}

async function installClientShellConfig(cdp, fixture, label, runID) {
  const config = {
    target: "browser",
    apiBaseURL: fixture.apiBaseURL,
    pushWebSocketURL: fixture.pushWebSocketURL,
    deviceID: `browser-${label}-${sha256Text(runID).slice(0, 12)}`
  };
  await cdp.send("Page.addScriptToEvaluateOnNewDocument", {
    source: `globalThis.__NEXUSIM_CLIENT_SHELL__ = ${JSON.stringify(config)};`
  });
}

function startWebDevServer(webURL) {
  const parsed = new URL(webURL);
  if (!isLoopbackHost(parsed.hostname) || parsed.protocol !== "http:" || parsed.port !== "5173") {
    throw new Error("--start-web currently requires http://127.0.0.1:5173 or http://localhost:5173");
  }
  const npm = process.platform === "win32" ? "npm.cmd" : "npm";
  const child = spawn(npm, ["--prefix", "clients", "run", "dev:web", "--", "--host", "127.0.0.1", "--port", "5173"], {
    cwd: resolve(workspaceRoot, ".."),
    stdio: "ignore",
    windowsHide: true,
    shell: process.platform === "win32"
  });
  child.unref();
  return child;
}

async function driveLogin(cdp, tenantID, userID, loginInput, timeoutMs) {
  await waitForSelector(cdp, "login-submit", timeoutMs);
  await setInput(cdp, "login-tenant", tenantID);
  await setInput(cdp, "login-user", userID);
  await setInput(cdp, "login-password", loginInput);
  await click(cdp, "login-submit");
  await waitForText(cdp, "runtime-status", value => value === "login ok", "login ok", timeoutMs);
  await waitForText(cdp, "push-status", value => value.includes("connected"), "push connected", timeoutMs);
}

async function openDirectFromFriendList(cdp, receiverUserID, timeoutMs) {
  await waitForEval(cdp, value => value?.conversationID ? { ok: true, conversationID: value.conversationID } : { ok: false }, {
    label: "active friend item",
    timeoutMs,
    expression: `(() => {
      const friend = Array.from(document.querySelectorAll('[data-testid="friend-conversation-item"]'))
        .find(item => item.dataset.contactUserId === ${JSON.stringify(receiverUserID)});
      if (!friend) return { ok: false };
      friend.click();
      return { ok: true };
    })()`
  });
  await waitForText(cdp, "runtime-status", value => value === "open direct conversation ok", "open direct conversation ok", timeoutMs);
  return activeConversationID(cdp, timeoutMs);
}

async function createGroup(cdp, groupName, timeoutMs) {
  await setInput(cdp, "new-group-name", groupName);
  await click(cdp, "create-group");
  await waitForText(cdp, "runtime-status", value => value === "create group ok", "create group ok", timeoutMs);
  return activeConversationID(cdp, timeoutMs);
}

async function inviteGroupMember(cdp, userID, timeoutMs) {
  await click(cdp, "group-settings-actions-tab");
  await waitForSelector(cdp, "group-invite-user", timeoutMs);
  await setInput(cdp, "group-invite-user", userID);
  await click(cdp, "group-invite-submit");
  await waitForText(cdp, "runtime-status", value => value === "invite group member ok", "invite group member ok", timeoutMs);
}

async function openKnownConversation(cdp, conversationID, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      await setInput(cdp, "conversation-id-input", conversationID);
      await click(cdp, "open-conversation");
      await waitForText(
        cdp,
        "runtime-status",
        value => value === "open conversation ok",
        "open conversation ok",
        Math.min(3000, Math.max(500, deadline - Date.now()))
      );
      return;
    } catch (error) {
      lastError = error;
      await sleep(500);
    }
  }
  throw new Error(`timed out opening conversation ${conversationID}: ${lastError?.message ?? "unknown error"}`);
}

async function sendText(cdp, text, timeoutMs) {
  await setInput(cdp, "message-composer", text);
  await click(cdp, "send-message");
  await waitForText(cdp, "runtime-status", value => value === "send message ok", "send message ok", timeoutMs);
  return waitForMessageSeq(cdp, text, timeoutMs);
}

async function waitForMessage(cdp, text, minSeq, timeoutMs) {
  await waitForEval(cdp, () => "", {
    label: "message visible",
    timeoutMs,
    expression: `(() => {
      const list = document.querySelector('[data-testid="message-list"]');
      const text = list?.textContent || "";
      const items = Array.from(document.querySelectorAll('[data-testid="message-item"]')).map(item => item.textContent || "");
      const seqVisible = items.some(item => item.includes("#${minSeq}"));
      return { ok: text.includes(${JSON.stringify(text)}) && seqVisible, text };
    })()`
  });
}

async function waitForMessageSeq(cdp, text, timeoutMs) {
  const result = await waitForEval(cdp, value => {
    const seq = Number.parseInt(String(value?.seq ?? "0"), 10);
    return Number.isInteger(seq) && seq > 0 ? { ok: true, seq } : { ok: false };
  }, {
    label: "sent message seq",
    timeoutMs,
    expression: `(() => {
      const item = Array.from(document.querySelectorAll('[data-testid="message-item"]'))
        .reverse()
        .find(node => (node.textContent || "").includes(${JSON.stringify(text)}));
      const match = (item?.textContent || "").match(/#(\\d+)/);
      return { ok: false, seq: match ? match[1] : "" };
    })()`
  });
  return result.seq;
}

async function waitForAck(cdp, minSeq, timeoutMs) {
  return waitForEval(cdp, value => {
    const match = String(value?.text ?? "").match(/#(\d+)/);
    const seq = match ? Number.parseInt(match[1], 10) : 0;
    return Number.isInteger(seq) && seq >= minSeq ? { ok: true, seq } : { ok: false };
  }, {
    label: "AckDelivery in browser UI",
    timeoutMs,
    expression: `(() => {
      const text = document.querySelector('[data-testid="ack-status"]')?.textContent || "";
      return { ok: false, text };
    })()`
  });
}

async function exerciseConversationManagement(cdp, conversationID, runID, timeoutMs) {
  const tag = "ui-smoke";
  const missingTag = `missing-${sha256Text(runID).slice(0, 8)}`;
  const draftText = `NexusIM UI smoke draft ${runID}`;
  await clickConversationItem(cdp, conversationID, timeoutMs);
  await waitForSelector(cdp, "active-conversation-actions", timeoutMs);

  await setInput(cdp, "conversation-tags-input", tag);
  await click(cdp, "conversation-tags-save");
  await waitForText(cdp, "runtime-status", value => value === "set conversation tags ok", "set conversation tags ok", timeoutMs);
  await waitForConversationRowText(cdp, conversationID, `#${tag}`, timeoutMs);

  await setInput(cdp, "conversation-tag-filter", tag);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItem(cdp, conversationID, timeoutMs);
  await setInput(cdp, "conversation-tag-filter", missingTag);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItemAbsent(cdp, conversationID, timeoutMs);
  await setInput(cdp, "conversation-tag-filter", tag);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItem(cdp, conversationID, timeoutMs);
  await clickConversationItem(cdp, conversationID, timeoutMs);

  await setInput(cdp, "conversation-draft-input", draftText);
  await click(cdp, "conversation-draft-save");
  await waitForText(cdp, "runtime-status", value => value === "set conversation draft ok", "set conversation draft ok", timeoutMs);
  await waitForConversationRowText(cdp, conversationID, "草稿", timeoutMs);

  await setCheckbox(cdp, "conversation-draft-only", true);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItem(cdp, conversationID, timeoutMs);
  await clickConversationItem(cdp, conversationID, timeoutMs);
  await click(cdp, "conversation-draft-clear");
  await waitForText(cdp, "runtime-status", value => value === "clear conversation draft ok", "clear conversation draft ok", timeoutMs);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItemAbsent(cdp, conversationID, timeoutMs);

  await setInput(cdp, "conversation-tag-filter", "");
  await setCheckbox(cdp, "conversation-draft-only", false);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItem(cdp, conversationID, timeoutMs);
  await clickConversationItem(cdp, conversationID, timeoutMs);
  await click(cdp, "active-conversation-archive-toggle");
  await waitForText(cdp, "runtime-status", value => value === "archive conversation ok", "archive conversation ok", timeoutMs);

  await setCheckbox(cdp, "conversation-archived-only", true);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItem(cdp, conversationID, timeoutMs);
  await waitForConversationRowText(cdp, conversationID, "归档", timeoutMs);
  await clickConversationItem(cdp, conversationID, timeoutMs);
  await click(cdp, "active-conversation-archive-toggle");
  await waitForText(cdp, "runtime-status", value => value === "unarchive conversation ok", "unarchive conversation ok", timeoutMs);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItemAbsent(cdp, conversationID, timeoutMs);
  await setCheckbox(cdp, "conversation-archived-only", false);
  await refreshConversations(cdp, timeoutMs);
  await waitForConversationItem(cdp, conversationID, timeoutMs);

  return {
    completed: true,
    conversationID,
    tag,
    tagFilterMatched: true,
    tagFilterExcludedMissingTag: true,
    draftSavedAndCleared: true,
    draftOnlyFilterMatched: true,
    draftOnlyFilterExcludedAfterClear: true,
    archiveRoundTrip: true,
    archivedOnlyFilterMatched: true,
    archivedOnlyFilterExcludedAfterUnarchive: true
  };
}

async function activeConversationID(cdp, timeoutMs) {
  const result = await waitForEval(cdp, value => value?.conversationID ? { ok: true, conversationID: value.conversationID } : { ok: false }, {
    label: "active conversation id",
    timeoutMs,
    expression: `(() => {
      const active = document.querySelector('[data-testid="conversation-item"][data-active="true"]');
      return { ok: Boolean(active?.dataset?.conversationId), conversationID: active?.dataset?.conversationId || "" };
    })()`
  });
  return result.conversationID;
}

async function waitForConversationItem(cdp, conversationID, timeoutMs) {
  await waitForEval(cdp, () => "", {
    label: `conversation item ${conversationID}`,
    timeoutMs,
    expression: `(() => {
      const item = Array.from(document.querySelectorAll('[data-testid="conversation-item"]'))
        .find(node => node.dataset.conversationId === ${JSON.stringify(conversationID)});
      return { ok: Boolean(item) };
    })()`
  });
}

async function waitForConversationItemAbsent(cdp, conversationID, timeoutMs) {
  await waitForEval(cdp, () => "", {
    label: `conversation item absent ${conversationID}`,
    timeoutMs,
    expression: `(() => {
      const item = Array.from(document.querySelectorAll('[data-testid="conversation-item"]'))
        .find(node => node.dataset.conversationId === ${JSON.stringify(conversationID)});
      return { ok: !item };
    })()`
  });
}

async function waitForConversationRowText(cdp, conversationID, expectedText, timeoutMs) {
  await waitForEval(cdp, () => "", {
    label: `conversation row ${conversationID} text ${expectedText}`,
    timeoutMs,
    expression: `(() => {
      const item = Array.from(document.querySelectorAll('[data-testid="conversation-item"]'))
        .find(node => node.dataset.conversationId === ${JSON.stringify(conversationID)});
      return { ok: Boolean(item && (item.textContent || "").includes(${JSON.stringify(expectedText)})) };
    })()`
  });
}

async function refreshConversations(cdp, timeoutMs) {
  await click(cdp, "refresh-conversations");
  await waitForText(
    cdp,
    "runtime-status",
    value => value === "load conversations ok" || value === "refresh conversations ok",
    "load conversations ok",
    timeoutMs
  );
}

async function clickConversationItem(cdp, conversationID, timeoutMs) {
  await waitForEval(cdp, value => value?.conversationID ? { ok: true, conversationID: value.conversationID } : { ok: false }, {
    label: `click conversation ${conversationID}`,
    timeoutMs,
    expression: `(() => {
      const item = Array.from(document.querySelectorAll('[data-testid="conversation-item"]'))
        .find(node => node.dataset.conversationId === ${JSON.stringify(conversationID)});
      if (!item) return { ok: false };
      item.click();
      return { ok: true, conversationID: item.dataset.conversationId || "" };
    })()`
  });
  await waitForActiveConversation(cdp, conversationID, timeoutMs);
}

async function waitForActiveConversation(cdp, conversationID, timeoutMs) {
  await waitForEval(cdp, () => "", {
    label: `active conversation ${conversationID}`,
    timeoutMs,
    expression: `(() => {
      const active = document.querySelector('[data-testid="conversation-item"][data-active="true"]');
      return { ok: active?.dataset?.conversationId === ${JSON.stringify(conversationID)} };
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
    if (!element) throw new Error('missing input ${testID}');
    const prototype = element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(prototype, 'value').set;
    setter.call(element, ${JSON.stringify(value)});
    element.dispatchEvent(new Event('input', { bubbles: true }));
    return true;
  })()`);
}

async function setCheckbox(cdp, testID, checked) {
  await cdp.evaluate(`(() => {
    const element = document.querySelector('[data-testid="${testID}"]');
    if (!element) throw new Error('missing checkbox ${testID}');
    if (!(element instanceof HTMLInputElement) || element.type !== 'checkbox') {
      throw new Error('selector is not checkbox ${testID}');
    }
    if (element.checked !== ${checked ? "true" : "false"}) {
      element.click();
    }
    return true;
  })()`);
}

async function click(cdp, testID) {
  await cdp.evaluate(`(() => {
    const element = document.querySelector('[data-testid="${testID}"]');
    if (!element) throw new Error('missing button ${testID}');
    if (element.disabled) throw new Error('button disabled ${testID}');
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

class CDPClient {
  constructor(socket) {
    this.socket = socket;
    this.nextID = 1;
    this.pending = new Map();
    this.networkIssues = [];
    socket.addEventListener("message", event => this.onMessage(event.data));
    socket.addEventListener("error", () => this.rejectAll(new Error("browser debug socket error")));
    socket.addEventListener("close", () => this.rejectAll(new Error("browser debug socket closed")));
  }

  static connect(url) {
    if (typeof WebSocket !== "function") {
      throw new Error("Node.js WebSocket is required for browser UI smoke");
    }
    const socket = new WebSocket(url);
    return new Promise((resolvePromise, rejectPromise) => {
      const timeout = setTimeout(() => {
        cleanup();
        rejectPromise(new Error("timed out connecting to browser debug socket"));
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
        rejectPromise(new Error("failed connecting to browser debug socket"));
      };
      socket.addEventListener("open", onOpen);
      socket.addEventListener("error", onError);
    });
  }

  send(method, params = {}) {
    const id = this.nextID++;
    const response = new Promise((resolvePromise, rejectPromise) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        rejectPromise(new Error(`browser debug command timed out: ${method}`));
      }, 10000);
      this.pending.set(id, {
        resolve: value => {
          clearTimeout(timer);
          resolvePromise(value);
        },
        reject: error => {
          clearTimeout(timer);
          rejectPromise(error);
        }
      });
    });
    this.socket.send(JSON.stringify({ id, method, params }));
    return response;
  }

  async evaluate(expression) {
    const response = await this.send("Runtime.evaluate", {
      expression,
      awaitPromise: true,
      returnByValue: true
    });
    if (response.exceptionDetails) {
      throw new Error(response.exceptionDetails.text || "browser evaluation failed");
    }
    return response.result?.value;
  }

  onMessage(data) {
    const message = JSON.parse(String(data));
    if (message.method) {
      this.recordEvent(message);
    }
    if (!message.id) {
      return;
    }
    const pending = this.pending.get(message.id);
    if (!pending) {
      return;
    }
    this.pending.delete(message.id);
    if (message.error) {
      pending.reject(new Error(message.error.message || "browser debug command failed"));
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

  recordEvent(message) {
    if (message.method === "Network.responseReceived") {
      const response = message.params?.response;
      const status = Number(response?.status ?? 0);
      if (status >= 400) {
        this.pushNetworkIssue({
          kind: "http-response",
          status,
          url: safeNetworkURL(response?.url)
        });
      }
      return;
    }
    if (message.method === "Network.loadingFailed") {
      this.pushNetworkIssue({
        kind: "loading-failed",
        errorText: safeNetworkText(message.params?.errorText),
        url: safeNetworkURL(message.params?.request?.url)
      });
    }
  }

  pushNetworkIssue(issue) {
    this.networkIssues.push(issue);
    if (this.networkIssues.length > 12) {
      this.networkIssues.shift();
    }
  }

  recentNetworkIssues() {
    return this.networkIssues.slice();
  }
}

async function connectBrowserCDP(port, timeoutMs) {
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
        await client.send("Page.enable");
        await client.send("Network.enable");
        return client;
      }
    } catch (error) {
      lastError = error;
    }
    await sleep(300);
  }
  throw new Error(`timed out waiting for browser debug target${lastError ? `: ${errorMessage(lastError)}` : ""}`);
}

async function fetchJSON(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`GET ${url} returned ${response.status}`);
  }
  return response.json();
}

async function waitForHTTP(url, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch (error) {
      lastError = error;
    }
    await sleep(500);
  }
  throw new Error(`timed out waiting for ${url}${lastError ? `: ${errorMessage(lastError)}` : ""}`);
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

function findBrowserExecutable() {
  const candidates = browserCandidates();
  return candidates.find(candidate => existsSync(candidate)) ?? "";
}

function browserCandidates() {
  if (process.platform === "win32") {
    const roots = [
      process.env.ProgramFiles,
      process.env["ProgramFiles(x86)"],
      process.env.LOCALAPPDATA
    ].filter(Boolean);
    return roots.flatMap(root => [
      join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
      join(root, "Google", "Chrome", "Application", "chrome.exe")
    ]);
  }
  if (process.platform === "darwin") {
    return [
      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
      "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
      "/Applications/Chromium.app/Contents/MacOS/Chromium"
    ];
  }
  return [
    "/usr/bin/google-chrome",
    "/usr/bin/google-chrome-stable",
    "/usr/bin/microsoft-edge",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser"
  ];
}

function terminateProcess(pid) {
  if (!pid) {
    return false;
  }
  if (process.platform === "win32") {
    const completed = spawnSync("taskkill", ["/PID", String(pid), "/T", "/F"], {
      encoding: "utf8",
      timeout: 10000,
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

function removeTempRoot(path) {
  try {
    rmSync(path, { recursive: true, force: true, maxRetries: 1, retryDelay: 100 });
  } catch {
    // Temp browser profiles are smoke-only artifacts. Cleanup is best-effort so
    // a locked profile cannot hide the already emitted smoke verdict.
  }
}

function safeFailureMessage(error) {
  const text = errorMessage(error).replace(/\s+/g, " ").trim().slice(0, 300);
  if (text.match(/(token|secret|password|credential|private|[A-Za-z]:\\|\\\\\?)/i)) {
    return "browser multi-user UI smoke failed with a sanitized local error";
  }
  return text || "browser multi-user UI smoke failed";
}

function requiredString(value, name) {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`fixture ${name} is required`);
  }
  return value;
}

function assertURL(value, protocols, name) {
  let parsed;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(`${name} must be a valid URL`);
  }
  if (!protocols.includes(parsed.protocol)) {
    throw new Error(`${name} has unsupported protocol`);
  }
}

function isLoopbackHost(host) {
  const normalized = String(host ?? "").toLowerCase();
  return normalized === "127.0.0.1" || normalized === "localhost" || normalized === "::1" || normalized === "[::1]";
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
    throw new Error("browser multi-user UI smoke leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("browser multi-user UI smoke leaked a sensitive field name");
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
    const page = await cdp.evaluate(`(() => ({
      url: location.href,
      title: document.title,
      runtimeStatus: document.querySelector('[data-testid="runtime-status"]')?.textContent || "",
      pushStatus: document.querySelector('[data-testid="push-status"]')?.textContent || "",
      ackStatus: document.querySelector('[data-testid="ack-status"]')?.textContent || "",
      error: (document.querySelector('[data-testid="error-banner"]')?.textContent || "").slice(0, 1000),
      activeConversationID: document.querySelector('[data-testid="conversation-item"][data-active="true"]')?.dataset?.conversationId || "",
      bodyTextPrefix: (document.body?.textContent || "").slice(0, 300)
    }))()`);
    return {
      ...page,
      recentNetworkIssues: cdp.recentNetworkIssues()
    };
  } catch (error) {
    return { error: errorMessage(error) };
  }
}

async function browserDiagnostics(session) {
  if (!session.cdp) {
    return { started: Boolean(session.child?.pid) };
  }
  return pageDiagnostics(session.cdp);
}

function safeNetworkURL(value) {
  if (typeof value !== "string" || value.trim() === "") {
    return "";
  }
  try {
    const parsed = new URL(value);
    const path = parsed.pathname.slice(0, 160);
    return `${parsed.origin}${path}`;
  } catch {
    return "";
  }
}

function safeNetworkText(value) {
  return typeof value === "string" ? value.replace(/\s+/g, " ").trim().slice(0, 120) : "";
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
