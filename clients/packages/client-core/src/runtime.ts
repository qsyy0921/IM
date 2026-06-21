import type { ClientRuntimeConfig } from "./config";
import { validateRuntimeConfig } from "./config";
import type { ClientPlatformAdapter } from "./platform";
import { AckQueue } from "./ack-queue";
import { AuthSessionManager } from "./auth-session";
import { BFFClient } from "./http-bff-client";
import { InboxSyncEngine } from "./inbox-sync";
import { PushConnectionManager } from "./push-connection";
import { MessageSendQueue } from "./send-queue";
import { WebSocketPushTransport } from "./websocket-push-transport";
import type { AuthSession, LoginRequest } from "@nexusim/protocol";

export interface ClientRuntime {
  config: ClientRuntimeConfig;
  platform: ClientPlatformAdapter;
  bff: BFFClient;
  pushTransport: WebSocketPushTransport;
  auth: AuthSessionManager;
  inboxSync: InboxSyncEngine;
  pushConnection: PushConnectionManager;
  sendQueue: MessageSendQueue;
  ackQueue: AckQueue;
  login(request: LoginRequest): Promise<AuthSession>;
  refresh(): Promise<AuthSession>;
  restoreSession(): Promise<AuthSession | null>;
  logout(): Promise<void>;
}

export interface CreateClientRuntimeOptions {
  config: ClientRuntimeConfig;
  platform: ClientPlatformAdapter;
  inboxPageSize?: number;
  idFactory?: () => string;
  nowMs?: () => number;
}

export function createClientRuntime(options: CreateClientRuntimeOptions): ClientRuntime {
  const config = validateRuntimeConfig(options.config);
  const bff = new BFFClient(config.apiBaseURL);
  const pushTransport = new WebSocketPushTransport();
  const idFactory = options.idFactory ?? defaultIDFactory;
  const nowMs = options.nowMs ?? Date.now;
  const auth = new AuthSessionManager(bff);
  const inboxSync = new InboxSyncEngine({
    deliveryAPI: bff,
    store: options.platform.messageStore,
    pageSize: options.inboxPageSize ?? 64
  });
  const pushConnection = new PushConnectionManager({
    url: config.pushWebSocketURL,
    transport: pushTransport,
    scheduler: inboxSync
  });

  return {
    config,
    platform: options.platform,
    bff,
    pushTransport,
    auth,
    inboxSync,
    pushConnection,
    sendQueue: new MessageSendQueue({
      messagingAPI: bff,
      store: options.platform.messageStore,
      idempotencyKeyFactory: idFactory,
      clientMessageIDFactory: idFactory,
      nowMs
    }),
    ackQueue: new AckQueue({
      deliveryAPI: bff,
      requestIDFactory: idFactory
    }),
    async login(request: LoginRequest): Promise<AuthSession> {
      const session = await auth.login(request);
      await options.platform.secureSessionStore.saveSession(session);
      return session;
    },
    async refresh(): Promise<AuthSession> {
      const session = await auth.refresh();
      await options.platform.secureSessionStore.saveSession(session);
      return session;
    },
    async restoreSession(): Promise<AuthSession | null> {
      const session = await options.platform.secureSessionStore.loadSession();
      auth.hydrate(session);
      return session;
    },
    async logout(): Promise<void> {
      try {
        await auth.logout();
      } finally {
        pushConnection.disconnect();
        await options.platform.secureSessionStore.clearSession();
        await options.platform.messageStore.clear();
      }
    }
  };
}

function defaultIDFactory(): string {
  if (globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
