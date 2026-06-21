import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import * as esbuild from "esbuild";

const tempDir = mkdtempSync(join(tmpdir(), "nexusim-web-shell-smoke-report-"));
const bundlePath = join(tempDir, "web-shell-smoke-report-entry.mjs");
const toolsDir = fileURLToPath(new URL(".", import.meta.url));
const entryPath = join(toolsDir, `.web-shell-smoke-report-entry-${process.pid}.ts`);

async function main() {
  writeFileSync(
    entryPath,
    `
      export {
        assertLoopbackCallbackURL,
        buildShellSmokeMetadataReport,
        postShellSmokeMetadataReport,
        shouldReportShellSmokeMetadata
      } from "../web/src/shell-smoke-report";
    `,
    "utf8"
  );

  await esbuild.build({
    entryPoints: [entryPath],
    bundle: true,
    platform: "browser",
    format: "esm",
    outfile: bundlePath,
    logLevel: "silent"
  });

  const {
    assertLoopbackCallbackURL,
    buildShellSmokeMetadataReport,
    postShellSmokeMetadataReport,
    shouldReportShellSmokeMetadata
  } = await import(pathToFileURL(bundlePath).href);

  const shellConfig = {
    target: "windows-desktop",
    smokeCallbackURL: "http://127.0.0.1:49152/shell-smoke",
    smokeRunID: "desktop-metadata-test",
    smokeMode: "metadata"
  };
  assertEqual(shouldReportShellSmokeMetadata(shellConfig), true, "smoke metadata should report");

  const report = buildShellSmokeMetadataReport(
    shellConfig,
    {
      apiBaseURL: "http://172.31.50.1:8080",
      pushWebSocketURL: "ws://172.31.50.1:8088/ws",
      deviceID: "desktop-local-device"
    },
    {
      target: "windows-desktop",
      nativeBridgeVersion: "0.1.0",
      runtimeLabel: "NexusIM desktop shell"
    }
  );
  assertEqual(report.schemaVersion, "nexusim.shell-webview-metadata-smoke.v1", "schema mismatch");
  assertEqual(report.nativeMetadataReady, true, "native metadata readiness mismatch");
  assertEqual(report.native?.target, "windows-desktop", "native target mismatch");
  assertEqual(report.runtimeConfig.apiConfigured, true, "api configured mismatch");

  let postedBody = "";
  await postShellSmokeMetadataReport(shellConfig.smokeCallbackURL, report, async (_url, init) => {
    postedBody = String(init.body);
    return { ok: true, status: 202 };
  });
  assertIncludes(postedBody, "desktop-metadata-test", "posted report includes run id");
  assertNoSensitive(postedBody, "posted smoke report");

  assertThrows(
    () => assertLoopbackCallbackURL("https://example.com/shell-smoke"),
    "loopback",
    "non-loopback callback rejected"
  );

  console.log("web shell smoke report ok");
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assertIncludes(value, expected, message) {
  if (!value.includes(expected)) {
    throw new Error(`${message}: expected ${expected}`);
  }
}

function assertNoSensitive(value, message) {
  if (value.match(/[A-Z]:\\\\/) || value.includes("\\\\?")) {
    throw new Error(`${message}: leaked Windows path`);
  }
  if (value.match(/token|secret|password|credential|private/i)) {
    throw new Error(`${message}: leaked sensitive field`);
  }
}

function assertThrows(fn, expected, message) {
  try {
    fn();
  } catch (error) {
    const actual = error instanceof Error ? error.message : String(error);
    if (!actual.includes(expected)) {
      throw new Error(`${message}: expected ${expected}, got ${actual}`);
    }
    return;
  }
  throw new Error(`${message}: expected error`);
}

try {
  await main();
} finally {
  try {
    rmSync(entryPath, { force: true });
  } catch {
    // The temp entry may not exist if startup failed before it was written.
  }
  rmSync(tempDir, { recursive: true, force: true });
}
