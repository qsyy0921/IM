import { useEffect, useMemo, useRef, useState } from "react";
import { createClientRuntime, createClientShellActions, validateRuntimeConfig } from "@nexusim/client-core";
import type { AuthSession, ConversationSummary, DeliveryNotifyFrame, MessageItem, ServerFrame } from "@nexusim/protocol";
import { createBrowserPlatformAdapter } from "./platform-adapter";
import type { BrowserPlatformAdapterOptions } from "./platform-adapter";
import {
  loadRuntimeConfig,
  readAndroidNativeBridgeMetadata,
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
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">N</span>
          <div>
            <h1>NexusIM</h1>
            <p>Client Platform MVP</p>
          </div>
        </div>

        <section className="panel">
          <h2>运行入口</h2>
          <dl className="config-list">
            <div>
              <dt>API</dt>
              <dd>{runtimeConfig.apiBaseURL}</dd>
            </div>
            <div>
              <dt>WebSocket</dt>
              <dd>{runtimeConfig.pushWebSocketURL}</dd>
            </div>
            <div>
              <dt>Device</dt>
              <dd>{runtimeConfig.deviceID}</dd>
            </div>
            <div>
              <dt>Target</dt>
              <dd>{shellConfig.target ?? "browser"}</dd>
            </div>
            {nativeMetadata ? (
              <div>
                <dt>Native</dt>
                <dd>{`${nativeMetadata.runtimeLabel} ${nativeMetadata.nativeBridgeVersion}`}</dd>
              </div>
            ) : null}
          </dl>
        </section>

        <section className="panel">
          <h2>登录</h2>
          <label>
            Tenant
            <input data-testid="login-tenant" value={tenantID} onChange={event => setTenantID(event.target.value)} />
          </label>
          <label>
            User
            <input data-testid="login-user" value={userID} onChange={event => setUserID(event.target.value)} />
          </label>
          <label>
            Password
            <input
              data-testid="login-password"
              type="password"
              value={password}
              onChange={event => setPassword(event.target.value)}
            />
          </label>
          <button data-testid="login-submit" type="button" onClick={() => void login()}>
            登录并连接
          </button>
          <button
            data-testid="logout-submit"
            className="secondary-button"
            type="button"
            onClick={() => void logout()}
            disabled={!session}
          >
            退出登录
          </button>
          <button
            data-testid="refresh-session"
            className="secondary-button"
            type="button"
            onClick={() => void refreshSession()}
            disabled={!session}
          >
            刷新登录态
          </button>
          <button
            data-testid="restore-session"
            className="secondary-button"
            type="button"
            onClick={() => void restoreSession()}
            disabled={!!session}
          >
            恢复会话
          </button>
        </section>

        <section className="conversation-list">
          <h2>会话</h2>
          <div className="manual-open">
            <input
              data-testid="conversation-id-input"
              value={manualConversationID}
              onChange={event => setManualConversationID(event.target.value)}
              placeholder="conversation_id"
            />
            <button data-testid="open-conversation" type="button" onClick={() => void openManualConversation()}>
              打开
            </button>
          </div>
          <button
            data-testid="refresh-conversations"
            type="button"
            onClick={() => void run("load conversations", () => loadConversations())}
          >
            刷新会话
          </button>
          {conversations.map(conversation => (
            <button
              data-testid="conversation-item"
              className={`conversation-item ${conversation.conversationID === activeConversationID ? "active" : ""}`}
              key={conversation.conversationID}
              onClick={() => void run("select conversation", () => selectConversation(conversation.conversationID))}
            >
              <span>{conversation.title}</span>
              <small>seq {conversation.lastSeq}</small>
            </button>
          ))}
        </section>
      </aside>

      <section className="chat">
        <header className="chat-header">
          <div>
            <h2>{(activeConversation?.title ?? activeConversationID) || "未选择会话"}</h2>
            <p>PullInbox 是事实源，WebSocket 只做在线唤醒。</p>
          </div>
          <div className="status-stack">
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
            <div className="empty-state">登录后选择会话，或手动输入 conversation_id 拉取 PullInbox。</div>
          ) : (
            messages.map(message => (
              <article data-testid="message-item" className="message" key={message.messageID || message.clientMessageID}>
                <header>
                  <strong>{message.senderUserID}</strong>
                  <span>#{message.conversationSeq || "pending"}</span>
                </header>
                <p>{message.text}</p>
                <footer>{message.status}</footer>
              </article>
            ))
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
            placeholder="输入文本消息"
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
  return options;
}

function newID(): string {
  if (globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function errorMessage(error: unknown): string {
  if (typeof error === "object" && error !== null && "message" in error) {
    return String((error as { message: unknown }).message);
  }
  return String(error);
}
