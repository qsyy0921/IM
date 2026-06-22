import { useEffect, useMemo, useRef, useState } from "react";
import { createClientRuntime, createClientShellActions, validateRuntimeConfig } from "@nexusim/client-core";
import type {
  AuthSession,
  ContactDecision,
  ContactItem,
  ContactRequestItem,
  ConversationSummary,
  DeliveryNotifyFrame,
  MessageItem,
  ServerFrame
} from "@nexusim/protocol";
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
type ActiveView = "conversations" | "contacts" | "settings";

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

  const [tenantID, setTenantID] = useState("tenant-client-local");
  const [userID, setUserID] = useState("user-a");
  const [password, setPassword] = useState("");
  const [session, setSession] = useState<AuthSession | null>(null);
  const [conversations, setConversations] = useState<ConversationSummary[]>([]);
  const [activeConversationID, setActiveConversationID] = useState("");
  const [manualConversationID, setManualConversationID] = useState("conv-client-local");
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [composerText, setComposerText] = useState("");
  const [newGroupName, setNewGroupName] = useState("");
  const [status, setStatus] = useState("ready");
  const [activeView, setActiveView] = useState<ActiveView>("conversations");
  const [contacts, setContacts] = useState<ContactItem[]>([]);
  const [incomingRequests, setIncomingRequests] = useState<ContactRequestItem[]>([]);
  const [outgoingRequests, setOutgoingRequests] = useState<ContactRequestItem[]>([]);
  const [contactQuery, setContactQuery] = useState("");
  const [contactGroupFilter, setContactGroupFilter] = useState("");
  const [newContactUserID, setNewContactUserID] = useState("");
  const [newContactMessage, setNewContactMessage] = useState("");
  const [remarkDrafts, setRemarkDrafts] = useState<Record<string, string>>({});
  const [groupDrafts, setGroupDrafts] = useState<Record<string, string>>({});
  const [pushStatus, setPushStatus] = useState("disconnected");
  const [lastAck, setLastAck] = useState<{ conversationID: string; seq: number } | null>(null);
  const [error, setError] = useState("");
  const [contactsError, setContactsError] = useState("");
  const [desktopNativeMetadata, setDesktopNativeMetadata] = useState<NativeBridgeMetadata | undefined>();
  const [desktopNativeMetadataProbeFinished, setDesktopNativeMetadataProbeFinished] = useState(false);

  const sessionRef = useRef<AuthSession | null>(null);
  const activeConversationRef = useRef("");
  const pushConnectionRef = useRef<{ close(): void } | null>(null);
  const shellSmokeReportedRef = useRef(false);
  const autoRestoreAttemptedRef = useRef(false);

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
    if (autoRestoreAttemptedRef.current) {
      return;
    }
    autoRestoreAttemptedRef.current = true;
    void (async () => {
      try {
        const state = await shellActions.restoreSession();
        const restoredSession = runtime.auth.current();
        if (!state.authenticated || !restoredSession) {
          return;
        }
        sessionRef.current = restoredSession;
        setSession(restoredSession);
        const currentSession = await ensureFreshSession();
        if (pushConnectionRef.current === null) {
          await connectPush(currentSession);
        }
        await loadConversations(currentSession);
        await loadContactsSoft(currentSession);
        setStatus("restore session ok");
      } catch (caught) {
        pushConnectionRef.current?.close();
        pushConnectionRef.current = null;
        sessionRef.current = null;
        setSession(null);
        await shellActions.logout().catch(() => undefined);
        setStatus("restore session failed");
        setError(`登录态已过期，请重新登录。${errorMessage(caught)}`);
      }
    })();
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
    await run("login", () => performLogin());
  }

  async function registerAccount(): Promise<void> {
    await run("register", async () => {
      if (sessionRef.current) {
        throw new Error("logout first");
      }
      const nextUserID = userID.trim();
      const nextPassword = password.trim();
      if (!nextUserID) {
        throw new Error("account is required");
      }
      if (!nextPassword) {
        throw new Error("password is required");
      }
      await runtime.bff.register({
        tenantID: tenantID.trim(),
        userID: nextUserID,
        password: nextPassword
      });
      await performLogin();
    });
  }

  async function performLogin(): Promise<void> {
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
    await loadContactsSoft(nextSession);
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
        setContacts([]);
        setIncomingRequests([]);
        setOutgoingRequests([]);
        setContactsError("");
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
      const refreshedSession = await refreshAuthenticatedSession();
      await loadConversations(refreshedSession);
      await loadContactsSoft(refreshedSession);
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
      await loadContactsSoft(restoredSession);
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

  async function loadContacts(currentSession = sessionRef.current): Promise<void> {
    if (!currentSession) {
      throw new Error("login first");
    }
    const [nextContacts, nextIncoming, nextOutgoing] = await Promise.all([
      runtime.bff.listContacts(currentSession, {
        pageSize: 80,
        query: contactQuery.trim(),
        groupName: contactGroupFilter.trim()
      }),
      runtime.bff.listContactRequests(currentSession, {
        direction: "INCOMING",
        status: "PENDING",
        pageSize: 50
      }),
      runtime.bff.listContactRequests(currentSession, {
        direction: "OUTGOING",
        status: "PENDING",
        pageSize: 50
      })
    ]);
    setContacts(nextContacts);
    setIncomingRequests(nextIncoming);
    setOutgoingRequests(nextOutgoing);
    setRemarkDrafts(draftsFromContacts(nextContacts, "remark"));
    setGroupDrafts(draftsFromContacts(nextContacts, "groupName"));
    setContactsError("");
  }

  async function loadContactsSoft(currentSession = sessionRef.current): Promise<void> {
    try {
      await loadContacts(currentSession);
    } catch (caught) {
      setContactsError(errorMessage(caught));
    }
  }

  async function sendContactRequest(): Promise<void> {
    await run("send contact request", async () => {
      const currentSession = requireSession();
      const targetUserID = newContactUserID.trim();
      if (!targetUserID) {
        throw new Error("contact user id is required");
      }
      await runtime.bff.sendContactRequest(currentSession, {
        targetUserID,
        message: newContactMessage.trim(),
        sourceType: "DIRECT"
      });
      setNewContactUserID("");
      setNewContactMessage("");
      await loadContacts(currentSession);
    });
  }

  async function respondContactRequest(requestID: string, decision: ContactDecision): Promise<void> {
    await run("respond contact request", async () => {
      const currentSession = requireSession();
      await runtime.bff.respondContactRequest(currentSession, { requestID, decision });
      await loadContacts(currentSession);
    });
  }

  async function cancelContactRequest(requestID: string): Promise<void> {
    await run("cancel contact request", async () => {
      const currentSession = requireSession();
      await runtime.bff.cancelContactRequest(currentSession, { requestID });
      await loadContacts(currentSession);
    });
  }

  async function deleteContact(contactUserID: string): Promise<void> {
    await run("delete contact", async () => {
      const currentSession = requireSession();
      await runtime.bff.deleteContact(currentSession, { contactUserID });
      await loadContacts(currentSession);
    });
  }

  async function blockContact(contactUserID: string): Promise<void> {
    await run("block contact", async () => {
      const currentSession = requireSession();
      await runtime.bff.blockContact(currentSession, { contactUserID, reason: "client block" });
      await loadContacts(currentSession);
    });
  }

  async function unblockContact(contactUserID: string): Promise<void> {
    await run("unblock contact", async () => {
      const currentSession = requireSession();
      await runtime.bff.unblockContact(currentSession, { contactUserID });
      await loadContacts(currentSession);
    });
  }

  async function saveContactRemark(contactUserID: string): Promise<void> {
    await run("update contact remark", async () => {
      const currentSession = requireSession();
      await runtime.bff.updateContactRemark(currentSession, {
        contactUserID,
        remark: remarkDrafts[contactUserID] ?? ""
      });
      await loadContacts(currentSession);
    });
  }

  async function saveContactGroup(contactUserID: string): Promise<void> {
    await run("update contact group", async () => {
      const currentSession = requireSession();
      await runtime.bff.updateContactGroup(currentSession, {
        contactUserID,
        groupName: groupDrafts[contactUserID] ?? ""
      });
      await loadContacts(currentSession);
    });
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

  async function createGroupConversation(): Promise<void> {
    await run("create group", async () => {
      const currentSession = requireSession();
      const displayName = newGroupName.trim() || "新群聊";
      const conversationID = newConversationID(displayName);
      const created = await runtime.bff.createConversation(
        {
          tenantID: currentSession.tenantID,
          conversationID,
          type: "GROUP",
          idempotencyKey: newID()
        },
        currentSession
      );
      const optimistic: ConversationSummary = {
        tenantID: currentSession.tenantID,
        conversationID: created.conversationID,
        type: created.type,
        status: "ACTIVE",
        title: displayName,
        lastSeq: created.boundarySeq,
        unreadCount: 0,
        muted: false,
        pinned: false,
        updatedAtMs: Date.now()
      };
      setConversations(current => [optimistic, ...current.filter(item => item.conversationID !== created.conversationID)]);
      setManualConversationID(created.conversationID);
      setNewGroupName("");
      activeConversationRef.current = created.conversationID;
      setActiveConversationID(created.conversationID);
      setMessages([]);
    });
  }

  async function openDirectConversation(contact: ContactItem): Promise<void> {
    await run("open direct conversation", async () => {
      const currentSession = requireSession();
      if (contact.status !== "ACTIVE") {
        throw new Error("contact is not active");
      }
      const created = await runtime.bff.openDirectConversation(
        {
          peerUserID: contact.contactUserID
        },
        currentSession
      );
      const displayName = contactDisplayName(contact);
      const directSummary: ConversationSummary = {
        tenantID: currentSession.tenantID,
        conversationID: created.conversationID,
        type: "DIRECT",
        status: "ACTIVE",
        title: displayName,
        lastSeq: created.boundarySeq,
        unreadCount: 0,
        muted: false,
        pinned: false,
        updatedAtMs: Date.now()
      };
      setConversations(current => [
        directSummary,
        ...current.filter(item => item.conversationID !== created.conversationID)
      ]);
      setManualConversationID(created.conversationID);
      setActiveView("conversations");
      activeConversationRef.current = created.conversationID;
      setActiveConversationID(created.conversationID);
      setMessages([]);
      await syncConversation(created.conversationID, currentSession);
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
      if (shouldRefreshBefore(label) && sessionRef.current) {
        await ensureFreshSession();
      }
      await task();
      setStatus(`${label} ok`);
    } catch (caught) {
      if (shouldRefreshBefore(label) && isUnauthenticated(caught) && sessionRef.current?.refreshToken) {
        try {
          await refreshAuthenticatedSession();
          await task();
          setStatus(`${label} ok`);
          return;
        } catch (retried) {
          caught = retried;
        }
      }
      setStatus(`${label} failed`);
      setError(errorMessage(caught));
    }
  }

  async function ensureFreshSession(): Promise<AuthSession> {
    const currentSession = requireSession();
    const expiresAtMs = currentSession.expiresAtMs ?? 0;
    if (expiresAtMs > 0 && expiresAtMs <= Date.now() + 60_000) {
      return refreshAuthenticatedSession();
    }
    return currentSession;
  }

  async function refreshAuthenticatedSession(): Promise<AuthSession> {
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
    try {
      await connectPush(refreshedSession);
    } catch (caught) {
      setPushStatus(`reconnect failed: ${errorMessage(caught)}`);
    }
    return refreshedSession;
  }

  function requireSession(): AuthSession {
    const currentSession = sessionRef.current ?? session;
    if (!currentSession) {
      throw new Error("login first");
    }
    return currentSession;
  }

  const activeConversation = conversations.find(item => item.conversationID === activeConversationID);
  const visibleContacts = contacts
    .filter(contact => contact.status !== "DELETED")
    .sort((left, right) => contactDisplayName(left).localeCompare(contactDisplayName(right)));
  const latestMessageSeq = messages.reduce((max, message) => Math.max(max, message.conversationSeq), 0);
  const nativeMetadata = desktopNativeMetadata ?? androidNativeMetadata;

  return (
    <main className="shell">
      <nav className="app-rail" aria-label="NexusIM">
        <div className="brand-mark">N</div>
        <button
          className={`rail-button ${activeView === "conversations" ? "active" : ""}`}
          type="button"
          aria-label="会话"
          onClick={() => setActiveView("conversations")}
        >
          会
        </button>
        <button
          className={`rail-button ${activeView === "contacts" ? "active" : ""}`}
          type="button"
          aria-label="联系人"
          onClick={() => setActiveView("contacts")}
        >
          人
        </button>
        <button
          className={`rail-button ${activeView === "settings" ? "active" : ""}`}
          type="button"
          aria-label="设置"
          onClick={() => setActiveView("settings")}
        >
          设
        </button>
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
            data-testid="register-submit"
            className="text-button"
            type="button"
            onClick={() => void registerAccount()}
            disabled={!!session}
          >
            注册并登录
          </button>
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

        {activeView === "conversations" ? (
          <section className="conversation-list" aria-label="好友和群聊列表">
            <section className="list-section" aria-label="群聊列表">
              <div className="list-section-header">
                <div>
                  <h2>群聊列表</h2>
                  <p>{session ? `${conversations.length} 个群聊 / 会话` : "登录后同步群聊"}</p>
                </div>
                <button type="button" onClick={() => void run("load conversations", () => loadConversations())} disabled={!session}>
                  刷新
                </button>
              </div>
              <form
                className="group-form"
                onSubmit={event => {
                  event.preventDefault();
                  void createGroupConversation();
                }}
              >
                <input
                  data-testid="new-group-name"
                  placeholder="群聊名称"
                  value={newGroupName}
                  onChange={event => setNewGroupName(event.target.value)}
                  disabled={!session}
                />
                <button data-testid="create-group" type="submit" disabled={!session}>
                  建群
                </button>
              </form>
              <div className="compact-list">
                {conversations.length === 0 ? (
                  <div className="conversation-empty">暂无群聊</div>
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
              </div>
            </section>

            <section className="list-section" aria-label="好友列表">
              <div className="list-section-header">
                <div>
                  <h2>好友列表</h2>
                  <p>{session ? `${visibleContacts.length} 位好友` : "登录后同步好友"}</p>
                </div>
                <button type="button" onClick={() => void run("load contacts", () => loadContacts())} disabled={!session}>
                  刷新
                </button>
              </div>
              <div className="friend-list">
                {visibleContacts.length === 0 ? <div className="mini-empty">暂无好友</div> : null}
                {visibleContacts.map(contact => (
                  <article className="friend-item" key={contact.contactUserID}>
                    <span className="conversation-avatar">{contactLabel(contact)}</span>
                    <span className="conversation-copy">
                      <strong>{contactDisplayName(contact)}</strong>
                      <small>
                        {contact.contactUserID}
                        {contact.groupName ? ` · ${contact.groupName}` : ""}
                      </small>
                    </span>
                    <span className={`friend-status ${contact.status === "BLOCKED" ? "blocked" : ""}`}>
                      {contact.status === "BLOCKED" ? "已拉黑" : "好友"}
                    </span>
                    <button
                      className="friend-message-button"
                      type="button"
                      onClick={() => void openDirectConversation(contact)}
                      disabled={!session || contact.status !== "ACTIVE"}
                    >
                      发消息
                    </button>
                  </article>
                ))}
              </div>
            </section>
          </section>
        ) : null}

        {activeView === "contacts" ? (
          <section className="contacts-panel" aria-label="联系人">
            <div className="contacts-toolbar">
              <input
                data-testid="contact-query"
                placeholder="搜索联系人"
                value={contactQuery}
                onChange={event => setContactQuery(event.target.value)}
                disabled={!session}
              />
              <input
                data-testid="contact-group-filter"
                placeholder="分组"
                value={contactGroupFilter}
                onChange={event => setContactGroupFilter(event.target.value)}
                disabled={!session}
              />
              <button type="button" onClick={() => void run("load contacts", () => loadContacts())} disabled={!session}>
                刷新
              </button>
            </div>

            <form
              className="contact-form"
              onSubmit={event => {
                event.preventDefault();
                void sendContactRequest();
              }}
            >
              <input
                data-testid="new-contact-user"
                placeholder="用户 ID"
                value={newContactUserID}
                onChange={event => setNewContactUserID(event.target.value)}
                disabled={!session}
              />
              <input
                data-testid="new-contact-message"
                placeholder="验证消息"
                value={newContactMessage}
                onChange={event => setNewContactMessage(event.target.value)}
                disabled={!session}
              />
              <button type="submit" disabled={!session || newContactUserID.trim() === ""}>
                添加
              </button>
            </form>

            {contactsError ? <div className="contacts-error">{contactsError}</div> : null}

            <div className="contact-section-title">收到的申请</div>
            <div className="request-list">
              {incomingRequests.length === 0 ? <div className="mini-empty">暂无待处理申请</div> : null}
              {incomingRequests.map(item => (
                <article className="request-item" key={item.requestID}>
                  <div>
                    <strong>{item.senderUserID}</strong>
                    <span>{item.message || "请求添加你为联系人"}</span>
                  </div>
                  <div className="contact-actions">
                    <button type="button" onClick={() => void respondContactRequest(item.requestID, "ACCEPT")}>
                      接受
                    </button>
                    <button
                      className="ghost-button"
                      type="button"
                      onClick={() => void respondContactRequest(item.requestID, "DECLINE")}
                    >
                      拒绝
                    </button>
                  </div>
                </article>
              ))}
            </div>

            <div className="contact-section-title">发出的申请</div>
            <div className="request-list">
              {outgoingRequests.length === 0 ? <div className="mini-empty">暂无待确认申请</div> : null}
              {outgoingRequests.map(item => (
                <article className="request-item" key={item.requestID}>
                  <div>
                    <strong>{item.receiverUserID}</strong>
                    <span>{item.status.toLowerCase()}</span>
                  </div>
                  <button className="ghost-button" type="button" onClick={() => void cancelContactRequest(item.requestID)}>
                    取消
                  </button>
                </article>
              ))}
            </div>

            <div className="contact-section-title">联系人</div>
            <div className="contact-list">
              {contacts.length === 0 ? <div className="mini-empty">暂无联系人</div> : null}
              {contacts.map(contact => (
                <article className="contact-item" key={contact.contactUserID}>
                  <header>
                    <span className="conversation-avatar">{contactLabel(contact)}</span>
                    <div>
                      <strong>{contact.contactUserID}</strong>
                      <small>{contact.status.toLowerCase()}</small>
                    </div>
                  </header>
                  <label>
                    备注
                    <input
                      value={remarkDrafts[contact.contactUserID] ?? ""}
                      onChange={event =>
                        setRemarkDrafts(current => ({ ...current, [contact.contactUserID]: event.target.value }))
                      }
                    />
                  </label>
                  <label>
                    分组
                    <input
                      value={groupDrafts[contact.contactUserID] ?? ""}
                      onChange={event =>
                        setGroupDrafts(current => ({ ...current, [contact.contactUserID]: event.target.value }))
                      }
                    />
                  </label>
                  <div className="contact-actions">
                    <button
                      type="button"
                      onClick={() => void openDirectConversation(contact)}
                      disabled={!session || contact.status !== "ACTIVE"}
                    >
                      发消息
                    </button>
                    <button type="button" onClick={() => void saveContactRemark(contact.contactUserID)}>
                      存备注
                    </button>
                    <button type="button" onClick={() => void saveContactGroup(contact.contactUserID)}>
                      存分组
                    </button>
                    {contact.status === "BLOCKED" ? (
                      <button className="ghost-button" type="button" onClick={() => void unblockContact(contact.contactUserID)}>
                        取消拉黑
                      </button>
                    ) : (
                      <button className="ghost-button" type="button" onClick={() => void blockContact(contact.contactUserID)}>
                        拉黑
                      </button>
                    )}
                    <button className="danger-button" type="button" onClick={() => void deleteContact(contact.contactUserID)}>
                      删除
                    </button>
                  </div>
                </article>
              ))}
            </div>
          </section>
        ) : null}

        {activeView === "settings" ? (
          <section className="settings-panel" aria-label="设置">
            <div className="conversation-empty">客户端设置将在后续切片接入。</div>
          </section>
        ) : null}

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

        <section className="message-panel" aria-label="消息列表">
          <div className="message-list-header">
            <div>
              <h3>消息列表</h3>
              <p>
                {activeConversationID
                  ? `${messages.length} 条消息${latestMessageSeq > 0 ? ` / 最新 #${latestMessageSeq}` : ""}`
                  : "请选择群聊或会话"}
              </p>
            </div>
            <button
              type="button"
              onClick={() => void run("sync messages", () => syncConversation(activeConversationID))}
              disabled={!session || !activeConversationID}
            >
              同步
            </button>
          </div>
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
        </section>

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

function newConversationID(title: string): string {
  const slug = title
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 24);
  return `group-${slug || "chat"}-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
}

function draftsFromContacts(contacts: ContactItem[], field: "remark" | "groupName"): Record<string, string> {
  const drafts: Record<string, string> = {};
  for (const contact of contacts) {
    drafts[contact.contactUserID] = field === "remark" ? contact.remark : contact.groupName;
  }
  return drafts;
}

function contactLabel(contact: ContactItem): string {
  const display = contactDisplayName(contact);
  return display.slice(0, 1).toUpperCase();
}

function contactDisplayName(contact: ContactItem): string {
  return contact.remark.trim() || contact.contactUserID;
}

function nativeLocalStoreStatus(localStore: NonNullable<NativeBridgeMetadata["capabilities"]>["localStore"]): string {
  if (!localStore) {
    return "local-storage";
  }
  const readiness = localStore.ready ? "ready" : localStore.reason;
  return `local-storage -> ${localStore.requestedStore}; ${readiness}; ${localStore.bridge}`;
}

function shouldRefreshBefore(label: string): boolean {
  return !["login", "register", "restore session", "refresh session", "logout"].includes(label);
}

function isUnauthenticated(error: unknown): boolean {
  if (typeof error !== "object" || error === null) {
    return false;
  }
  const candidate = error as { code?: unknown; message?: unknown };
  return candidate.code === "UNAUTHENTICATED" || String(candidate.message ?? "").toLowerCase().includes("token expired");
}

function errorMessage(error: unknown): string {
  if (typeof error === "object" && error !== null && "message" in error) {
    const message = String((error as { message: unknown }).message);
    if (message === "Failed to fetch" || message.includes("NetworkError")) {
      return `无法连接本地 NexusIM 服务，请先启动客户端后端。API: ${runtimeConfig.apiBaseURL}，WebSocket: ${runtimeConfig.pushWebSocketURL}`;
    }
    return message;
  }
  return String(error);
}
