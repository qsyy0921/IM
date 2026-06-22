import { useEffect, useMemo, useRef, useState } from "react";
import { createClientRuntime, createClientShellActions, validateRuntimeConfig } from "@nexusim/client-core";
import type { AuthSession, ConversationSummary, DeliveryNotifyFrame, MessageItem, ServerFrame } from "@nexusim/protocol";
import { createBrowserPlatformAdapter } from "./platform-adapter";
import type { BrowserPlatformAdapterOptions } from "./platform-adapter";
import {
  loadRuntimeConfig,
  readAndroidNativeBridgeMetadata,
  readAndroidNativeStorageBridge,
  readClientShellConfig,
  readDesktopNativeBridgeMetadata
} from "./runtime-config";
import type { NativeBridgeMetadata } from "./runtime-config";
import {
  buildShellSmokeMetadataReport,
  postShellSmokeMetadataReport,
  shouldReportShellSmokeMetadata
} from "./shell-smoke-report";

const runtimeConfig = validateRuntimeConfig(loadRuntimeConfig());
const shellConfig = readClientShellConfig();
const androidNativeMetadata = readAndroidNativeBridgeMetadata();

export function App() {
  const platform = useMemo(
    () => createBrowserPlatformAdapter(browserPlatformOptions()),
    []
  );
  const runtime = useMemo(
    () =>
      createClientRuntime({
        config: runtimeConfig,
        platform,
        idFactory: newID,
        nowMs: () => Date.now()
      }),
    [platform]
  );
  const shellActions = useMemo(() => createClientShellActions(runtime), [runtime]);
  const store = platform.messageStore;

  const [tenantID, setTenantID] = useState("tenant-local");
  const [userID, setUserID] = useState("user-a");
  const [password, setPassword] = useState("password");
  const [session, setSession] = useState<AuthSession | null>(null);
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [activeConversationID, setActiveConversationID] = useState("");
  const [manualConversationID, setManualConversationID] = useState("conv-demo");
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [composerText, setComposerText] = useState("");
  const [status, setStatus] = useState("ready");
  const [pushStatus, setPushStatus] = useState("disconnected");
  const [lastAck, setLastAck] = useState<{ conversationID: string; seq: number } | null>(null);
  const [error, setError] = useState("");
  const [desktopNativeMetadata, setDesktopNativeMetadata] = useState<NativeBridgeMetadata | undefined>();
  const [desktopNativeMetadataProbeFinished, setDesktopNativeMetadataProbeFinished] = useState(false);

  const sessionRef = useRef<AuthSession | null>(null);
  const activeConversationRef = useRef("");
  const pushConnectionRef = useRef<{ close(): void } | null>(null);
  const shellSmokeReportedRef = useRef(false);

  useEffect(() => {
    let active = true;
    let retryTimer: ReturnType<typeof window.setTimeout> | undefined;
    const deadlineMs = Date.now() + 5000;
    const readMetadata = async () => {
      const metadata = await readDesktopNativeBridgeMetadata();
      if (!active) {
        return;
      }
      if (metadata || Date.now() >= deadlineMs) {
        setDesktopNativeMetadata(metadata);
        setDesktopNativeMetadataProbeFinished(true);
        return;
      }
      retryTimer = window.setTimeout(() => void readMetadata(), 100);
    };
    void readMetadata();
    return () => {
      active = false;
      if (retryTimer !== undefined) {
        window.clearTimeout(retryTimer);
      }
    };
  }, []);

  useEffect(() => {
    if (!shouldReportShellSmokeMetadata(shellConfig) || shellSmokeReportedRef.current) {
      return;
    }
    const nativeMetadata = desktopNativeMetadata ?? androidNativeMetadata;
    if (shellConfig.target === "windows-desktop" && !nativeMetadata && !desktopNativeMetadataProbeFinished) {
      return;
    }
    if (shellConfig.target === "android" && !nativeMetadata) {
      return;
    }
    shellSmokeReportedRef.current = true;
    const report = buildShellSmokeMetadataReport(shellConfig, runtimeConfig, nativeMetadata);
    void postShellSmokeMetadataReport(shellConfig.smokeCallbackURL!, report).catch(caught => {
      shellSmokeReportedRef.current = false;
      setError(errorMessage(caught));
    });
  }, [desktopNativeMetadata, desktopNativeMetadataProbeFinished]);

  async function login(): Promise<void> {
    await run("login", async () => {
      pushConnectionRef.current?.close();
      const loginState = await shellActions.login({
        tenantID,
        userID,
        password,
        deviceID: runtimeConfig.deviceID
      });
      const nextSession = runtime.auth.current();
      if (!loginState.authenticated || !nextSession) {
        throw new Error("login did not create session");
      }
      sessionRef.current = nextSession;
      setSession(nextSession);
      await connectPush(nextSession);
      await loadConversations(nextSession);
    });
  }

  async function logout(): Promise<void> {
    await run("logout", async () => {
      try {
        await shellActions.logout();
      } finally {
        pushConnectionRef.current?.close();
        pushConnectionRef.current = null;
        sessionRef.current = null;
        activeConversationRef.current = "";
        setSession(null);
        setConversations([]);
        setActiveConversationID("");
        setMessages([]);
        setComposerText("");
        setPushStatus("disconnected");
        await store.clear();
      }
    });
  }

  async function refreshSession(): Promise<void> {
    await run("refresh session", async () => {
      if (!sessionRef.current) {
        throw new Error("login first");
      }
      pushConnectionRef.current?.close();
      const refreshState = await shellActions.refresh();
      const refreshedSession = runtime.auth.current();
      if (!refreshState.authenticated || !refreshedSession) {
        throw new Error("refresh did not create session");
      }
      sessionRef.current = refreshedSession;
      setSession(refreshedSession);
      await connectPush(refreshedSession);
      await loadConversations(refreshedSession);
    });
  }

  async function restoreSession(): Promise<void> {
    await run("restore session", async () => {
      const state = await shellActions.restoreSession();
      const restoredSession = runtime.auth.current();
      if (!state.authenticated || !restoredSession) {
        throw new Error("no saved session");
      }
      sessionRef.current = restoredSession;
      setSession(restoredSession);
      await connectPush(restoredSession);
      await loadConversations(restoredSession);
    });
  }

  async function loadConversations(currentSession = sessionRef.current): Promise<void> {
    if (!currentSession) {
      throw new Error("login first");
    }
    const items = await runtime.bff.listConversations(currentSession, { limit: 50 });
    setConversations(items);
    if (items.length > 0) {
      const firstConversationID = items[0]!.conversationID;
      await selectConversation(firstConversationID, currentSession);
    }
  }

  async function selectConversation(conversationID: string, currentSession = sessionRef.current): Promise<void> {
    if (!currentSession) {
      throw new Error("login first");
    }
    activeConversationRef.current = conversationID;
    setActiveConversationID(conversationID);
    await syncConversation(conversationID, currentSession);
  }

  async function openManualConversation(): Promise<void> {
    await run("open conversation", async () => {
      const conversationID = manualConversationID.trim();
      if (!conversationID) {
        throw new Error("conversation id is required");
      }
      await selectConversation(conversationID);
    });
  }

  async function sendMessage(): Promise<void> {
    await run("send message", async () => {
      const currentSession = requireSession();
      const conversationID = activeConversationID || manualConversationID.trim();
      if (!conversationID) {
        throw new Error("conversation id is required");
      }
      const text = composerText.trim();
      if (!text) {
        return;
      }
      setComposerText("");
      await runtime.sendQueue.sendText({ session: currentSession, conversationID, text });
      await syncConversation(conversationID, currentSession);
    });
  }

  async function connectPush(currentSession: AuthSession): Promise<void> {
    setPushStatus("connecting");
    const connection = await runtime.pushTransport.connect({
      url: runtimeConfig.pushWebSocketURL,
      session: currentSession,
      onFrame: frame => void handlePushFrame(frame),
      onClose: reason => {
        pushConnectionRef.current = null;
        setPushStatus(reason);
      }
    });
    pushConnectionRef.current = connection;
  }

  async function handlePushFrame(frame: ServerFrame): Promise<void> {
    if (frame.op === "server.hello") {
      setPushStatus("connected");
      return;
    }
    if (frame.op === "delivery.notify") {
      await syncConversation((frame as DeliveryNotifyFrame).conversation_id);
      return;
    }
    if (frame.op === "server.resume_hint") {
      const cursors = frame.conversations ?? [];
      if (cursors.length === 0 && activeConversationRef.current) {
        await syncConversation(activeConversationRef.current);
        return;
      }
      for (const cursor of cursors) {
        await syncConversation(cursor.conversation_id);
      }
      return;
    }
    if (frame.op === "error") {
      setError(`${frame.code}: ${frame.message}`);
    }
  }

  async function syncConversation(conversationID: string, currentSession = sessionRef.current): Promise<void> {
    if (!currentSession) {
      return;
    }
    const afterSeq = await store.getLastReceivedSeq(conversationID);
    const response = await runtime.bff.pullInbox(
      {
        tenantID: currentSession.tenantID,
        userID: currentSession.userID,
        deviceID: currentSession.deviceID,
        conversationID,
        afterSeq,
        limit: 50
      },
      currentSession
    );
    await store.upsertMessages(response.items);
    const nextMessages = await store.listMessages(conversationID);
    if (activeConversationRef.current === conversationID || !activeConversationRef.current) {
      setMessages(nextMessages);
    }
    const maxSeq = nextMessages.reduce((max, message) => Math.max(max, message.conversationSeq), afterSeq);
    if (maxSeq > afterSeq) {
      runtime.ackQueue.recordReceived(conversationID, maxSeq);
      await runtime.ackQueue.flush(currentSession);
      setLastAck({ conversationID, seq: maxSeq });
    }
  }

  async function run(label: string, task: () => Promise<void>): Promise<void> {
    setStatus(label);
    setError("");
    try {
      await task();
      setStatus(`${label} ok`);
    } catch (caught) {
      setStatus(`${label} failed`);
      setError(errorMessage(caught));
    }
  }

  function requireSession(): AuthSession {
    const currentSession = sessionRef.current ?? session;
    if (!currentSession) {
      throw new Error("login first");
    }
    return currentSession;
  }

  const activeConversation = conversations.find(item => item.conversationID === activeConversationID);
  const nativeMetadata = desktopNativeMetadata ?? androidNativeMetadata;

  return (
    <main className="shell">
      <nav className="app-rail" aria-label="NexusIM">
        <div className="brand-mark">N</div>
        <button className="rail-button active" type="button" aria-label="会话">会</button>
        <button className="rail-button" type="button" aria-label="联系人">人</button>
        <button className="rail-button" type="button" aria-label="设置">设</button>
      </nav>

      <aside className="conversation-pane">
        <header className="pane-header">
          <div>
            <h1>NexusIM</h1>
            <p>{session ? userID : "账号密码登录"}</p>
          </div>
          <button
            data-testid="refresh-conversations"
            className="icon-button"
            type="button"
            onClick={() => void run("load conversations", () => loadConversations())}
            aria-label="刷新会话"
          >
            ↻
          </button>
        </header>

        <section className={`login-card ${session ? "compact" : ""}`}>
          <input
            data-testid="login-tenant"
            className="automation-input"
            aria-hidden="true"
            tabIndex={-1}
            value={tenantID}
            onChange={event => setTenantID(event.target.value)}
          />
          <label>
            账号
            <input
              data-testid="login-user"
              autoComplete="username"
              value={userID}
              onChange={event => setUserID(event.target.value)}
              disabled={!!session}
            />
          </label>
          <label>
            密码
            <input
              data-testid="login-password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={event => setPassword(event.target.value)}
              disabled={!!session}
            />
          </label>
          <div className="login-actions">
            <button data-testid="login-submit" type="button" onClick={() => void login()} disabled={!!session}>
              登录
            </button>
            <button
              data-testid="logout-submit"
              className="secondary-button"
              type="button"
              onClick={() => void logout()}
              disabled={!session}
            >
              退出
            </button>
          </div>
          <button
            data-testid="restore-session"
            className="text-button"
            type="button"
            onClick={() => void restoreSession()}
            disabled={!!session}
          >
            恢复上次登录
          </button>
          <button
            data-testid="refresh-session"
            className="automation-button"
            type="button"
            onClick={() => void refreshSession()}
            disabled={!session}
          >
            刷新登录态
          </button>
        </section>

        <section className="conversation-list" aria-label="会话列表">
          {conversations.length === 0 ? (
            <div className="conversation-empty">暂无会话</div>
          ) : null}
          {conversations.map(conversation => (
            <button
              data-testid="conversation-item"
              className={`conversation-item ${conversation.conversationID === activeConversationID ? "active" : ""}`}
              key={conversation.conversationID}
              onClick={() => void run("select conversation", () => selectConversation(conversation.conversationID))}
            >
              <span className="conversation-avatar">{conversation.title.slice(0, 1).toUpperCase()}</span>
              <span className="conversation-copy">
                <strong>{conversation.title}</strong>
                <small>最新 #{conversation.lastSeq}</small>
              </span>
            </button>
          ))}
        </section>

        <div className="automation-controls" aria-hidden="true">
          <input
            data-testid="conversation-id-input"
            value={manualConversationID}
            onChange={event => setManualConversationID(event.target.value)}
            tabIndex={-1}
          />
          <button data-testid="open-conversation" type="button" onClick={() => void openManualConversation()} tabIndex={-1}>
            打开
          </button>
        </div>
      </aside>

      <section className="chat">
        <header className="chat-header">
          <div className="chat-title">
            <h2>{(activeConversation?.title ?? activeConversationID) || "选择一个会话"}</h2>
            <p>{session ? "在线" : "未登录"}</p>
          </div>
          <div className="status-stack" aria-label="连接状态">
            <span data-testid="runtime-status" className="status-pill">{status}</span>
            <span data-testid="push-status" className="status-pill neutral">push {pushStatus}</span>
            <span data-testid="ack-status" className="status-pill neutral">
              {lastAck ? `ack ${lastAck.conversationID} #${lastAck.seq}` : "ack none"}
            </span>
          </div>
        </header>

        {error ? <div data-testid="error-banner" className="error-banner">{error}</div> : null}

        <div data-testid="message-list" className="messages">
          {messages.length === 0 ? (
            <div className="empty-state">
              <strong>{session ? "还没有消息" : "登录后查看消息"}</strong>
              <span>{session ? "选择会话后即可收发文本消息。" : "输入账号和密码即可进入 NexusIM。"}</span>
            </div>
          ) : (
            messages.map(message => {
              const isMine = session?.userID === message.senderUserID;
              return (
                <article
                  data-testid="message-item"
                  className={`message ${isMine ? "mine" : ""}`}
                  key={message.messageID || message.clientMessageID}
                >
                  <header>
                    <strong>{message.senderUserID}</strong>
                    <span>#{message.conversationSeq || "pending"}</span>
                  </header>
                  <p>{message.text}</p>
                  <footer>{message.status}</footer>
                </article>
              );
            })
          )}
        </div>

        <form
          className="composer"
          onSubmit={event => {
            event.preventDefault();
            void sendMessage();
          }}
        >
          <input
            data-testid="message-composer"
            placeholder="输入消息"
            value={composerText}
            onChange={event => setComposerText(event.target.value)}
            disabled={!session || !activeConversationID}
          />
          <button
            data-testid="send-message"
            type="submit"
            disabled={!session || !activeConversationID || composerText.trim() === ""}
          >
            发送
          </button>
        </form>
      </section>

      <div className="system-probes" aria-hidden="true">
        {nativeMetadata?.capabilities?.localStore ? (
          <span data-testid="native-store-readiness">
            {nativeLocalStoreStatus(nativeMetadata.capabilities.localStore)}
          </span>
        ) : null}
        <span>{runtimeConfig.apiBaseURL}</span>
        <span>{runtimeConfig.pushWebSocketURL}</span>
        <span>{runtimeConfig.deviceID}</span>
        <span>{shellConfig.target ?? "browser"}</span>
      </div>
    </main>
  );
}

