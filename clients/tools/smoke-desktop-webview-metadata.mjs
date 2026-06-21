import { createHash, randomUUID } from "node:crypto";
import { createServer } from "node:http";
import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { localNodeBin, workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.desktop-webview-metadata-smoke.v1";
const metadataSchemaVersion = "nexusim.shell-webview-metadata-smoke.v1";
const toolsDir = dirname(fileURLToPath(import.meta.url));
const buildDesktopArtifactScript = join(toolsDir, "build-desktop-artifact.mjs");
const desktopArtifactPath = join(workspaceRoot, "desktop", "src-tauri", "target", "release", "nexusim-desktop.exe");

async function main(argv) {
  const options = parseArgs(argv);
  const runID = options.runID || `desktop-webview-metadata-${randomUUID()}`;
  const plan = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    dryRun: options.dryRun,
    runID,
    build: {
      command: "npm --prefix clients run smoke:desktop-webview-metadata",
      shellConfig: "temporary-loopback-metadata",
      artifactHint: safeHint(desktopArtifactPath)
    },
    callback: {
      mode: "metadata",
      loopbackOnly: true
    },
    verdict: {
      metadataWebViewSmoke: false,
      loginLevelDesktopUISmoke: false
    }
  };

  if (options.dryRun) {
    assertLowSensitive(plan);
    process.stdout.write(`${JSON.stringify(plan, null, 2)}\n`);
    return;
  }

  if (process.platform !== "win32") {
    throw new Error("desktop WebView metadata smoke is supported on Windows only");
  }

  const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-webview-metadata-"));
  let server;
  let callback;
  let child;
  try {
    callback = await startCallbackServer(runID, options.holdMs);
    server = callback.server;
    const shellConfigPath = join(tempRoot, "shell-config.json");
    writeShellConfig(shellConfigPath, {
      runID,
      callbackURL: callback.url,
      apiBaseURL: options.apiBaseURL,
      pushWebSocketURL: options.pushWebSocketURL,
      deviceID: options.deviceID
    });
    buildDesktopArtifact(shellConfigPath, options);
    validateArtifactExists();
    child = launchDesktopArtifact();
    callback.armTimeout(options.holdMs);
    const metadataReport = await callback.report;
    const terminated = terminateProcess(child.pid);
    await sleep(500);

    const result = {
      ...plan,
      dryRun: false,
      build: {
        ...plan.build,
        artifactReady: true
      },
      callback: summarizeMetadataReport(metadataReport),
      process: {
        started: true,
        terminated
      },
      verdict: {
        metadataWebViewSmoke: isExpectedMetadataReport(metadataReport, runID),
        loginLevelDesktopUISmoke: false
      },
      caveats: [
        "This smoke proves WebView-loaded shell assets can read desktop native metadata and post a low-sensitive callback.",
        "It does not submit login form data and does not prove PullInbox or WebSocket delivery inside the Tauri WebView."
      ]
    };
    assertLowSensitive(result);
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
  } finally {
    if (child?.pid) {
      terminateProcess(child.pid);
    }
    if (callback) {
      callback.cancel();
    }
    if (server) {
      await closeServer(server);
    }
    rmSync(tempRoot, { recursive: true, force: true });
  }
}

