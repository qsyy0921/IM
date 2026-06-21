import type {
  AppLifecyclePort,
  ClientPlatformAdapter,
  ClientRuntimeConfig,
  ClientRuntimeTarget,
  ConnectivityState,
  LocalMessageStore,
  NetworkStatePort,
  NativeStringKeyValueBridge,
  SecureSessionStore,
  WakeupNotificationPort
} from "@nexusim/client-core";
import { KeyValueMessageStore, NativeBridgeStringKeyValueStorage } from "@nexusim/client-core";
import type { AuthSession } from "@nexusim/protocol";
import { IndexedDBMessageStore } from "./adapters/indexeddb-message-store";

export interface BrowserPlatformAdapterOptions {
  config: ClientRuntimeConfig;
  target?: ClientRuntimeTarget;
  appVersion?: string;
  installationID?: string;
  sessionKey?: string;
  messageStore?: LocalMessageStore;
  nativeStorageBridge?: NativeStringKeyValueBridge;
}

export type BrowserPlatformAdapter = ClientPlatformAdapter & {
  identity: ClientPlatformAdapter["identity"] & {
    target: ClientRuntimeTarget;
  };
  messageStore: LocalMessageStore;
};

export function createBrowserPlatformAdapter(
  options: BrowserPlatformAdapterOptions
): BrowserPlatformAdapter {
  const target = options.target ?? "browser";
  return {
    identity: {
      target,
      deviceID: options.config.deviceID,
      installationID: options.installationID ?? `${target}:${options.config.deviceID}`,
      appVersion: options.appVersion ?? "0.1.0"
    },
    secureSessionStore: new BrowserSessionStore(options.sessionKey),
    messageStore: options.messageStore ?? messageStoreForTarget(target, options.config, options.nativeStorageBridge),
    networkState: browserNetworkState(),
    lifecycle: browserLifecycle(),
    wakeupNotifications: unsupportedWakeupNotifications()
  };
}

function messageStoreForTarget(
  target: ClientRuntimeTarget,
  config: ClientRuntimeConfig,
  nativeStorageBridge?: NativeStringKeyValueBridge
): LocalMessageStore {
  if (nativeStorageBridge) {
    return new KeyValueMessageStore(
      new NativeBridgeStringKeyValueStorage(nativeStorageBridge),
      { namespace: `${target}:${config.deviceID}` }
    );
  }
  return new IndexedDBMessageStore();
}

export class BrowserSessionStore implements SecureSessionStore {
  readonly #key: string;
  #memorySession: AuthSession | null = null;

  constructor(key = "nexusim:web:session") {
    this.#key = key;
  }

  async loadSession(): Promise<AuthSession | null> {
    if (!hasSessionStorage()) {
      return this.#memorySession ? { ...this.#memorySession } : null;
    }
    const raw = globalThis.sessionStorage.getItem(this.#key);
    if (!raw) {
      return null;
    }
    try {
      const parsed = JSON.parse(raw) as unknown;
      return isAuthSession(parsed) ? parsed : null;
    } catch {
      return null;
    }
  }

  async saveSession(session: AuthSession): Promise<void> {
    const snapshot = { ...session };
    this.#memorySession = snapshot;
    if (hasSessionStorage()) {
      globalThis.sessionStorage.setItem(this.#key, JSON.stringify(snapshot));
    }
  }

  async clearSession(): Promise<void> {
    this.#memorySession = null;
    if (hasSessionStorage()) {
      globalThis.sessionStorage.removeItem(this.#key);
    }
  }
}

function browserNetworkState(): NetworkStatePort {
  return {
    async current(): Promise<ConnectivityState> {
      if (typeof navigator === "undefined") {
        return "UNKNOWN";
      }
      return navigator.onLine ? "ONLINE" : "OFFLINE";
    },
    subscribe(listener: (state: ConnectivityState) => void): () => void {
      if (typeof window === "undefined") {
        listener("UNKNOWN");
        return () => undefined;
      }
      const emit = () => listener(navigator.onLine ? "ONLINE" : "OFFLINE");
      window.addEventListener("online", emit);
      window.addEventListener("offline", emit);
      emit();
      return () => {
        window.removeEventListener("online", emit);
        window.removeEventListener("offline", emit);
      };
    }
  };
}

function browserLifecycle(): AppLifecyclePort {
  return {
    current(): "ACTIVE" | "BACKGROUND" {
      if (typeof document === "undefined") {
        return "ACTIVE";
      }
      return document.hidden ? "BACKGROUND" : "ACTIVE";
    },
    subscribe(listener: (state: "ACTIVE" | "BACKGROUND" | "SUSPENDED") => void): () => void {
      if (typeof document === "undefined") {
        listener("ACTIVE");
        return () => undefined;
      }
      const emit = () => listener(document.hidden ? "BACKGROUND" : "ACTIVE");
      document.addEventListener("visibilitychange", emit);
      emit();
      return () => document.removeEventListener("visibilitychange", emit);
    }
  };
}

function unsupportedWakeupNotifications(): WakeupNotificationPort {
  return {
    async requestPermission(): Promise<"UNSUPPORTED"> {
      return "UNSUPPORTED";
    },
    async showLocalConversationWakeup(): Promise<void> {
      return;
    }
  };
}

function hasSessionStorage(): boolean {
  try {
    return typeof globalThis.sessionStorage !== "undefined";
  } catch {
    return false;
  }
}

function isAuthSession(value: unknown): value is AuthSession {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Partial<AuthSession>;
  return (
    typeof candidate.tenantID === "string" &&
    typeof candidate.userID === "string" &&
    typeof candidate.deviceID === "string" &&
    typeof candidate.sessionID === "string" &&
    typeof candidate.accessToken === "string"
  );
}