function browserPlatformOptions(): BrowserPlatformAdapterOptions {
  const options: BrowserPlatformAdapterOptions = { config: runtimeConfig };
  if (shellConfig.target) {
    options.target = shellConfig.target;
  }
  if (shellConfig.appVersion) {
    options.appVersion = shellConfig.appVersion;
  }
  if (shellConfig.installationID) {
    options.installationID = shellConfig.installationID;
  }
  if (shellConfig.sessionKey) {
    options.sessionKey = shellConfig.sessionKey;
  }
  if (shellConfig.target === "android") {
    const nativeStorageBridge = readAndroidNativeStorageBridge(androidNativeMetadata);
    if (nativeStorageBridge) {
      options.nativeStorageBridge = nativeStorageBridge;
    }
  }
  return options;
}

function newID(): string {
  if (globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function nativeLocalStoreStatus(localStore: NonNullable<NativeBridgeMetadata["capabilities"]>["localStore"]): string {
  if (!localStore) {
    return "local-storage";
  }
  const readiness = localStore.ready ? "ready" : localStore.reason;
  return `local-storage -> ${localStore.requestedStore}; ${readiness}; ${localStore.bridge}`;
}

function errorMessage(error: unknown): string {
  if (typeof error === "object" && error !== null && "message" in error) {
    return String((error as { message: unknown }).message);
  }
  return String(error);
}
