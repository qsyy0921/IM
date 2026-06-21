import { useMemo, useRef, useState } from "react";
import { MessageSendQueue, validateRuntimeConfig } from "@nexusim/client-core";
import type { AuthSession, ConversationSummary, DeliveryNotifyFrame, MessageItem, ServerFrame } from "@nexusim/protocol";
import { BFFClient } from "./adapters/bff-client";
import { BrowserPushTransport } from "./adapters/browser-push-transport";
import { IndexedDBMessageStore } from "./adapters/indexeddb-message-store";
import { loadRuntimeConfig } from "./runtime-config";

const runtimeConfig = validateRuntimeConfig(loadRuntimeConfig());

export function App() {
  const api = useMemo(() => new BFFClient(runtimeConfig.apiBaseURL), []);
  const store = useMemo(() => new IndexedDBMessageStore(), []);
  const pushTransport = useMemo(() => new BrowserPushTransport(), []);
  const sendQueue = useMemo(
    () =>
      new MessageSendQueue({
        messagingAPI: api,
        store,
        idempotencyKeyFactory: newID,
        clientMessageIDFactory: newID,
        nowMs: () => Date.now()
      }),
    [api, store]
  );

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
  const [error, setError] = useState("");

  const sessionRef = useRef<AuthSession | null>(null);
  const activeConversationRef = useRef("");
  const pushConnectionRef = useRef<{ close(): void } | null>(null);

  async function login(): Promise<void> {
    await run("login", async () => {
      pushConnectionRef.current?.close();
      const response = await api.login({
        tenantID,
        userID,
        password,
        deviceID: runtimeConfig.deviceID
      });
      const nextSession = response.session;
      sessionRef.current = nextSession;
      setSession(nextSession);
      await connectPush(nextSession);
      await loadConversations(nextSession);
    });
  }

  async function logout(): Promise<void> {
    await run("logout", async () => {
      const currentSession = sessionRef.current;
      try {
        if (currentSession) {
          await api.logout(currentSession);
        }
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

  async function loadConversations(currentSession = sessionRef.current): Promise<void> {
    if (!currentSession) {
      throw new Error("login first");
    }
    const items = await api.listConversations(currentSession, { limit: 50 });
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
      await sendQueue.sendText({ session: currentSession, conversationID, text });
      await syncConversation(conversationID, currentSession);
    });
  }

  async function connectPush(currentSession: AuthSession): Promise<void> {
    setPushStatus("connecting");
    const connection = await pushTransport.connect({
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
    const response = await api.pullInbox(
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
      await api.ackDelivery(
        {
          tenantID: currentSession.tenantID,
          userID: currentSession.userID,
          deviceID: currentSession.deviceID,
          conversationID,
          lastReceivedSeq: maxSeq,
          requestID: newID()
        },
        currentSession
      );
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
          </dl>
        </section>

        <section className="panel">
          <h2>登录</h2>
          <label>
            Tenant
            <input value={tenantID} onChange={event => setTenantID(event.target.value)} />
          </label>
          <label>
            User
            <input value={userID} onChange={event => setUserID(event.target.value)} />
          </label>
          <label>
            Password
            <input type="password" value={password} onChange={event => setPassword(event.target.value)} />
          </label>
          <button type="button" onClick={() => void login()}>
            登录并连接
          </button>
          <button className="secondary-button" type="button" onClick={() => void logout()} disabled={!session}>
            退出登录
          </button>
        </section>

        <section className="conversation-list">
          <h2>会话</h2>
          <div className="manual-open">
            <input
              value={manualConversationID}
              onChange={event => setManualConversationID(event.target.value)}
              placeholder="conversation_id"
            />
            <button type="button" onClick={() => void openManualConversation()}>
              打开
            </button>
          </div>
          <button type="button" onClick={() => void run("load conversations", () => loadConversations())}>
            刷新会话
          </button>
          {conversations.map(conversation => (
            <button
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
            <span className="status-pill">{status}</span>
            <span className="status-pill neutral">push {pushStatus}</span>
          </div>
        </header>

        {error ? <div className="error-banner">{error}</div> : null}

        <div className="messages">
          {messages.length === 0 ? (
            <div className="empty-state">登录后选择会话，或手动输入 conversation_id 拉取 PullInbox。</div>
          ) : (
            messages.map(message => (
              <article className="message" key={message.messageID || message.clientMessageID}>
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
            placeholder="输入文本消息"
            value={composerText}
            onChange={event => setComposerText(event.target.value)}
            disabled={!session || !activeConversationID}
          />
          <button type="submit" disabled={!session || !activeConversationID || composerText.trim() === ""}>
            发送
          </button>
        </form>
      </section>
    </main>
  );
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
