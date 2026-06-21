import type { ClientRuntimeConfig } from "@nexusim/client-core";

interface ViteImportMetaEnv {
  readonly VITE_NEXUSIM_API_BASE?: string;
  readonly VITE_NEXUSIM_WS_URL?: string;
  readonly VITE_NEXUSIM_DEVICE_ID?: string;
}

interface ViteImportMeta {
  readonly env: ViteImportMetaEnv;
}

export function loadRuntimeConfig(): ClientRuntimeConfig {
  const env = (import.meta as unknown as ViteImportMeta).env;
  return {
    apiBaseURL: env.VITE_NEXUSIM_API_BASE ?? "http://127.0.0.1:8080",
    pushWebSocketURL: env.VITE_NEXUSIM_WS_URL ?? "ws://127.0.0.1:8088/ws",
    deviceID: env.VITE_NEXUSIM_DEVICE_ID ?? "web-local-device"
  };
}
