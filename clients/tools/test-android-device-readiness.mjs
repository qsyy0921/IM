import {
  buildAndroidDeviceReadinessReport,
  parseAdbDevicesOutput
} from "./report-android-device-readiness.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const parsed = parseAdbDevicesOutput(`
List of devices attached
18b5eb2387 device product:SEA-AL10 model:nova_5_Pro device:HWSEA transport_id:2
192.168.0.149:5555 unauthorized product:unknown model:unknown transport_id:3
emulator-5554 offline transport_id:4
`);

assert(parsed.length === 3, "expected three parsed devices");
assert(parsed[0].serial === "18b5eb2387", "usb serial parse mismatch");
assert(parsed[0].state === "device", "usb state parse mismatch");
assert(parsed[0].transport === "usb", "usb transport parse mismatch");
assert(parsed[1].transport === "network", "network transport parse mismatch");
assert(parsed[2].transport === "emulator", "emulator transport parse mismatch");

const ready = buildAndroidDeviceReadinessReport({
  adbResult: {
    status: 0,
    stdout: `
List of devices attached
18b5eb2387 device product:SEA-AL10 model:nova_5_Pro device:HWSEA transport_id:2
`
  }
});
const readyText = JSON.stringify(ready);
assert(ready.schemaVersion === "nexusim.android-device-readiness.v1", "schema mismatch");
assert(ready.adbAvailable === true, "adb should be available");
assert(ready.readyForInstallSmoke === true, "device state should make install smoke ready");
assert(ready.counts.device === 1, "device count mismatch");
assert(ready.devices[0].serialHash.length === 16, "serial hash should be short");
assert(!readyText.includes("18b5eb2387"), "report leaked raw serial");
assert(!readyText.includes("nova_5_Pro"), "report leaked model name");
assert(ready.nextActions.some(action => action.action === "run-android-install-or-metadata-smoke"), "ready next action missing");

const unauthorized = buildAndroidDeviceReadinessReport({
  adbResult: {
    status: 0,
    stdout: `
List of devices attached
18b5eb2387 unauthorized product:SEA-AL10 model:nova_5_Pro transport_id:2
`
  }
});
assert(unauthorized.readyForInstallSmoke === false, "unauthorized device should not be install-smoke-ready");
assert(unauthorized.counts.unauthorized === 1, "unauthorized count mismatch");
assert(unauthorized.nextActions.some(action => action.action === "authorize-device"), "authorize next action missing");

const offline = buildAndroidDeviceReadinessReport({
  adbResult: {
    status: 0,
    stdout: `
List of devices attached
emulator-5554 offline transport_id:4
`
  }
});
assert(offline.counts.offline === 1, "offline count mismatch");
assert(offline.nextActions.some(action => action.action === "reconnect-adb"), "reconnect next action missing");

const missingAdb = buildAndroidDeviceReadinessReport({
  adbResult: {
    status: 1,
    stdout: ""
  }
});
assert(missingAdb.adbAvailable === false, "adb missing flag mismatch");
assert(missingAdb.readyForInstallSmoke === false, "missing adb should not be install-smoke-ready");
assert(missingAdb.nextActions.some(action => action.action === "install-android-platform-tools"), "platform tools next action missing");

console.log("Android device readiness report ok");
