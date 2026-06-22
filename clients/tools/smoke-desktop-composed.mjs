import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.desktop-composed-smoke.v1";
const toolsDir = dirname(fileURLToPath(import.meta.url));
const desktopLaunchSmokeScript = join(toolsDir, "smoke-desktop-artifact-launch.mjs");
const clientWebSmokeScript = join(workspaceRoot, "..", "loadtest", "clientweb", "run-local-smoke.ps1");

function main(argv) {
  const options = parseArgs(argv);
  const clientWebSummaryPath = resolveClientWebSummary(options);
  const clientWebSummary = summarizeClientWebSummary(readJsonFile(clientWebSummaryPath), clientWebSummaryPath);
  const desktopLaunch = runDesktopLaunchSmoke(options);

  const result = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    mode: {
      ranClientWeb: options.runClientWeb,
      launchDryRun: options.launchDryRun
    },
    executionPolicy: buildExecutionPolicy(options),
    clientWeb: clientWebSummary,
    desktop: summarizeDesktopLaunch(desktopLaunch),
    verdict: buildVerdict(clientWebSummary, desktopLaunch),
    caveats: [
      "This composed smoke combines the public clientweb BFF/push path with desktop artifact process launch.",
      "It is not GUI automation and does not prove login-level Tauri WebView interaction."
    ]
  };
  assertLowSensitive(result);
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
}

function parseArgs(argv) {
  const options = {
    clientWebSummary: "",
    runClientWeb: false,
    bindHost: "127.0.0.1",
    clientHost: "",
    runName: "",
    resultRoot: "",
    skipBuild: false,
    artifactManifest: "",
    holdMs: 5000,
    launchDryRun: false
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--clientweb-summary") {
      options.clientWebSummary = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--run-clientweb") {
      options.runClientWeb = true;
      continue;
    }
    if (arg === "--bind-host") {
      options.bindHost = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--client-host") {
      options.clientHost = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--run-name") {
      options.runName = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--result-root") {
      options.resultRoot = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--skip-build") {
      options.skipBuild = true;
      continue;
    }
    if (arg === "--artifact-manifest") {
      options.artifactManifest = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--hold-ms") {
      const value = Number.parseInt(requiredValue(argv, index, arg), 10);
      if (!Number.isInteger(value) || value < 1000 || value > 30000) {
        throw new Error("--hold-ms must be between 1000 and 30000");
      }
      options.holdMs = value;
      index += 1;
      continue;
    }
    if (arg === "--launch-dry-run") {
      options.launchDryRun = true;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }

  if (options.clientWebSummary && options.runClientWeb) {
    throw new Error("--clientweb-summary and --run-clientweb are mutually exclusive");
  }
  if (!options.clientWebSummary && !options.runClientWeb) {
    throw new Error("provide --clientweb-summary or --run-clientweb");
  }
  return options;
}

function buildExecutionPolicy(options) {
  return {
    planOnly: options.launchDryRun === true && options.runClientWeb !== true,
    readsClientWebSummary: options.clientWebSummary !== "",
    runsClientWebSmoke: options.runClientWeb === true,
    startsServices: options.runClientWeb === true,
    readsArtifactManifest: true,
    validatesArtifactFile: true,
    runsDesktopLaunchSmoke: true,
    desktopLaunchDryRun: options.launchDryRun === true,
    startsDesktopArtifact: options.launchDryRun !== true,
    opensNetworkConnection: options.runClientWeb === true,
    installsArtifacts: false,
    contactsDevice: false,
    startsDocker: false,
    downloadsToolchain: false
  };
}

function resolveClientWebSummary(options) {
  if (options.clientWebSummary) {
    const summaryPath = resolve(options.clientWebSummary);
    if (!existsSync(summaryPath)) {
      throw new Error("clientweb summary was not found");
    }
    return summaryPath;
  }

  if (process.platform !== "win32") {
    throw new Error("--run-clientweb currently requires Windows PowerShell");
  }

  const args = [
    "-NoProfile",
    "-ExecutionPolicy",
    "Bypass",
    "-File",
    clientWebSmokeScript,
    "-BindHost",
    options.bindHost
  ];
  if (options.clientHost) {
    args.push("-ClientHost", options.clientHost);
  }
  if (options.runName) {
    args.push("-RunName", options.runName);
  }
  if (options.resultRoot) {
    args.push("-ResultRoot", options.resultRoot);
  }
  if (options.skipBuild) {
    args.push("-SkipBuild");
  }

  const completed = spawnSync("powershell", args, {
    cwd: resolve(workspaceRoot, ".."),
    encoding: "utf8",
    windowsHide: true
  });
  if (completed.status !== 0) {
    throw new Error(`clientweb smoke failed with exit code ${completed.status ?? "unknown"}`);
  }
  const resultDir = parseResultDir(completed.stdout ?? "");
  const summaryPath = join(resultDir, "client-web-summary.json");
  if (!existsSync(summaryPath)) {
    throw new Error("clientweb smoke did not produce client-web-summary.json");
  }
  return summaryPath;
}

