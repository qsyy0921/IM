import type { AuthSession, ConversationID } from "@nexusim/protocol";
import type { LocalMessageStore } from "./ports";

export type ClientRuntimeTarget = "browser" | "windows-desktop" | "android";

export type ConnectivityState = "ONLINE" | "OFFLINE" | "CAPTIVE" | "UNKNOWN";

export interface SecureSessionStore {
  loadSession(): Promise<AuthSession | null>;
  saveSession(session: AuthSession): Promise<void>;
  clearSession(): Promise<void>;
}

export interface RuntimeDeviceIdentity {
  target: ClientRuntimeTarget;
  deviceID: string;
  installationID: string;
  appVersion: string;
}

export interface NetworkStatePort {
  current(): Promise<ConnectivityState>;
  subscribe(listener: (state: ConnectivityState) => void): () => void;
}

export interface AppLifecyclePort {
  current(): "ACTIVE" | "BACKGROUND" | "SUSPENDED";
  subscribe(listener: (state: "ACTIVE" | "BACKGROUND" | "SUSPENDED") => void): () => void;
}

export interface WakeupNotificationPort {
  requestPermission(): Promise<"GRANTED" | "DENIED" | "UNSUPPORTED">;
  registerDevicePushToken?(): Promise<string | null>;
  showLocalConversationWakeup(input: {
    conversationID: ConversationID;
    title: string;
    body: string;
  }): Promise<void>;
}

export interface ClientPlatformAdapter {
  identity: RuntimeDeviceIdentity;
  secureSessionStore: SecureSessionStore;
  messageStore: LocalMessageStore;
  networkState: NetworkStatePort;
  lifecycle: AppLifecyclePort;
  wakeupNotifications: WakeupNotificationPort;
}
