import { randomUUID } from "node:crypto";
import { createServer } from "node:http";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { buildClientArtifactInstallPlan } from "./plan-client-artifact-install.mjs";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.android-webview-metadata-smoke.v1";
const metadataSchemaVersion = "nexusim.shell-webview-metadata-smoke.v1";
const toolsDir = dirname(fileURLToPath(import.meta.url));
const buildAndroidApkScript = join(toolsDir, "build-android-apk.mjs");
const packageName = "com.nexusim.android";
const mainActivity = "com.nexusim.android/.MainActivity";

async function main(argv) {
  const options = parseArgs(argv);
  const runID = options.runID || `android-webview-metadata-${randomUUID()}`;
  const plan = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    dryRun: options.dryRun,
    runID,
    build: {
      command: "npm --prefix clients run smoke:android-webview-metadata",
      shellConfig: "temporary-loopback-metadata",
      freshBuildRequired: true,
      freshBuildReason: "metadata callback URL is injected into shell assets before APK packaging",
      artifactHint: "clients/artifacts/<run-id>/nexusim-android-debug.apk"
    },
    adb: {
      packageName,
      mainActivity,
      reverseLoopback: true
    },
    callback: {
      mode: "metadata",
      loopbackOnly: true
    },
    verdict: {
      metadataWebViewSmoke: false,
      loginLevelAndroidUISmoke: false
    }
  };

  if (options.dryRun) {
    assertLowSensitive(plan);
    process.stdout.write(`${JSON.stringify(plan, null, 2)}\n`);
    return;
  }

  const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-android-webview-metadata-"));
  let callback;
  try {
    callback = await startCallbackServer(runID, options.holdMs);
    const shellConfigPath = join(tempRoot, "shell-config.json");
    writeShellConfig(shellConfigPath, {
      runID,
      callbackURL: callback.url,
      apiBaseURL: options.apiBaseURL,
      pushWebSocketURL: options.pushWebSocketURL,
      deviceID: options.deviceID
    });

    buildAndroidApk(shellConfigPath, runID, options);
    const manifestPath = join(workspaceRoot, "artifacts", runID, "manifest.json");
    const installPlan = buildClientArtifactInstallPlan({ manifest: manifestPath });
    const androidPlan = installPlan.targets.android;
    if (!androidPlan?.artifactReady || !androidPlan.readyForInstall) {
      throw new Error("Android APK artifact or adb install prerequisites are not ready");
    }
    const apkPath = artifactPathFromManifest(manifestPath);

    reverseAdbLoopback(callback.port);
    installAndLaunch(apkPath);
    callback.armTimeout(options.holdMs);
    const metadataReport = await callback.report;
    await sleep(500);

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
        launched: true
      },
      callback: summarizeMetadataReport(metadataReport),
      verdict: {
        metadataWebViewSmoke: isExpectedMetadataReport(metadataReport, runID),
        loginLevelAndroidUISmoke: false
      },
      caveats: [
        "This smoke proves Android WebView-loaded shell assets can read native metadata and post a low-sensitive callback through adb reverse.",
        "It does not submit login form data and does not prove PullInbox or WebSocket delivery inside the Android WebView."
      ]
    };
    assertLowSensitive(result);
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
  } finally {
    if (callback?.port) {
      removeAdbReverse(callback.port);
    }
    if (callback) {
      callback.cancel();
      await closeServer(callback.server);
    }
    rmSync(tempRoot, { recursive: true, force: true });
  }
}

function parseArgs(argv) {
  const options = {
    dryRun: false,
    holdMs: 30000,
    runID: "",
    apiBaseURL: "http://127.0.0.1:8080",
    pushWebSocketURL: "ws://127.0.0.1:8088/ws",
    deviceID: "android-webview-metadata-device",
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
      if (!Number.isInteger(value) || value < 5000 || value > 120000) {
        throw new Error("--hold-ms must be between 5000 and 120000");
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
  const report = new Promise((resolvePromise, rejectPromise) => {
    resolveReport = resolvePromise;
    rejectReport = rejectPromise;
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
  await new Promise((resolvePromise, rejectPromise) => {
    server.once("error", rejectPromise);
    server.listen(0, "127.0.0.1", resolvePromise);
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("callback server did not bind to a TCP port");
  }
  return {
    server,
    port: address.port,
    url: `http://127.0.0.1:${address.port}/shell-smoke`,
    report,
    armTimeout(timeoutMs = holdMs) {
      if (settled || timer) {
        return;
      }
      timer = setTimeout(() => {
        if (!settled) {
          settled = true;
          rejectReport(new Error("timed out waiting for Android WebView metadata callback"));
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
    target: "android",
    apiBaseURL: options.apiBaseURL,
    pushWebSocketURL: options.pushWebSocketURL,
    deviceID: options.deviceID,
    installationID: `android:${options.deviceID}`,
    appVersion: "0.1.0",
    sessionKey: "nexusim:android:metadata-smoke",
    smokeCallbackURL: options.callbackURL,
    smokeRunID: options.runID,
    smokeMode: "metadata"
  }, null, 2)}\n`);
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

function reverseAdbLoopback(port) {
  runAdb(["reverse", `tcp:${port}`, `tcp:${port}`], "adb reverse failed");
}

function removeAdbReverse(port) {
  spawnSync("adb", ["reverse", "--remove", `tcp:${port}`], {
    stdio: "ignore",
    windowsHide: true
  });
}

function installAndLaunch(apkPath) {
  runAdb(["install", "-r", apkPath], "adb install failed");
  runAdb(["shell", "am", "force-stop", packageName], "adb force-stop failed");
  runAdb(["shell", "am", "start", "-n", mainActivity], "adb activity launch failed");
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

function validateMetadataReport(report, runID) {
  if (report?.schemaVersion !== metadataSchemaVersion) {
    throw new Error("metadata report schema mismatch");
  }
  if (report.runID !== runID) {
    throw new Error("metadata report run id mismatch");
  }
  if (!isExpectedMetadataReport(report, runID)) {
    throw new Error("metadata report did not prove Android native metadata");
  }
  assertLowSensitive(report);
}

function isExpectedMetadataReport(report, runID) {
  return Boolean(
    report?.schemaVersion === metadataSchemaVersion &&
    report?.runID === runID &&
    report?.mode === "metadata" &&
    report?.shellTarget === "android" &&
    report?.nativeMetadataReady === true &&
    report?.native?.target === "android" &&
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

function closeServer(server) {
  return new Promise(resolvePromise => {
    server.close(() => resolvePromise());
  });
}

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("Android WebView metadata smoke leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("Android WebView metadata smoke leaked a sensitive field name");
  }
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
