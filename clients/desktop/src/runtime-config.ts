import { validateRuntimeConfig } from "@nexusim/client-core";
import type { DesktopOS, DesktopRuntimeConfig } from "./platform-contract";

declare const process:
  | {
      env?: Record<string, string | undefined>;
      platform?: string;
    }
  | undefined;

export function loadDesktopRuntimeConfig(
  overrides: Partial<DesktopRuntimeConfig> = {}
): DesktopRuntimeConfig {
  const env = readEnv();
  const config: DesktopRuntimeConfig = {
    apiBaseURL:
      overrides.apiBaseURL ??
      env.NEXUSIM_DESKTOP_API_BASE ??
      env.NEXUSIM_CLIENT_API_BASE ??
      "http://127.0.0.1:8080",
    pushWebSocketURL:
      overrides.pushWebSocketURL ??
      env.NEXUSIM_DESKTOP_PUSH_WS_URL ??
      env.NEXUSIM_CLIENT_PUSH_WS_URL ??
      "ws://127.0.0.1:8090/ws",
    deviceID:
      overrides.deviceID ??
      env.NEXUSIM_DESKTOP_DEVICE_ID ??
      env.NEXUSIM_CLIENT_DEVICE_ID ??
      "desktop-local-device",
    os: overrides.os ?? detectDesktopOS(),
    secureStorage: overrides.secureStorage ?? "development",
    localStore: overrides.localStore ?? "local-storage",
    shell: "tauri"
  };

  validateRuntimeConfig(config);
  return config;
}

function detectDesktopOS(): DesktopOS {
  const platform = typeof process === "undefined" ? "" : process.platform ?? "";
  if (platform === "darwin") {
    return "macos";
  }
  if (platform === "linux") {
    return "linux";
  }
  return "windows";
}

function readEnv(): Record<string, string | undefined> {
  if (typeof process === "undefined") {
    return {};
  }
  return process.env ?? {};
}
