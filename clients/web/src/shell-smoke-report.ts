import type { ClientRuntimeConfig, ClientRuntimeTarget } from "@nexusim/client-core";
import type { ClientShellConfig, NativeBridgeMetadata } from "./runtime-config";

export const shellSmokeMetadataSchemaVersion = "nexusim.shell-webview-metadata-smoke.v1";

export interface ShellSmokeMetadataReport {
  readonly schemaVersion: typeof shellSmokeMetadataSchemaVersion;
  readonly mode: "metadata";
  readonly runID: string;
  readonly shellTarget: ClientRuntimeTarget | "unknown";
  readonly nativeMetadataReady: boolean;
  readonly native?: {
    readonly target: NativeBridgeMetadata["target"];
    readonly nativeBridgeVersion: string;
    readonly runtimeLabel: string;
  };
  readonly runtimeConfig: {
    readonly apiConfigured: boolean;
    readonly pushConfigured: boolean;
  };
}

export function shouldReportShellSmokeMetadata(config: ClientShellConfig): boolean {
  return Boolean(config.smokeCallbackURL && (!config.smokeMode || config.smokeMode === "metadata"));
}

export function buildShellSmokeMetadataReport(
  shellConfig: ClientShellConfig,
  runtimeConfig: ClientRuntimeConfig,
  nativeMetadata: NativeBridgeMetadata | undefined
): ShellSmokeMetadataReport {
  const report: ShellSmokeMetadataReport = {
    schemaVersion: shellSmokeMetadataSchemaVersion,
    mode: "metadata",
    runID: shellConfig.smokeRunID ?? "unspecified",
    shellTarget: shellConfig.target ?? "unknown",
    nativeMetadataReady: Boolean(nativeMetadata),
    runtimeConfig: {
      apiConfigured: runtimeConfig.apiBaseURL.trim() !== "",
      pushConfigured: runtimeConfig.pushWebSocketURL.trim() !== ""
    }
  };
  if (nativeMetadata) {
    return withCheckedReport({
      ...report,
      native: {
        target: nativeMetadata.target,
        nativeBridgeVersion: nativeMetadata.nativeBridgeVersion,
        runtimeLabel: nativeMetadata.runtimeLabel
      }
    });
  }
  return withCheckedReport(report);
}

function withCheckedReport(report: ShellSmokeMetadataReport): ShellSmokeMetadataReport {
  assertLowSensitiveReport(report);
  return report;
}

export async function postShellSmokeMetadataReport(
  callbackURL: string,
  report: ShellSmokeMetadataReport,
  fetchImpl: typeof fetch = globalThis.fetch
): Promise<void> {
  if (!fetchImpl) {
    throw new Error("fetch is not available for shell smoke callback");
  }
  assertLoopbackCallbackURL(callbackURL);
  assertLowSensitiveReport(report);
  const response = await fetchImpl(callbackURL, {
    method: "POST",
    headers: {
      "content-type": "application/json"
    },
    body: JSON.stringify(report)
  });
  if (!response.ok) {
    throw new Error(`shell smoke callback failed: ${response.status}`);
  }
}

export function assertLoopbackCallbackURL(value: string): void {
  const url = new URL(value);
  if (
    url.protocol !== "http:" ||
    (url.hostname !== "127.0.0.1" && url.hostname !== "localhost" && url.hostname !== "[::1]")
  ) {
    throw new Error("shell smoke callback must use loopback http");
  }
}

function assertLowSensitiveReport(report: ShellSmokeMetadataReport): void {
  const serialized = JSON.stringify(report);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("shell smoke report leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("shell smoke report leaked a sensitive field name");
  }
}
