import { randomUUID } from "node:crypto";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const schemaVersion = "nexusim.android-webview-login-smoke-plan.v1";

function main(argv) {
  const options = parseArgs(argv);
  const runID = options.runID || `android-webview-login-${randomUUID()}`;
  const plan = buildAndroidWebViewLoginSmokePlan({ runID });
  assertLowSensitive(plan);
  process.stdout.write(`${JSON.stringify(plan, null, 2)}\n`);
}

export function buildAndroidWebViewLoginSmokePlan(options = {}) {
  const runID = options.runID || `android-webview-login-${randomUUID()}`;
  return {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    runID,
    target: "android",
    objective: "Drive the rendered Android WebView through login, delivery.notify, PullInbox and AckDelivery using the same public UI action contract as browser and desktop.",
    prerequisites: [
      {
        name: "debuggable-apk",
        required: true,
        evidence: "APK is built from a debuggable variant so WebView remote inspection follows ApplicationInfo.FLAG_DEBUGGABLE."
      },
      {
        name: "collected-apk",
        required: true,
        evidence: "clients/artifacts/<run-id>/manifest.json contains an android APK entry with a SHA-256 digest."
      },
      {
        name: "authorized-adb-device",
        required: true,
        evidence: "npm --prefix clients run report:android-device-readiness reports at least one authorized device without raw serial/model leakage."
      },
      {
        name: "clientweb-fixture",
        required: true,
        evidence: "loadtest/clientweb prepares tenant, receiver, sender, conversation, BFF base URL and push WebSocket URL."
      },
      {
        name: "webview-devtools-socket",
        required: true,
        evidence: "adb exposes a webview_devtools_remote socket after launching com.nexusim.android/.MainActivity."
      }
    ],
    plannedFlow: [
      "Build a fresh Android debug APK with shell config pointing at the local BFF / push endpoints.",
      "Install the collected APK with adb install -r and launch com.nexusim.android/.MainActivity.",
      "Discover the WebView devtools socket through adb and forward it to a local loopback TCP port.",
      "Use the Chrome DevTools Protocol to fill the public Web UI login fields and submit through ClientShellActions.",
      "Trigger an external sender message through api-gateway BFF while the Android WebView is connected to push-gateway.",
      "Assert delivery.notify is observed by the WebView, PullInbox renders the message, and AckDelivery advances through the UI.",
      "Write a low-sensitive summary with booleans and sequence numbers only; never print auth input, raw device serial, local paths or message payload bodies."
    ],
    commands: {
      deviceReadiness: "npm --prefix clients run report:android-device-readiness",
      buildDebugAPK: "npm --prefix clients run build:android-apk:docker",
      installAndLaunch: "adb install -r <apk> && adb shell am start -n com.nexusim.android/.MainActivity",
      devtoolsDiscovery: "adb shell cat /proc/net/unix | findstr webview_devtools_remote",
      devtoolsForward: "adb forward tcp:<local-port> localabstract:<webview-devtools-socket>",
      runner: "npm --prefix clients run smoke:android-webview-login -- --fixture <clientweb-fixture.json>"
    },
    selectorContract: {
      source: "clients/web/src/App.tsx data-testid",
      required: [
        "login-tenant",
        "login-user",
        "login-submit",
        "runtime-status",
        "push-status",
        "conversation-id-input",
        "open-conversation",
        "message-list",
        "message-item",
        "ack-status"
      ]
    },
    verdict: {
      planOnly: true,
      loginLevelAndroidUISmoke: false,
      deliveryNotifyInWebView: false,
      pullInboxInWebView: false,
      ackDeliveryInWebView: false
    },
    boundaries: [
      "This is a plan artifact, not proof that the Android APK or login-level smoke has run.",
      "It does not download Android toolchains, build an APK, contact a device, install a package or open a network connection.",
      "The future runner must keep output low-sensitive and must not expose auth material, raw serial/model values or local absolute paths.",
      "The Android client remains a WebView shell over the shared TypeScript UI; Kotlin stays a thin platform bridge."
    ]
  };
}

function parseArgs(argv) {
  const options = { runID: "" };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--run-id") {
      options.runID = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return options;
}

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("Android WebView login smoke plan leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("Android WebView login smoke plan leaked a sensitive field name");
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
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