function parseArgs(argv) {
  const options = {
    dryRun: false,
    holdMs: 20000,
    runID: "",
    apiBaseURL: "http://127.0.0.1:8080",
    pushWebSocketURL: "ws://127.0.0.1:8088/ws",
    deviceID: "desktop-webview-metadata-device",
    skipWebBuild: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--dry-run") {
      options.dryRun = true;
      continue;
    }
    if (arg === "--hold-ms") {
      const value = Number.parseInt(requiredValue(argv, index, arg), 10);
      if (!Number.isInteger(value) || value < 5000 || value > 60000) {
        throw new Error("--hold-ms must be between 5000 and 60000");
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
    if (arg === "--api-base-url") {
      options.apiBaseURL = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--push-ws-url") {
      options.pushWebSocketURL = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--device-id") {
      options.deviceID = requiredValue(argv, index, arg);
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

async function startCallbackServer(runID, holdMs) {
  let resolveReport;
  let rejectReport;
  let settled = false;
  let timer;
  const report = new Promise((resolve, reject) => {
    resolveReport = resolve;
    rejectReport = reject;
  });

  const server = createServer((request, response) => {
    setCORSHeaders(response);
    if (request.method === "OPTIONS") {
      response.writeHead(204);
      response.end();
      return;
    }
    if (request.method !== "POST" || request.url !== "/shell-smoke") {
      response.writeHead(404);
      response.end();
      return;
    }
    let body = "";
    request.setEncoding("utf8");
    request.on("data", chunk => {
      body += chunk;
      if (body.length > 16 * 1024) {
        request.destroy();
      }
    });
    request.on("end", () => {
      try {
        const parsed = JSON.parse(body);
        validateMetadataReport(parsed, runID);
        if (!settled) {
          settled = true;
          clearTimeout(timer);
          resolveReport(parsed);
        }
        response.writeHead(202, { "content-type": "application/json" });
        response.end("{\"accepted\":true}\n");
      } catch (error) {
        if (!settled) {
          settled = true;
          clearTimeout(timer);
          rejectReport(error);
        }
        response.writeHead(400, { "content-type": "application/json" });
        response.end("{\"accepted\":false}\n");
      }
    });
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("callback server did not bind to a TCP port");
  }
  return {
    server,
    url: `http://127.0.0.1:${address.port}/shell-smoke`,
    report,
    armTimeout(timeoutMs = holdMs) {
      if (settled || timer) {
        return;
      }
      timer = setTimeout(() => {
        if (!settled) {
          settled = true;
          rejectReport(new Error("timed out waiting for desktop WebView metadata callback"));
        }
      }, timeoutMs);
    },
    cancel() {
      if (!settled) {
        settled = true;
      }
      if (timer) {
        clearTimeout(timer);
      }
    }
  };
}

function setCORSHeaders(response) {
  response.setHeader("access-control-allow-origin", "*");
  response.setHeader("access-control-allow-methods", "POST, OPTIONS");
  response.setHeader("access-control-allow-headers", "content-type");
}

function writeShellConfig(path, options) {
  writeFileSync(path, `${JSON.stringify({
    target: "windows-desktop",
    apiBaseURL: options.apiBaseURL,
    pushWebSocketURL: options.pushWebSocketURL,
    deviceID: options.deviceID,
    installationID: `desktop:windows:${options.deviceID}`,
    appVersion: "0.1.0",
    sessionKey: "nexusim:desktop:metadata-smoke",
    smokeCallbackURL: options.callbackURL,
    smokeRunID: options.runID,
    smokeMode: "metadata"
  }, null, 2)}\n`);
}

function buildDesktopArtifact(shellConfigPath, options) {
  const args = [
    buildDesktopArtifactScript,
    "--shell-config",
    shellConfigPath
  ];
  if (options.skipWebBuild) {
    args.push("--skip-web-build");
  }
  const completed = spawnSync(process.execPath, args, {
    cwd: resolve(workspaceRoot, ".."),
    encoding: "utf8",
    stdio: "inherit",
    env: process.env
  });
  if (completed.status !== 0) {
    throw new Error(`desktop artifact build failed with exit code ${completed.status ?? "unknown"}`);
  }
}

function validateArtifactExists() {
  if (!existsSync(localNodeBin("tauri"))) {
    throw new Error("repo-local Tauri CLI is missing; run npm --prefix clients install first");
  }
  if (!existsSync(desktopArtifactPath)) {
    throw new Error("desktop artifact build did not produce nexusim-desktop.exe");
  }
}

function launchDesktopArtifact() {
  const child = spawn(desktopArtifactPath, [], {
    cwd: dirname(desktopArtifactPath),
    stdio: "ignore",
    windowsHide: false
  });
  child.once("error", error => {
    throw error;
  });
  return child;
}

function validateMetadataReport(report, runID) {
  if (report?.schemaVersion !== metadataSchemaVersion) {
    throw new Error("metadata report schema mismatch");
  }
  if (report.runID !== runID) {
    throw new Error("metadata report run id mismatch");
  }
  if (!isExpectedMetadataReport(report, runID)) {
    throw new Error("metadata report did not prove desktop native metadata");
  }
  assertLowSensitive(report);
}

function isExpectedMetadataReport(report, runID) {
  return Boolean(
    report?.schemaVersion === metadataSchemaVersion &&
    report?.runID === runID &&
    report?.mode === "metadata" &&
    report?.shellTarget === "windows-desktop" &&
    report?.nativeMetadataReady === true &&
    report?.native?.target === "windows-desktop" &&
    typeof report?.native?.nativeBridgeVersion === "string" &&
    report.native.nativeBridgeVersion.trim() !== "" &&
    typeof report?.native?.runtimeLabel === "string" &&
    report.native.runtimeLabel.trim() !== "" &&
    report?.runtimeConfig?.apiConfigured === true &&
    report?.runtimeConfig?.pushConfigured === true
  );
}

function summarizeMetadataReport(report) {
  return {
    received: true,
    schemaVersion: report.schemaVersion,
    mode: report.mode,
    shellTarget: report.shellTarget,
    nativeMetadataReady: report.nativeMetadataReady,
    native: {
      target: report.native.target,
      nativeBridgeVersion: report.native.nativeBridgeVersion,
      runtimeLabel: report.native.runtimeLabel
    },
    runtimeConfig: {
      apiConfigured: report.runtimeConfig.apiConfigured,
      pushConfigured: report.runtimeConfig.pushConfigured
    }
  };
}

function terminateProcess(pid) {
  if (!pid) {
    return false;
  }
  if (process.platform === "win32") {
    const killed = spawnSync("taskkill", ["/PID", String(pid), "/T", "/F"], {
      encoding: "utf8",
      stdio: "ignore",
      windowsHide: true
    });
    return killed.status === 0;
  }
  try {
    process.kill(pid, "SIGTERM");
    return true;
  } catch {
    return false;
  }
}

function closeServer(server) {
  return new Promise(resolve => {
    server.close(() => resolve());
  });
}

function safeHint(path) {
  const relativePath = relative(resolve(workspaceRoot, ".."), resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(resolve(path)).slice(0, 12)}`;
  }
  return relativePath;
}

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("desktop WebView metadata smoke leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("desktop WebView metadata smoke leaked a sensitive field name");
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
  main(process.argv.slice(2)).catch(error => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  });
}
