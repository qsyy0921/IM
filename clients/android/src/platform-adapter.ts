import type {
  AppLifecyclePort,
  ConnectivityState,
  LocalMessageStore,
  NetworkStatePort,
  NativeStringKeyValueBridge,
  WakeupNotificationPort
} from "@nexusim/client-core";
import { assertNativeStoreReady, NativeBridgeStringKeyValueStorage } from "@nexusim/client-core";
import type { AndroidPlatformAdapter, AndroidRuntimeConfig } from "./platform-contract";
import { AndroidDevelopmentSessionStore } from "./development-session-store";
import { AndroidMemoryMessageStore } from "./memory-message-store";
import { AndroidPersistentMessageStore } from "./persistent-message-store";
import { loadAndroidRuntimeConfig } from "./runtime-config";

export interface AndroidPlatformAdapterOptions {
  config?: AndroidRuntimeConfig;
  appVersion?: string;
  installationID?: string;
  initialNetworkState?: ConnectivityState;
  messageStore?: LocalMessageStore;
  nativeStorageBridge?: NativeStringKeyValueBridge;
}

export function createAndroidPlatformAdapter(
  options: AndroidPlatformAdapterOptions = {}
): AndroidPlatformAdapter {
  const config = options.config ?? loadAndroidRuntimeConfig();
  return {
    identity: {
      target: "android",
      deviceID: config.deviceID,
      installationID: options.installationID ?? `android:${config.deviceID}`,
      appVersion: options.appVersion ?? "0.1.0"
    },
    secureSessionStore: new AndroidDevelopmentSessionStore(),
    messageStore: options.messageStore ?? androidMessageStore(config, options.nativeStorageBridge),
    networkState: staticNetworkState(options.initialNetworkState ?? "UNKNOWN"),
    lifecycle: staticLifecycle(),
    wakeupNotifications: androidWakeupNotifications(config.notificationProvider)
  };
}

function androidMessageStore(
  config: AndroidRuntimeConfig,
  nativeStorageBridge?: NativeStringKeyValueBridge
): LocalMessageStore {
  if (config.localStore === "memory") {
    return new AndroidMemoryMessageStore();
  }
  if (config.localStore === "sqlite") {
    assertNativeStoreReady({
      target: "android",
      requestedStore: "sqlite",
      nativeBridgeAvailable: Boolean(nativeStorageBridge)
    });
    return new AndroidPersistentMessageStore({
      namespace: `${config.platform}:${config.deviceID}`,
      storage: new NativeBridgeStringKeyValueStorage(nativeStorageBridge!)
    });
  }
  return new AndroidPersistentMessageStore({
    namespace: `${config.platform}:${config.deviceID}`
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

function androidWakeupNotifications(
  provider: AndroidRuntimeConfig["notificationProvider"]
): WakeupNotificationPort {
  return {
    async requestPermission(): Promise<"UNSUPPORTED"> {
      void provider;
      return "UNSUPPORTED";
    },
    async showLocalConversationWakeup(): Promise<void> {
      return;
    }
  };
}
