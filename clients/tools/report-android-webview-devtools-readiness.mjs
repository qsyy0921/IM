import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { parseWebViewDevtoolsSockets } from "./smoke-android-webview-login.mjs";

const schemaVersion = "nexusim.android-webview-devtools-readiness.v1";

function main(argv) {
  const options = parseArgs(argv);
  const report = buildAndroidWebViewDevtoolsReadinessReport(options);
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
}

export function buildAndroidWebViewDevtoolsReadinessReport(options = {}) {
  const source = options.procNetUnixOutput !== undefined
    ? {
        status: 0,
        stdout: options.procNetUnixOutput,
        source: "fixture"
      }
    : readProcNetUnix();
  const sockets = source.status === 0 ? parseWebViewDevtoolsSockets(source.stdout) : [];
  const report = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    inputSource: source.source,
    adbAvailable: source.status !== "missing-adb",
    procNetUnixReadable: source.status === 0,
    readyForWebViewAutomation: sockets.length > 0,
    counts: {
      webViewDevtoolsSockets: sockets.length
    },
    sockets: sockets.map(redactSocket),
    commands: {
      discover: "adb shell cat /proc/net/unix",
      forward: "adb forward tcp:<local-port> localabstract:<webview-devtools-socket>",
      loginSmoke: "npm --prefix clients run smoke:android-webview-login -- --fixture <clientweb-fixture.json>"
    },
    nextActions: nextActions(source.status, sockets)
  };
  assertLowSensitive(report);
  return report;
}

function parseArgs(argv) {
  const options = {};
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--input") {
      const path = requiredValue(argv, index, arg);
      options.procNetUnixOutput = readFileSync(path, "utf8");
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return options;
}

function readProcNetUnix() {
  const result = spawnSync("adb", ["shell", "cat", "/proc/net/unix"], {
    encoding: "utf8",
    timeout: 5000,
    windowsHide: true
  });
  if (result.error?.code === "ENOENT") {
    return {
      status: "missing-adb",
      stdout: "",
      source: "adb"
    };
  }
  return {
    status: result.status ?? 1,
    stdout: result.stdout || result.stderr || "",
    source: "adb"
  };
}

function redactSocket(socket) {
  return {
    socketHash: sha256Text(socket).slice(0, 16),
    kind: "android-webview-devtools"
  };
}

function nextActions(status, sockets) {
  if (status === "missing-adb") {
    return [
      {
        action: "install-android-platform-tools",
        command: "npm --prefix clients run report:android-device-readiness"
      }
    ];
  }
  if (status !== 0) {
    return [
      {
        action: "check-authorized-device",
        command: "npm --prefix clients run report:android-device-readiness"
      },
      {
        action: "launch-debuggable-android-shell",
        reason: "proc/net/unix could not be read through adb"
      }
    ];
  }
  if (sockets.length === 0) {
    return [
      {
        action: "launch-debuggable-android-shell",
        reason: "no WebView devtools socket was visible"
      },
      {
        action: "recheck-webview-devtools",
        command: "npm --prefix clients run report:android-webview-devtools-readiness"
      }
    ];
  }
  return [
    {
      action: "run-android-webview-smoke",
      command: "npm --prefix clients run smoke:android-webview-login -- --fixture <clientweb-fixture.json>"
    }
  ];
}

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("Android WebView devtools readiness report leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("Android WebView devtools readiness report leaked a sensitive field name");
  }
  if (serialized.match(/webview_devtools_remote_\d+/i)) {
    throw new Error("Android WebView devtools readiness report leaked a raw socket name");
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

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(errorMessage(error));
    process.exitCode = 2;
  }
}
