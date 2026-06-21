import { validateRuntimeConfig } from "@nexusim/client-core";
import type { AndroidRuntimeConfig } from "./platform-contract";

declare const process:
  | {
      env?: Record<string, string | undefined>;
    }
  | undefined;

export function loadAndroidRuntimeConfig(
  overrides: Partial<AndroidRuntimeConfig> = {}
): AndroidRuntimeConfig {
  const env = readEnv();
  const config: AndroidRuntimeConfig = {
    apiBaseURL:
      overrides.apiBaseURL ??
      env.NEXUSIM_ANDROID_API_BASE ??
      env.NEXUSIM_CLIENT_API_BASE ??
      "http://127.0.0.1:8080",
    pushWebSocketURL:
      overrides.pushWebSocketURL ??
      env.NEXUSIM_ANDROID_PUSH_WS_URL ??
      env.NEXUSIM_CLIENT_PUSH_WS_URL ??
      "ws://127.0.0.1:8090/ws",
    deviceID:
      overrides.deviceID ??
      env.NEXUSIM_ANDROID_DEVICE_ID ??
      env.NEXUSIM_CLIENT_DEVICE_ID ??
      "android-local-device",
    platform: "android",
    secureStorage: overrides.secureStorage ?? "development",
    localStore: overrides.localStore ?? "local-storage",
    notificationProvider: overrides.notificationProvider ?? "none"
  };

  validateRuntimeConfig(config);
  return config;
}

function readEnv(): Record<string, string | undefined> {
  if (typeof process === "undefined") {
    return {};
  }
  return process.env ?? {};
}
