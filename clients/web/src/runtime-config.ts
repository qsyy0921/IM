import type { ClientRuntimeConfig, ClientRuntimeTarget } from "@nexusim/client-core";

interface ViteImportMetaEnv {
  readonly VITE_NEXUSIM_API_BASE?: string;
  readonly VITE_NEXUSIM_WS_URL?: string;
  readonly VITE_NEXUSIM_DEVICE_ID?: string;
}

interface ViteImportMeta {
  readonly env?: ViteImportMetaEnv;
}

export interface ClientShellConfig {
  readonly target?: ClientRuntimeTarget;
  readonly apiBaseURL?: string;
  readonly pushWebSocketURL?: string;
  readonly deviceID?: string;
  readonly installationID?: string;
  readonly appVersion?: string;
  readonly sessionKey?: string;
}

export interface AndroidNativeBridgeMetadata {
  readonly target: "android";
  readonly nativeBridgeVersion: string;
  readonly runtimeLabel: string;
}

export function loadRuntimeConfig(): ClientRuntimeConfig {
  const env = (import.meta as unknown as ViteImportMeta).env ?? {};
  const shell = readClientShellConfig();
  return {
    apiBaseURL: shell.apiBaseURL ?? env.VITE_NEXUSIM_API_BASE ?? "http://127.0.0.1:8080",
    pushWebSocketURL: shell.pushWebSocketURL ?? env.VITE_NEXUSIM_WS_URL ?? "ws://127.0.0.1:8088/ws",
    deviceID: shell.deviceID ?? env.VITE_NEXUSIM_DEVICE_ID ?? "web-local-device"
  };
}

export function readAndroidNativeBridgeMetadata(): AndroidNativeBridgeMetadata | undefined {
  const bridge = (globalThis as {
    NexusIMNative?: {
      runtimeMetadata?: () => string;
    };
  }).NexusIMNative;
  if (!bridge || typeof bridge.runtimeMetadata !== "function") {
    return undefined;
  }
  try {
    const raw = JSON.parse(bridge.runtimeMetadata()) as Record<string, unknown>;
    if (
      raw.target !== "android" ||
      typeof raw.nativeBridgeVersion !== "string" ||
      raw.nativeBridgeVersion.trim() === "" ||
      typeof raw.runtimeLabel !== "string" ||
      raw.runtimeLabel.trim() === ""
    ) {
      return undefined;
    }
    return {
      target: "android",
      nativeBridgeVersion: raw.nativeBridgeVersion,
      runtimeLabel: raw.runtimeLabel
    };
  } catch {
    return undefined;
  }
}

export function readClientShellConfig(): ClientShellConfig {
  const candidate = (globalThis as { __NEXUSIM_CLIENT_SHELL__?: unknown }).__NEXUSIM_CLIENT_SHELL__;
  if (typeof candidate !== "object" || candidate === null) {
    return {};
  }
  const raw = candidate as Record<string, unknown>;
  const target = shellTarget(raw.target);
  const output: ClientShellConfig = {};
  assignIfPresent(output, "target", target);
  assignIfPresent(output, "apiBaseURL", stringValue(raw.apiBaseURL));
  assignIfPresent(output, "pushWebSocketURL", stringValue(raw.pushWebSocketURL));
  assignIfPresent(output, "deviceID", stringValue(raw.deviceID));
  assignIfPresent(output, "installationID", stringValue(raw.installationID));
  assignIfPresent(output, "appVersion", stringValue(raw.appVersion));
  assignIfPresent(output, "sessionKey", stringValue(raw.sessionKey));
  return output;
}

function shellTarget(value: unknown): ClientRuntimeTarget | undefined {
  if (value === "browser" || value === "windows-desktop" || value === "android") {
    return value;
  }
  return undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
}

function assignIfPresent<Key extends keyof ClientShellConfig>(
  output: ClientShellConfig,
  key: Key,
  value: ClientShellConfig[Key] | undefined
): void {
  if (value !== undefined) {
    (output as Record<Key, ClientShellConfig[Key]>)[key] = value;
  }
}
