import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const schemaVersion = "nexusim.android-device-readiness.v1";

function main() {
  process.stdout.write(`${JSON.stringify(buildAndroidDeviceReadinessReport(), null, 2)}\n`);
}

export function buildAndroidDeviceReadinessReport(options = {}) {
  const adbResult = options.adbResult ?? runAdbDevices();
  const devices = adbResult.status === 0 ? parseAdbDevicesOutput(adbResult.stdout) : [];
  const report = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    executionPolicy: readinessExecutionPolicy(),
    adbAvailable: adbResult.status === 0,
    readyForInstallSmoke: devices.some(device => device.state === "device"),
    counts: {
      total: devices.length,
      device: devices.filter(device => device.state === "device").length,
      unauthorized: devices.filter(device => device.state === "unauthorized").length,
      offline: devices.filter(device => device.state === "offline").length,
      other: devices.filter(device => !["device", "unauthorized", "offline"].includes(device.state)).length
    },
    devices: devices.map(redactDevice),
    commands: {
      listDevices: "adb devices -l",
      reconnect: "adb reconnect",
      resetServer: "adb kill-server && adb start-server"
    },
    nextActions: nextActions(adbResult.status === 0, devices)
  };
  assertLowSensitive(report);
  return report;
}

export function parseAdbDevicesOutput(output) {
  return output
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(line => line && !line.toLowerCase().startsWith("list of devices attached"))
    .map(parseAdbDeviceLine)
    .filter(Boolean);
}

function parseAdbDeviceLine(line) {
  const [serial, state, ...details] = line.split(/\s+/);
  if (!serial || !state) {
    return null;
  }
  return {
    serial,
    state,
    transport: classifyTransport(serial),
    detailKeys: details
      .map(detail => detail.split(":")[0])
      .filter(Boolean)
      .sort()
  };
}

function classifyTransport(serial) {
  if (serial.includes(":")) {
    return "network";
  }
  if (serial.toLowerCase().startsWith("emulator-")) {
    return "emulator";
  }
  return "usb";
}

function redactDevice(device) {
  return {
    serialHash: sha256Text(device.serial).slice(0, 16),
    state: device.state,
    transport: device.transport,
    detailKeys: device.detailKeys
  };
}

function nextActions(adbAvailable, devices) {
  if (!adbAvailable) {
    return [
      {
        action: "install-android-platform-tools",
        reason: "adb command is not available"
      }
    ];
  }
  if (devices.some(device => device.state === "device")) {
    return [
      {
        action: "run-android-install-or-metadata-smoke",
        command: "npm --prefix clients run plan:artifact-install"
      }
    ];
  }
  if (devices.some(device => device.state === "unauthorized")) {
    return [
      {
        action: "authorize-device",
        reason: "confirm USB debugging authorization prompt on the Android device"
      },
      {
        action: "recheck-devices",
        command: "adb devices -l"
      }
    ];
  }
  if (devices.some(device => device.state === "offline")) {
    return [
      {
        action: "reconnect-adb",
        command: "adb reconnect"
      },
      {
        action: "recheck-devices",
        command: "adb devices -l"
      }
    ];
  }
  return [
    {
      action: "connect-android-device",
      reason: "no Android device is visible to adb"
    },
    {
      action: "recheck-devices",
      command: "adb devices -l"
    }
  ];
}

function readinessExecutionPolicy() {
  return {
    reportOnly: true,
    planOnly: false,
    runsReadinessCommands: true,
    readsAdbDeviceList: true,
    contactsDeviceReadOnly: true,
    readsLocalToolchainState: false,
    readsDockerBuilderState: false,
    readsWebViewDevtoolsSockets: false,
    buildsNativeArtifacts: false,
    startsServices: false,
    startsDocker: false,
    buildsDockerImages: false,
    installsArtifacts: false,
    startsDeviceActivities: false,
    opensAdbReverse: false,
    opensAdbForward: false,
    downloadsToolchain: false,
    exposesRawDeviceIdentifiers: false
  };
}

function runAdbDevices() {
  const result = spawnSync("adb", ["devices", "-l"], {
    encoding: "utf8",
    timeout: 5000,
    windowsHide: true
  });
  return {
    status: result.status ?? 1,
    stdout: result.stdout ?? ""
  };
}

function assertLowSensitive(value) {
  const serialized = JSON.stringify(value);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("Android device readiness report leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("Android device readiness report leaked a sensitive field name");
  }
  if (serialized.match(/nova|huawei|honor|xiaomi|samsung|pixel/i)) {
    throw new Error("Android device readiness report leaked a device model string");
  }
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
