import type {
  ClientRuntimeConfig,
  ClientRuntimeTarget,
  NativeStringKeyValueBridge,
  NativeStoreReadiness
} from "@nexusim/client-core";

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
  readonly smokeCallbackURL?: string;
  readonly smokeRunID?: string;
  readonly smokeMode?: "metadata";
}

export interface AndroidNativeBridgeMetadata {
  readonly target: "android";
  readonly nativeBridgeVersion: string;
  readonly runtimeLabel: string;
  readonly capabilities?: NativeBridgeCapabilities;
}

export interface DesktopNativeBridgeMetadata {
  readonly target: "windows-desktop";
  readonly nativeBridgeVersion: string;
  readonly runtimeLabel: string;
  readonly capabilities?: NativeBridgeCapabilities;
}

export type NativeBridgeMetadata = AndroidNativeBridgeMetadata | DesktopNativeBridgeMetadata;

export interface NativeBridgeCapabilities {
  readonly localStore?: NativeStoreReadiness;
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

export async function readDesktopNativeBridgeMetadata(): Promise<DesktopNativeBridgeMetadata | undefined> {
  const tauri = (globalThis as {
    __TAURI__?: {
      core?: {
        invoke?: (command: string) => Promise<unknown>;
      };
    };
  }).__TAURI__;
  if (!tauri?.core || typeof tauri.core.invoke !== "function") {
    return undefined;
  }
  try {
    const response = await tauri.core.invoke("runtime_metadata");
    if (typeof response !== "string") {
      return undefined;
    }
    const raw = JSON.parse(response) as Record<string, unknown>;
    if (
      raw.target !== "windows-desktop" ||
      typeof raw.nativeBridgeVersion !== "string" ||
      raw.nativeBridgeVersion.trim() === "" ||
      typeof raw.runtimeLabel !== "string" ||
      raw.runtimeLabel.trim() === ""
    ) {
      return undefined;
    }
    const capabilities = nativeBridgeCapabilities("windows-desktop", raw.capabilities);
    return {
      target: "windows-desktop",
      nativeBridgeVersion: raw.nativeBridgeVersion,
      runtimeLabel: raw.runtimeLabel,
      ...(capabilities ? { capabilities } : {})
    };
  } catch {
    return undefined;
  }
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
    const capabilities = nativeBridgeCapabilities("android", raw.capabilities);
    return {
      target: "android",
      nativeBridgeVersion: raw.nativeBridgeVersion,
      runtimeLabel: raw.runtimeLabel,
      ...(capabilities ? { capabilities } : {})
    };
  } catch {
    return undefined;
  }
}

export function readDesktopNativeStorageBridge(
  metadata?: DesktopNativeBridgeMetadata
): NativeStringKeyValueBridge | undefined {
  const tauri = (globalThis as {
    __TAURI__?: {
      core?: {
        invoke?: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
      };
    };
  }).__TAURI__;
  if (
    metadata?.capabilities?.localStore?.ready !== true ||
    metadata.capabilities.localStore.bridge !== "tauri-sqlite" ||
    !tauri?.core ||
    typeof tauri.core.invoke !== "function"
  ) {
    return undefined;
  }
  const invoke = tauri.core.invoke;
  return {
    async getItem(key: string): Promise<string | null> {
      const value = await invoke("local_store_get_item", { key });
      return typeof value === "string" ? value : null;
    },
    async setItem(key: string, value: string): Promise<void> {
      await invoke("local_store_set_item", { key, value });
    },
    async removeItem(key: string): Promise<void> {
      await invoke("local_store_remove_item", { key });
    }
  };
}

export function readAndroidNativeStorageBridge(
  metadata?: AndroidNativeBridgeMetadata
): NativeStringKeyValueBridge | undefined {
  const bridge = (globalThis as {
    NexusIMNative?: {
      localStoreGetItem?: (key: string) => string | null;
      localStoreSetItem?: (key: string, value: string) => void;
      localStoreRemoveItem?: (key: string) => void;
    };
  }).NexusIMNative;
  if (
    metadata?.capabilities?.localStore?.ready !== true ||
    metadata.capabilities.localStore.bridge !== "android-sqlite" ||
    !bridge ||
    typeof bridge.localStoreGetItem !== "function" ||
    typeof bridge.localStoreSetItem !== "function" ||
    typeof bridge.localStoreRemoveItem !== "function"
  ) {
    return undefined;
  }
  const getItem = bridge.localStoreGetItem;
  const setItem = bridge.localStoreSetItem;
  const removeItem = bridge.localStoreRemoveItem;
  return {
    getItem(key: string): string | null {
      const value = getItem(key);
      return typeof value === "string" ? value : null;
    },
    setItem(key: string, value: string): void {
      setItem(key, value);
    },
    removeItem(key: string): void {
      removeItem(key);
    }
  };
}

function nativeBridgeCapabilities(
  target: NativeBridgeMetadata["target"],
  value: unknown
): NativeBridgeCapabilities | undefined {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const raw = value as Record<string, unknown>;
  const localStore = nativeLocalStoreReadiness(target, raw.localStore);
  if (!localStore) {
    return undefined;
  }
  return { localStore };
}

function nativeLocalStoreReadiness(
  target: NativeBridgeMetadata["target"],
  value: unknown
): NativeStoreReadiness | undefined {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const raw = value as Record<string, unknown>;
  const bridge = target === "windows-desktop" ? "tauri-sqlite" : "android-sqlite";
  const ready = raw.nativeStoreReady === true;
  const expectedReason = ready ? "" : "sqlite-native-bridge-unavailable";
  if (
    raw.currentDefault !== "local-storage" ||
    raw.productionTarget !== "sqlite" ||
    typeof raw.nativeStoreReady !== "boolean" ||
    raw.nativeStoreReason !== expectedReason ||
    raw.nativeStoreBridge !== bridge
  ) {
    return undefined;
  }
  return {
    target,
    requestedStore: "sqlite",
    ready,
    reason: ready ? "" : "sqlite-native-bridge-unavailable",
    bridge,
    nextAction: ready ? "" : `${bridge} is required before ${target} can use sqlite local store`
  };
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
  assignIfPresent(output, "smokeCallbackURL", loopbackHTTPURL(raw.smokeCallbackURL));
  assignIfPresent(output, "smokeRunID", stringValue(raw.smokeRunID));
  assignIfPresent(output, "smokeMode", smokeMode(raw.smokeMode));
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

function loopbackHTTPURL(value: unknown): string | undefined {
  if (typeof value !== "string" || value.trim() === "") {
    return undefined;
  }
  try {
    const url = new URL(value);
    if (
      url.protocol === "http:" &&
      (url.hostname === "127.0.0.1" || url.hostname === "localhost" || url.hostname === "[::1]")
    ) {
      return url.toString();
    }
  } catch {
    return undefined;
  }
  return undefined;
}

function smokeMode(value: unknown): "metadata" | undefined {
  return value === "metadata" ? "metadata" : undefined;
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
