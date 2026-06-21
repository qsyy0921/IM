import type {
  AppLifecyclePort,
  ConnectivityState,
  LocalMessageStore,
  NetworkStatePort,
  NativeStringKeyValueBridge,
  WakeupNotificationPort
} from "@nexusim/client-core";
import { assertNativeStoreReady, NativeBridgeStringKeyValueStorage } from "@nexusim/client-core";
import type { DesktopPlatformAdapter, DesktopRuntimeConfig } from "./platform-contract";
import { DesktopDevelopmentSessionStore } from "./development-session-store";
import { DesktopMemoryMessageStore } from "./memory-message-store";
import { DesktopPersistentMessageStore } from "./persistent-message-store";
import { loadDesktopRuntimeConfig } from "./runtime-config";

export interface DesktopPlatformAdapterOptions {
  config?: DesktopRuntimeConfig;
  appVersion?: string;
  installationID?: string;
  initialNetworkState?: ConnectivityState;
  messageStore?: LocalMessageStore;
  nativeStorageBridge?: NativeStringKeyValueBridge;
}

export function createDesktopPlatformAdapter(
  options: DesktopPlatformAdapterOptions = {}
): DesktopPlatformAdapter {
  const config = options.config ?? loadDesktopRuntimeConfig();
  return {
    identity: {
      target: "windows-desktop",
      deviceID: config.deviceID,
      installationID:
        options.installationID ?? `desktop:${config.os}:${config.deviceID}`,
      appVersion: options.appVersion ?? "0.1.0"
    },
    secureSessionStore: new DesktopDevelopmentSessionStore(),
    messageStore: options.messageStore ?? desktopMessageStore(config, options.nativeStorageBridge),
    networkState: staticNetworkState(options.initialNetworkState ?? "UNKNOWN"),
    lifecycle: staticLifecycle(),
    wakeupNotifications: unsupportedWakeupNotifications()
  };
}

function desktopMessageStore(
  config: DesktopRuntimeConfig,
  nativeStorageBridge?: NativeStringKeyValueBridge
): LocalMessageStore {
  if (config.localStore === "memory") {
    return new DesktopMemoryMessageStore();
  }
  if (config.localStore === "sqlite") {
    assertNativeStoreReady({
      target: "windows-desktop",
      requestedStore: "sqlite",
      nativeBridgeAvailable: Boolean(nativeStorageBridge)
    });
    return new DesktopPersistentMessageStore({
      namespace: `${config.os}:${config.deviceID}`,
      storage: new NativeBridgeStringKeyValueStorage(nativeStorageBridge!)
    });
  }
  return new DesktopPersistentMessageStore({
    namespace: `${config.os}:${config.deviceID}`
  });
}

function staticNetworkState(initial: ConnectivityState): NetworkStatePort {
  let state = initial;
  return {
    async current(): Promise<ConnectivityState> {
      return state;
    },
    subscribe(listener: (state: ConnectivityState) => void): () => void {
      listener(state);
      return () => {
        state = initial;
      };
    }
  };
}

function staticLifecycle(): AppLifecyclePort {
  return {
    current(): "ACTIVE" {
      return "ACTIVE";
    },
    subscribe(listener: (state: "ACTIVE" | "BACKGROUND" | "SUSPENDED") => void): () => void {
      listener("ACTIVE");
      return () => undefined;
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