function parseResultDir(stdout) {
  const match = stdout.match(/^result_dir=(.+)$/m);
  if (!match?.[1]) {
    throw new Error("clientweb smoke output did not include result_dir");
  }
  return resolve(match[1].trim());
}

function readJsonFile(path) {
  const raw = readFileSync(path, "utf8");
  return JSON.parse(raw);
}

function summarizeClientWebSummary(summary, summaryPath) {
  const sendSeq = integerOrNull(summary.send_message?.conversation_seq);
  const notifySeq = integerOrNull(summary.delivery_notify?.conversation_seq);
  const pullMaxSeq = integerOrNull(summary.pull_inbox?.max_seq);
  const ackSeq = integerOrNull(summary.ack_delivery?.last_received_seq);
  const cursorSeq = integerOrNull(summary.postgres?.device_delivery_cursor_seq);
  const sentMessageID = stringOrEmpty(summary.send_message?.message_id);
  const notifiedMessageID = stringOrEmpty(summary.delivery_notify?.message_id);
  return {
    summaryHint: safeHint(summaryPath),
    success: summary.success === true,
    cleanGit: summary.git_dirty === false,
    commit: safeCommit(summary.commit),
    flow: {
      bffLogin: Boolean(summary.sender_login && summary.receiver_login),
      pushHello: summary.server_hello?.op === "server.hello",
      sendSeq,
      notifySeq,
      notifyMatchesSend: notifySeq !== null && notifySeq === sendSeq && sentMessageID !== "" && sentMessageID === notifiedMessageID,
      pullItemCount: integerOrNull(summary.pull_inbox?.item_count),
      pullMaxSeq,
      pullMatchesNotify: pullMaxSeq !== null && pullMaxSeq === notifySeq,
      conversationCount: integerOrNull(summary.list_conversations?.item_count),
      ackSeq,
      ackMatchesPull: ackSeq !== null && ackSeq === pullMaxSeq,
      inboxRows: integerOrNull(summary.postgres?.user_inbox_count),
      cursorSeq,
      cursorMatchesAck: cursorSeq !== null && cursorSeq === ackSeq
    }
  };
}

function runDesktopLaunchSmoke(options) {
  const args = [
    desktopLaunchSmokeScript,
    "--hold-ms",
    String(options.holdMs)
  ];
  if (options.artifactManifest) {
    args.push("--manifest", options.artifactManifest);
  }
  if (options.launchDryRun) {
    args.push("--dry-run");
  }
  const output = execFileSync(process.execPath, args, {
    cwd: resolve(workspaceRoot, ".."),
    encoding: "utf8"
  });
  return JSON.parse(output);
}

function summarizeDesktopLaunch(launch) {
  return {
    manifestHint: stringOrEmpty(launch.manifestHint),
    artifact: {
      filename: stringOrEmpty(launch.artifact?.filename),
      bytes: integerOrNull(launch.artifact?.bytes),
      sha256: stringOrEmpty(launch.artifact?.sha256)
    },
    dryRun: launch.dryRun === true,
    launched: launch.launched === true,
    processStarted: launch.processStarted === true,
    aliveMs: integerOrNull(launch.aliveMs),
    terminated: launch.terminated === true
  };
}

function buildVerdict(clientWeb, desktopLaunch) {
  const flow = clientWeb.flow;
  const clientWebPath = Boolean(
    clientWeb.success &&
    flow.bffLogin &&
    flow.pushHello &&
    flow.notifyMatchesSend &&
    flow.pullMatchesNotify &&
    flow.ackMatchesPull &&
    flow.cursorMatchesAck
  );
  const desktopLaunchPath = desktopLaunch.dryRun === true
    ? false
    : Boolean(desktopLaunch.launched && desktopLaunch.processStarted && desktopLaunch.terminated);
  return {
    clientWebPath,
    desktopLaunchPath,
    composedEvidenceReady: clientWebPath && (desktopLaunchPath || desktopLaunch.dryRun === true),
    loginLevelDesktopUISmoke: false
  };
}

function integerOrNull(value) {
  return Number.isInteger(value) ? value : null;
}

function stringOrEmpty(value) {
  return typeof value === "string" ? value : "";
}

function safeCommit(value) {
  if (typeof value !== "string") {
    return "";
  }
  const match = value.match(/^[a-f0-9]{7,12}$/i);
  return match ? value : "";
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
    throw new Error("desktop composed smoke leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop composed smoke leaked a sensitive field name");
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

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
