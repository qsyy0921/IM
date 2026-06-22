import { useEffect, useMemo, useRef, useState } from "react";
import { createClientRuntime, createClientShellActions, validateRuntimeConfig } from "@nexusim/client-core";
import type {
  AuthSession,
  ContactDecision,
  ContactItem,
  ContactRequestItem,
  ConversationMember,
  ConversationSummary,
  DeliveryNotifyFrame,
  ListConversationMembersRequest,
  MemberRole,
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
type GroupMemberRoleFilter = "ALL" | "OWNER" | "ADMIN" | "MEMBER";

const GROUP_MEMBER_PAGE_SIZE = 8;

interface LoadGroupMembersOptions {
  pageToken?: string;
  pageTokens?: string[];
  pageIndex?: number;
  query?: string;
  roleFilter?: GroupMemberRoleFilter;
}

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
  const [groupInviteUserID, setGroupInviteUserID] = useState("");
  const [groupMembers, setGroupMembers] = useState<ConversationMember[]>([]);
  const [groupMembersConversationID, setGroupMembersConversationID] = useState("");
  const [groupMembersError, setGroupMembersError] = useState("");
  const [groupMemberQuery, setGroupMemberQuery] = useState("");
  const [groupMemberRoleFilter, setGroupMemberRoleFilter] = useState<GroupMemberRoleFilter>("ALL");
  const [groupMemberNextPageToken, setGroupMemberNextPageToken] = useState("");
  const [groupMemberPageTokens, setGroupMemberPageTokens] = useState<string[]>([]);
  const [groupMemberPageIndex, setGroupMemberPageIndex] = useState(0);
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
    clearSessionViewState();
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
        clearSessionViewState();
        setComposerText("");
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
    const mergedItems = mergeConversationSummaries(conversations, items);
    setConversations(mergedItems);
    const nextActiveID = chooseActiveConversationID(mergedItems, activeConversationRef.current);
    if (!nextActiveID) {
      activeConversationRef.current = "";
      setActiveConversationID("");
      setMessages([]);
      return;
    }
    await selectConversation(nextActiveID, currentSession);
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
    const selectedConversation = conversations.find(item => item.conversationID === conversationID);
    activeConversationRef.current = conversationID;
    setActiveConversationID(conversationID);
    setManualConversationID(conversationID);
    setActiveView("conversations");
    await showCachedMessages(conversationID);
    await syncConversation(conversationID, currentSession);
    if (selectedConversation?.type === "GROUP") {
      await loadGroupMembers(conversationID, currentSession, {
        query: "",
        roleFilter: "ALL",
        pageToken: "",
        pageTokens: [],
        pageIndex: 0
      });
    } else {
      clearGroupMemberState();
    }
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
        memberVersion: created.memberVersion,
        unreadCount: 0,
        muted: false,
        pinned: false,
        updatedAtMs: Date.now()
      };
      upsertConversationSummary(optimistic);
      setNewGroupName("");
      await selectConversation(created.conversationID, currentSession);
      await loadGroupMembers(created.conversationID, currentSession);
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
        memberVersion: created.memberVersion,
        unreadCount: 0,
        muted: false,
        pinned: false,
        updatedAtMs: Date.now()
      };
      upsertConversationSummary(directSummary);
      await selectConversation(created.conversationID, currentSession);
    });
  }

  async function loadGroupMembers(
    conversationID = activeConversationRef.current,
    currentSession = sessionRef.current,
    options: LoadGroupMembersOptions = {}
  ): Promise<void> {
    if (!currentSession) {
      throw new Error("login first");
    }
    if (!conversationID) {
      throw new Error("conversation id is required");
    }
    const query = (options.query ?? groupMemberQuery).trim();
    const roleFilter = options.roleFilter ?? groupMemberRoleFilter;
    const pageToken = options.pageToken ?? "";
    const pageTokens = options.pageTokens ?? [];
    const pageIndex = options.pageIndex ?? 0;
    try {
      const request: ListConversationMembersRequest = {
        conversationID,
        pageSize: GROUP_MEMBER_PAGE_SIZE
      };
      if (pageToken) {
        request.pageToken = pageToken;
      }
      if (roleFilter !== "ALL") {
        request.roleFilter = roleFilter;
      }
      if (query) {
        request.userIDPrefix = query;
      }
      const response = await runtime.bff.listConversationMembers(request, currentSession);
      setGroupMembers(response.members);
      setGroupMembersConversationID(response.conversationID);
      setGroupMembersError("");
      setGroupMemberQuery(query);
      setGroupMemberRoleFilter(roleFilter);
      setGroupMemberNextPageToken(response.nextPageToken);
      setGroupMemberPageTokens(pageTokens);
      setGroupMemberPageIndex(pageIndex);
      updateConversationMemberVersion(response.conversationID, response.memberVersion);
    } catch (caught) {
      setGroupMembers([]);
      setGroupMembersConversationID(conversationID);
      setGroupMembersError(errorMessage(caught));
      setGroupMemberNextPageToken("");
      throw caught;
    }
  }

  async function applyGroupMemberFilters(): Promise<void> {
    await run("filter group members", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      await loadGroupMembers(conversation.conversationID, currentSession, {
        query: groupMemberQuery,
        roleFilter: groupMemberRoleFilter,
        pageToken: "",
        pageTokens: [],
        pageIndex: 0
      });
    });
  }

  async function resetGroupMemberFilters(): Promise<void> {
    await run("reset group member filters", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      await loadGroupMembers(conversation.conversationID, currentSession, {
        query: "",
        roleFilter: "ALL",
        pageToken: "",
        pageTokens: [],
        pageIndex: 0
      });
    });
  }

  async function loadNextGroupMemberPage(): Promise<void> {
    if (!groupMemberNextPageToken) {
      return;
    }
    await run("load next group member page", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      const nextPageTokens = [...groupMemberPageTokens, groupMemberNextPageToken];
      await loadGroupMembers(conversation.conversationID, currentSession, {
        query: groupMemberQuery,
        roleFilter: groupMemberRoleFilter,
        pageToken: groupMemberNextPageToken,
        pageTokens: nextPageTokens,
        pageIndex: groupMemberPageIndex + 1
      });
    });
  }

  async function loadPreviousGroupMemberPage(): Promise<void> {
    if (groupMemberPageIndex <= 0) {
      return;
    }
    await run("load previous group member page", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      const previousPageIndex = groupMemberPageIndex - 1;
      let previousPageToken = "";
      if (previousPageIndex > 0) {
        const previousTokenCandidate = groupMemberPageTokens[previousPageIndex - 1];
        if (!previousTokenCandidate) {
          throw new Error("group member page token is required");
        }
        previousPageToken = previousTokenCandidate;
      }
      await loadGroupMembers(conversation.conversationID, currentSession, {
        query: groupMemberQuery,
        roleFilter: groupMemberRoleFilter,
        pageToken: previousPageToken,
        pageTokens: groupMemberPageTokens.slice(0, previousPageIndex),
        pageIndex: previousPageIndex
      });
    });
  }

  async function inviteGroupMember(): Promise<void> {
    await run("invite group member", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      const targetUserID = groupInviteUserID.trim();
      if (!targetUserID) {
        throw new Error("target user id is required");
      }
      const result = await runtime.bff.inviteConversationMember(
        {
          conversationID: conversation.conversationID,
          targetUserID,
          expectedMemberVersion: conversation.memberVersion,
          reason: "client group invite"
        },
        currentSession
      );
      setGroupInviteUserID("");
      updateConversationMemberVersion(result.conversationID, result.memberVersion);
      await loadGroupMembers(result.conversationID, currentSession);
      await loadConversations(currentSession);
      if (result.conversationID) {
        await selectConversation(result.conversationID, currentSession);
      }
    });
  }

  async function leaveGroupConversation(): Promise<void> {
    await run("leave group", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      const result = await runtime.bff.leaveConversation(
        {
          conversationID: conversation.conversationID,
          expectedMemberVersion: conversation.memberVersion,
          reason: "client leave group"
        },
        currentSession
      );
      setConversations(current => current.filter(item => item.conversationID !== result.conversationID));
      activeConversationRef.current = "";
      setActiveConversationID("");
      setManualConversationID("");
      setMessages([]);
      clearGroupMemberState();
      await loadConversations(currentSession);
    });
  }

  async function removeGroupMember(member: ConversationMember): Promise<void> {
    await run("remove group member", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      const result = await runtime.bff.removeConversationMember(
        {
          conversationID: conversation.conversationID,
          targetUserID: member.userID,
          expectedMemberVersion: conversation.memberVersion,
          reason: "client remove group member"
        },
        currentSession
      );
      updateConversationMemberVersion(result.conversationID, result.memberVersion);
      await loadGroupMembers(result.conversationID, currentSession);
      await loadConversations(currentSession);
    });
  }

  async function updateGroupMemberRole(member: ConversationMember, targetRole: MemberRole): Promise<void> {
    await run("update group member role", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      const result = await runtime.bff.updateConversationMemberRole(
        {
          conversationID: conversation.conversationID,
          targetUserID: member.userID,
          targetRole,
          expectedMemberVersion: conversation.memberVersion,
          reason: `client set member role ${targetRole}`
        },
        currentSession
      );
      updateConversationMemberVersion(result.conversationID, result.memberVersion);
      await loadGroupMembers(result.conversationID, currentSession);
      await loadConversations(currentSession);
    });
  }

  async function transferGroupOwner(member: ConversationMember): Promise<void> {
    await run("transfer group owner", async () => {
      const currentSession = requireSession();
      const conversation = requireActiveGroupConversation();
      const result = await runtime.bff.transferConversationOwner(
        {
          conversationID: conversation.conversationID,
          newOwnerUserID: member.userID,
          expectedMemberVersion: conversation.memberVersion,
          reason: "client transfer group owner"
        },
        currentSession
      );
      updateConversationMemberVersion(result.conversationID, result.memberVersion);
      await loadGroupMembers(result.conversationID, currentSession);
      await loadConversations(currentSession);
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
      try {
        const sent = await runtime.sendQueue.sendText({ session: currentSession, conversationID, text });
        updateConversationLastSeq(conversationID, sent.conversationSeq);
      } catch (caught) {
        await showCachedMessages(conversationID);
        throw caught;
      }
      await showCachedMessages(conversationID);
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
    updateConversationLastSeq(conversationID, maxSeq);
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
      if (isUnauthenticated(caught) && sessionRef.current) {
        await clearExpiredSession(caught);
        return;
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

  function requireActiveGroupConversation(): ConversationSummary {
    const conversation = conversations.find(item => item.conversationID === activeConversationRef.current);
    if (!conversation) {
      throw new Error("select a group conversation first");
    }
    if (conversation.type !== "GROUP") {
      throw new Error("group conversation required");
    }
    return conversation;
  }

  function clearSessionViewState(): void {
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
    clearGroupMemberState();
    setLastAck(null);
    setPushStatus("disconnected");
  }

  function clearGroupMemberState(): void {
    setGroupMembers([]);
    setGroupMembersConversationID("");
    setGroupMembersError("");
    setGroupMemberQuery("");
    setGroupMemberRoleFilter("ALL");
    setGroupMemberNextPageToken("");
    setGroupMemberPageTokens([]);
    setGroupMemberPageIndex(0);
  }

  async function clearExpiredSession(caught: unknown): Promise<void> {
    await shellActions.logout().catch(() => undefined);
    clearSessionViewState();
    setComposerText("");
    setStatus("login expired");
    setError(`登录态已过期，请重新登录。${errorMessage(caught)}`);
  }

  function upsertConversationSummary(summary: ConversationSummary): void {
    setConversations(current => [summary, ...current.filter(item => item.conversationID !== summary.conversationID)]);
  }

  function updateConversationLastSeq(conversationID: string, seq: number): void {
    if (seq <= 0) {
      return;
    }
    setConversations(current =>
      current.map(item =>
        item.conversationID === conversationID
          ? { ...item, lastSeq: Math.max(item.lastSeq, seq), updatedAtMs: Date.now() }
          : item
      )
    );
  }

  function updateConversationMemberVersion(conversationID: string, memberVersion: number): void {
    if (memberVersion <= 0) {
      return;
    }
    setConversations(current =>
      current.map(item =>
        item.conversationID === conversationID
          ? { ...item, memberVersion: Math.max(item.memberVersion, memberVersion), updatedAtMs: Date.now() }
          : item
      )
    );
  }

  async function showCachedMessages(conversationID: string): Promise<void> {
    const cachedMessages = await store.listMessages(conversationID);
    if (activeConversationRef.current === conversationID) {
      setMessages(cachedMessages);
    }
  }

  const activeConversation = conversations.find(item => item.conversationID === activeConversationID);
  const activeConversationTitle = activeConversation
    ? conversationDisplayTitle(activeConversation)
    : activeConversationID
      ? titleFromConversationID(activeConversationID, "GROUP")
      : "选择一个会话";
  const activeConversationSubtitle = activeConversation
    ? conversationSubtitle(activeConversation)
    : session
      ? "请选择左侧好友或群聊"
      : "请先登录";
  const emptyState = emptyMessageState(Boolean(session), Boolean(activeConversationID));
  const activeGroupConversation = activeConversation?.type === "GROUP" ? activeConversation : undefined;
  const groupMembersForActive =
    activeGroupConversation && groupMembersConversationID === activeGroupConversation.conversationID
      ? groupMembers
      : [];
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
                    <span className="conversation-avatar">{conversationAvatarText(conversation)}</span>
                    <span className="conversation-copy">
                      <strong>{conversationDisplayTitle(conversation)}</strong>
                      <small>{conversationSubtitle(conversation)}</small>
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
                  <button
                    data-testid="friend-conversation-item"
                    className="friend-item"
                    key={contact.contactUserID}
                    type="button"
                    onClick={() => void openDirectConversation(contact)}
                    disabled={!session || contact.status !== "ACTIVE"}
                  >
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
                  </button>
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
            <h2>{activeConversationTitle}</h2>
            <p>{activeConversationSubtitle}</p>
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

        {activeGroupConversation ? (
          <section className="group-settings" aria-label="群设置">
            <div className="group-profile-card" data-testid="group-profile-card">
              <span className="group-profile-avatar" data-testid="group-profile-avatar">
                {conversationAvatarText(activeGroupConversation)}
              </span>
              <div className="group-profile-copy">
                <strong data-testid="group-profile-title">{conversationDisplayTitle(activeGroupConversation)}</strong>
                <span data-testid="group-profile-subtitle">
                  {conversationStatusLabel(activeGroupConversation.status)} · 最新 #
                  {activeGroupConversation.lastSeq || 0} · member v{activeGroupConversation.memberVersion}
                </span>
              </div>
              <div className="group-profile-badges" aria-label="群状态">
                <span data-testid="group-profile-id">{compactConversationID(activeGroupConversation.conversationID)}</span>
                <span data-testid="group-profile-member-count">{groupMembersForActive.length} 人</span>
                {activeGroupConversation.pinned ? <span data-testid="group-profile-pinned">置顶</span> : null}
                {activeGroupConversation.muted ? <span data-testid="group-profile-muted">免打扰</span> : null}
                {activeGroupConversation.unreadCount > 0 ? (
                  <span data-testid="group-profile-unread">未读 {activeGroupConversation.unreadCount}</span>
                ) : null}
              </div>
            </div>
            <button
              data-testid="group-members-refresh"
              className="ghost-button"
              type="button"
              onClick={() => void applyGroupMemberFilters()}
              disabled={!session}
            >
              刷新成员
            </button>
            <form
              className="group-member-toolbar"
              onSubmit={event => {
                event.preventDefault();
                void applyGroupMemberFilters();
              }}
            >
              <input
                data-testid="group-member-search"
                placeholder="按用户 ID 前缀搜索"
                value={groupMemberQuery}
                onChange={event => setGroupMemberQuery(event.target.value)}
                disabled={!session}
              />
              <select
                data-testid="group-member-role-filter"
                value={groupMemberRoleFilter}
                onChange={event => setGroupMemberRoleFilter(event.target.value as GroupMemberRoleFilter)}
                disabled={!session}
              >
                <option value="ALL">全部角色</option>
                <option value="OWNER">群主</option>
                <option value="ADMIN">管理员</option>
                <option value="MEMBER">成员</option>
              </select>
              <button data-testid="group-member-filter-submit" type="submit" disabled={!session}>
                筛选
              </button>
              <button
                data-testid="group-member-filter-reset"
                type="button"
                onClick={() => void resetGroupMemberFilters()}
                disabled={!session}
              >
                清空
              </button>
            </form>
            <form
              className="group-member-form"
              onSubmit={event => {
                event.preventDefault();
                void inviteGroupMember();
              }}
            >
              <input
                data-testid="group-invite-user"
                placeholder="添加成员用户 ID"
                value={groupInviteUserID}
                onChange={event => setGroupInviteUserID(event.target.value)}
                disabled={!session}
              />
              <button data-testid="group-invite-submit" type="submit" disabled={!session || groupInviteUserID.trim() === ""}>
                添加成员
              </button>
              <span data-testid="group-invite-source" className="group-invite-source">
                邀请来源：当前群 {compactConversationID(activeGroupConversation.conversationID)}
              </span>
            </form>
            <button
              data-testid="group-leave-submit"
              className="danger-inline-button"
              type="button"
              onClick={() => void leaveGroupConversation()}
              disabled={!session}
            >
              退群
            </button>
            {groupMembersError ? <div className="group-members-error">{groupMembersError}</div> : null}
            <div className="group-member-pagination" aria-label="群成员分页">
              <span data-testid="group-member-page-status">
                第 {groupMemberPageIndex + 1} 页 · 本页 {groupMembersForActive.length} 人
                {groupMemberNextPageToken ? " · 还有更多" : ""}
              </span>
              <div>
                <button
                  data-testid="group-member-prev-page"
                  type="button"
                  onClick={() => void loadPreviousGroupMemberPage()}
                  disabled={!session || groupMemberPageIndex <= 0}
                >
                  上一页
                </button>
                <button
                  data-testid="group-member-next-page"
                  type="button"
                  onClick={() => void loadNextGroupMemberPage()}
                  disabled={!session || !groupMemberNextPageToken}
                >
                  下一页
                </button>
              </div>
            </div>
            <div className="group-member-list" aria-label="群成员列表">
              {groupMembersForActive.length === 0 && !groupMembersError ? (
                <div className="mini-empty">当前筛选下暂无成员数据</div>
              ) : null}
              {groupMembersForActive.map(member => (
                <article className="group-member-item" key={member.userID} data-testid="group-member-item">
                  <div className="group-member-copy">
                    <strong>{member.userID}</strong>
                    <span>
                      {memberRoleLabel(member.role)} · {memberStatusLabel(member.status)}
                    </span>
                  </div>
                  <div className="group-member-actions">
                    {member.role !== "ADMIN" ? (
                      <button
                        type="button"
                        onClick={() => void updateGroupMemberRole(member, "ADMIN")}
                        disabled={!session || member.status !== "ACTIVE" || member.role === "OWNER"}
                      >
                        设管理员
                      </button>
                    ) : null}
                    {member.role !== "MEMBER" ? (
                      <button
                        type="button"
                        onClick={() => void updateGroupMemberRole(member, "MEMBER")}
                        disabled={!session || member.status !== "ACTIVE" || member.role === "OWNER"}
                      >
                        设成员
                      </button>
                    ) : null}
                    {member.role !== "OWNER" ? (
                      <button
                        type="button"
                        onClick={() => void transferGroupOwner(member)}
                        disabled={!session || member.status !== "ACTIVE"}
                      >
                        转让群主
                      </button>
                    ) : null}
                    <button
                      className="danger-inline-button"
                      type="button"
                      onClick={() => void removeGroupMember(member)}
                      disabled={!session || member.status !== "ACTIVE" || member.userID === session?.userID}
                    >
                      移除
                    </button>
                  </div>
                </article>
              ))}
            </div>
          </section>
        ) : null}

        <section className="message-panel" aria-label="消息列表">
          <div className="message-list-header">
            <div>
              <h3>消息列表</h3>
              <p>
                {activeConversationID
                  ? `${messages.length} 条消息${latestMessageSeq > 0 ? ` / 最新 #${latestMessageSeq}` : ""}`
                  : session
                    ? "请选择左侧好友或群聊"
                    : "登录后即可同步消息"}
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
                <strong>{emptyState.title}</strong>
                <span>{emptyState.body}</span>
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

function memberRoleLabel(role: string): string {
  switch (role) {
    case "OWNER":
      return "群主";
    case "ADMIN":
      return "管理员";
    case "MEMBER":
      return "成员";
    default:
      return role || "未知角色";
  }
}

function memberStatusLabel(status: string): string {
  switch (status) {
    case "ACTIVE":
      return "活跃";
    case "LEFT":
      return "已离开";
    case "BANNED":
      return "已封禁";
    default:
      return status || "未知状态";
  }
}

function contactDisplayName(contact: ContactItem): string {
  return contact.remark.trim() || contact.contactUserID;
}

function mergeConversationSummaries(
  current: ConversationSummary[],
  incoming: ConversationSummary[]
): ConversationSummary[] {
  const currentByID = new Map(current.map(item => [item.conversationID, item]));
  return incoming.map(item => {
    const existing = currentByID.get(item.conversationID);
    if (!existing || !isGenericConversationTitle(item.title)) {
      return item;
    }
    if (isGenericConversationTitle(existing.title)) {
      return item;
    }
    return { ...item, title: existing.title };
  });
}

function conversationDisplayTitle(conversation: ConversationSummary): string {
  if (!isGenericConversationTitle(conversation.title)) {
    return conversation.title;
  }
  return titleFromConversationID(conversation.conversationID, conversation.type);
}

function titleFromConversationID(conversationID: string, type: ConversationSummary["type"]): string {
  const shortID = compactConversationID(conversationID);
  if (type === "DIRECT") {
    return `私聊 ${shortID}`;
  }
  return `群聊 ${shortID}`;
}

function conversationSubtitle(conversation: ConversationSummary): string {
  const kind = conversation.type === "DIRECT" ? "私聊" : "群聊";
  const seq = conversation.lastSeq > 0 ? `最新 #${conversation.lastSeq}` : "暂无消息";
  return `${kind} · ${seq}`;
}

function conversationStatusLabel(status: string): string {
  switch (status) {
    case "ACTIVE":
      return "活跃";
    case "ARCHIVED":
      return "已归档";
    case "DELETED":
      return "已删除";
    default:
      return status || "未知状态";
  }
}

function conversationAvatarText(conversation: ConversationSummary): string {
  const title = conversationDisplayTitle(conversation).trim();
  return (title.slice(0, 1) || "会").toUpperCase();
}

function isGenericConversationTitle(title: string): boolean {
  return title.trim() === "" || title.startsWith("Conversation ");
}

function compactConversationID(conversationID: string): string {
  if (conversationID.length <= 14) {
    return conversationID;
  }
  return `${conversationID.slice(0, 6)}...${conversationID.slice(-6)}`;
}

function emptyMessageState(hasSession: boolean, hasActiveConversation: boolean): { title: string; body: string } {
  if (!hasSession) {
    return {
      title: "登录后查看消息",
      body: "输入账号和密码即可进入 NexusIM。"
    };
  }
  if (!hasActiveConversation) {
    return {
      title: "请选择一个会话",
      body: "点击左侧好友发起私聊，或选择 / 创建群聊。"
    };
  }
  return {
    title: "这里还没有消息",
    body: "发送第一条消息，或点击同步从 PullInbox 拉取最新消息。"
  };
}

function chooseActiveConversationID(conversations: ConversationSummary[], preferredID: string): string {
  if (preferredID && conversations.some(conversation => conversation.conversationID === preferredID)) {
    return preferredID;
  }
  return conversations[0]?.conversationID ?? "";
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
    const code = "code" in error ? String((error as { code: unknown }).code) : "";
    const publicMessage = publicErrorMessage(code, message);
    if (publicMessage) {
      return publicMessage;
    }
    if (message === "Failed to fetch" || message.includes("NetworkError")) {
      return `无法连接本地 NexusIM 服务，请先启动客户端后端。API: ${runtimeConfig.apiBaseURL}，WebSocket: ${runtimeConfig.pushWebSocketURL}`;
    }
    return message;
  }
  return publicErrorMessage("", String(error)) ?? String(error);
}

function publicErrorMessage(code: string, message: string): string | null {
  const normalized = message.trim().toLowerCase();
  if (code === "UNAUTHENTICATED" || normalized.includes("token expired")) {
    return "登录态已过期，请重新登录。";
  }
  if (code === "PERMISSION_DENIED") {
    return "当前账号没有执行该操作的权限。";
  }
  if (code === "INVALID_ARGUMENT") {
    return `输入内容不符合要求：${message}`;
  }
  if (message === "login first") {
    return "请先登录。";
  }
  if (message === "logout first") {
    return "当前已有登录账号，请先退出后再注册。";
  }
  if (message === "account is required") {
    return "请输入账号。";
  }
  if (message === "password is required") {
    return "请输入密码。";
  }
  if (message === "conversation id is required") {
    return "请先选择一个会话。";
  }
  if (message === "contact user id is required") {
    return "请输入要添加的用户 ID。";
  }
  if (message === "contact is not active") {
    return "该联系人当前不是可聊天状态。";
  }
  if (normalized === "endpoint not found") {
    return "当前客户端接口在后端未启用，请重启最新本地客户端后端。";
  }
  return null;
}
