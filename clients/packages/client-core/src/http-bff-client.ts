import {
  CLIENT_API_ENDPOINTS,
  type AckDeliveryRequest,
  type AckDeliveryResponse,
  type AuthSession,
  type ContactItem,
  type ConversationSummary,
  type LoginRequest,
  type LoginResponse,
  type MessageItem,
  type PublicError,
  type PublicErrorCode,
  type PullInboxRequest,
  type PullInboxResponse,
  type SendMessageRequest,
  type SendMessageResponse
} from "@nexusim/protocol";
import type { AuthAPI, DeliveryAPI, MessagingAPI } from "./ports";

interface BFFErrorPayload {
  error?: {
    code?: string;
    message?: string;
  };
}

interface BFFLoginResponse {
  tenant_id?: string;
  user_id?: string;
  device_id?: string;
  session_id?: string;
  token_type?: string;
  gateway_token?: string;
  push_gateway_token?: string;
  refresh_token?: string;
  gateway_expires_at_unix_ms?: string | number;
  push_gateway_expires_at_unix_ms?: string | number;
}

interface BFFConversationSummary {
  conversation_id?: string;
  last_visible_seq?: string | number;
  last_message_id?: string;
  last_sender_id?: string;
  unread_count?: string | number;
  updated_at_unix_ms?: string | number;
  archived?: boolean;
  pinned?: boolean;
  muted?: boolean;
}

interface BFFListConversationsResponse {
  items?: BFFConversationSummary[];
  next_page_cursor?: string;
}

interface BFFInboxItem {
  conversation_id?: string;
  conversation_seq?: string | number;
  event_id?: string;
  event_type?: string;
  message_id?: string;
  sender_id?: string;
  payload_json?: string;
  created_at_unix_ms?: string | number;
}

interface BFFPullInboxResponse {
  items?: BFFInboxItem[];
  next_seq?: string | number;
}

interface BFFSendMessageResponse {
  message_id?: string;
  conversation_id?: string;
  conversation_seq?: string | number;
}

interface BFFAckDeliveryResponse {
  conversation_id?: string;
  last_received_seq?: string | number;
}

interface BFFContactItem {
  contact_user_id?: string;
  status?: string;
  updated_at_unix_ms?: string | number;
  remark?: string;
  group_name?: string;
}

interface BFFListContactsResponse {
  contacts?: BFFContactItem[];
}

export class BFFClient implements AuthAPI, MessagingAPI, DeliveryAPI {
  readonly #apiBaseURL: string;

  constructor(apiBaseURL: string) {
    this.#apiBaseURL = apiBaseURL.replace(/\/+$/, "");
  }

  async login(request: LoginRequest): Promise<LoginResponse> {
    const response = await this.#request<BFFLoginResponse>("POST", CLIENT_API_ENDPOINTS.login, {
      tenant_id: request.tenantID,
      user_id: request.userID,
      password: request.password,
      device_id: request.deviceID,
      mfa_factor_id: request.mfaFactorID,
      mfa_code: request.mfaCode,
      mfa_recovery_code: request.recoveryCode
    });
    return { session: loginResponseToSession(response) };
  }

  async refresh(session: AuthSession): Promise<LoginResponse> {
    if (!session.refreshToken) {
      throw publicError("FAILED_PRECONDITION", "refresh token is missing", false);
    }
    const response = await this.#request<BFFLoginResponse>("POST", CLIENT_API_ENDPOINTS.refresh, {
      tenant_id: session.tenantID,
      user_id: session.userID,
      device_id: session.deviceID,
      refresh_token: session.refreshToken
    });
    return { session: loginResponseToSession(response) };
  }

  async logout(session: AuthSession): Promise<void> {
    const response = await fetch(this.#url(CLIENT_API_ENDPOINTS.logout), {
      method: "POST",
      headers: this.#headers(session)
    });
    if (response.status === 501) {
      return;
    }
    await this.#parseResponse<unknown>(response);
  }

  async listConversations(
    session: AuthSession,
    input: { limit?: number; pageCursor?: string } = {}
  ): Promise<ConversationSummary[]> {
    const query = new URLSearchParams();
    if (input.limit && input.limit > 0) {
      query.set("limit", String(input.limit));
    }
    if (input.pageCursor) {
      query.set("page_cursor", input.pageCursor);
    }
    const suffix = query.toString();
    const response = await this.#request<BFFListConversationsResponse>(
      "GET",
      suffix ? `${CLIENT_API_ENDPOINTS.conversations}?${suffix}` : CLIENT_API_ENDPOINTS.conversations,
      undefined,
      session
    );
    return (response.items ?? []).map(item => conversationSummaryFromBFF(item, session));
  }

  async sendMessage(request: SendMessageRequest, session: AuthSession): Promise<SendMessageResponse> {
    const response = await this.#request<BFFSendMessageResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.sendMessage,
      {
        conversation_id: request.conversationID,
        client_msg_id: request.clientMessageID,
        message_type: "TEXT",
        payload: { text: request.text },
        attachment_ids: []
      },
      session
    );
    return {
      messageID: requiredString(response.message_id, "message_id"),
      conversationID: requiredString(response.conversation_id, "conversation_id"),
      conversationSeq: numberValue(response.conversation_seq)
    };
  }

  async pullInbox(request: PullInboxRequest, session: AuthSession): Promise<PullInboxResponse> {
    const query = new URLSearchParams({
      after_seq: String(request.afterSeq),
      limit: String(request.limit)
    });
    const response = await this.#request<BFFPullInboxResponse>(
      "GET",
      `${CLIENT_API_ENDPOINTS.conversationMessages(request.conversationID)}?${query.toString()}`,
      undefined,
      session
    );
    return {
      conversationID: request.conversationID,
      items: (response.items ?? []).map(item => inboxItemFromBFF(item, session)),
      nextSeq: numberValue(response.next_seq)
    };
  }

  async ackDelivery(request: AckDeliveryRequest, session: AuthSession): Promise<AckDeliveryResponse> {
    const response = await this.#request<BFFAckDeliveryResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.ackDelivery,
      {
        conversation_id: request.conversationID,
        received_seq: request.lastReceivedSeq
      },
      session
    );
    return {
      conversationID: requiredString(response.conversation_id, "conversation_id"),
      lastReceivedSeq: numberValue(response.last_received_seq)
    };
  }

  async listContacts(session: AuthSession): Promise<ContactItem[]> {
    const response = await this.#request<BFFListContactsResponse>("GET", CLIENT_API_ENDPOINTS.contacts, undefined, session);
    return (response.contacts ?? []).map(contactFromBFF);
  }

  async #request<T>(method: "GET" | "POST", path: string, body?: unknown, session?: AuthSession): Promise<T> {
    const init: RequestInit = {
      method,
      headers: this.#headers(session, body !== undefined)
    };
    if (body !== undefined) {
      init.body = JSON.stringify(stripUndefined(body));
    }
    const response = await fetch(this.#url(path), init);
    return this.#parseResponse<T>(response);
  }

  async #parseResponse<T>(response: Response): Promise<T> {
    const text = await response.text();
    const payload = text ? (JSON.parse(text) as unknown) : undefined;
    if (!response.ok) {
      const errorPayload = payload as BFFErrorPayload | undefined;
      throw publicError(
        publicErrorCodeFromBFF(errorPayload?.error?.code),
        errorPayload?.error?.message ?? `HTTP ${response.status}`,
        response.status === 429 || response.status >= 500
      );
    }
    return payload as T;
  }

  #headers(session?: AuthSession, hasBody = false): HeadersInit {
    const headers: Record<string, string> = {};
    if (hasBody) {
      headers["Content-Type"] = "application/json";
    }
    if (session?.accessToken) {
      headers.Authorization = `Bearer ${session.accessToken}`;
      headers["X-NexusIM-Gateway-Token"] = session.accessToken;
    }
    return headers;
  }

  #url(path: string): string {
    return new URL(path, `${this.#apiBaseURL}/`).toString();
  }
}

function loginResponseToSession(response: BFFLoginResponse): AuthSession {
  const session: AuthSession = {
    tenantID: requiredString(response.tenant_id, "tenant_id"),
    userID: requiredString(response.user_id, "user_id"),
    deviceID: requiredString(response.device_id, "device_id"),
    sessionID: requiredString(response.session_id, "session_id"),
    accessToken: requiredString(response.gateway_token, "gateway_token")
  };
  if (response.refresh_token) {
    session.refreshToken = response.refresh_token;
  }
  if (response.push_gateway_token) {
    session.pushToken = response.push_gateway_token;
  }
  const expiresAtMs = numberValue(response.gateway_expires_at_unix_ms);
  if (expiresAtMs > 0) {
    session.expiresAtMs = expiresAtMs;
  }
  const pushExpiresAtMs = numberValue(response.push_gateway_expires_at_unix_ms);
  if (pushExpiresAtMs > 0) {
    session.pushExpiresAtMs = pushExpiresAtMs;
  }
  return session;
}

function conversationSummaryFromBFF(item: BFFConversationSummary, session: AuthSession): ConversationSummary {
  const archived = item.archived === true;
  return {
    tenantID: session.tenantID,
    conversationID: requiredString(item.conversation_id, "conversation_id"),
    type: "GROUP",
    status: archived ? "ARCHIVED" : "ACTIVE",
    title: item.conversation_id ? `Conversation ${item.conversation_id}` : "Conversation",
    lastSeq: numberValue(item.last_visible_seq),
    unreadCount: numberValue(item.unread_count),
    muted: item.muted === true,
    pinned: item.pinned === true,
    updatedAtMs: numberValue(item.updated_at_unix_ms)
  };
}

function inboxItemFromBFF(item: BFFInboxItem, session: AuthSession): MessageItem {
  const payload = decodePayloadJSON(item.payload_json);
  const message: MessageItem = {
    tenantID: session.tenantID,
    conversationID: requiredString(item.conversation_id, "conversation_id"),
    messageID: requiredString(item.message_id, "message_id"),
    senderUserID: requiredString(item.sender_id, "sender_id"),
    conversationSeq: numberValue(item.conversation_seq),
    contentType: "TEXT",
    text: textFromPayload(payload),
    status: "DELIVERED",
    createdAtMs: numberValue(item.created_at_unix_ms)
  };
  if (item.event_id) {
    message.sourceEventID = item.event_id;
  }
  return message;
}

function contactFromBFF(item: BFFContactItem): ContactItem {
  return {
    contactUserID: requiredString(item.contact_user_id, "contact_user_id"),
    status: item.status ?? "CONTACT_EDGE_STATUS_UNSPECIFIED",
    remark: item.remark ?? "",
    groupName: item.group_name ?? "",
    updatedAtMs: numberValue(item.updated_at_unix_ms)
  };
}

function decodePayloadJSON(value: string | undefined): unknown {
  if (!value) {
    return {};
  }
  try {
    return JSON.parse(atob(value));
  } catch {
    try {
      return JSON.parse(value);
    } catch {
      return {};
    }
  }
}

function textFromPayload(payload: unknown): string {
  if (typeof payload !== "object" || payload === null) {
    return "";
  }
  const candidate = (payload as { text?: unknown }).text;
  return typeof candidate === "string" ? candidate : JSON.stringify(payload);
}

function publicErrorCodeFromBFF(code: string | undefined): PublicErrorCode {
  switch (code) {
    case "Unauthenticated":
      return "UNAUTHENTICATED";
    case "PermissionDenied":
      return "PERMISSION_DENIED";
    case "InvalidArgument":
      return "INVALID_ARGUMENT";
    case "FailedPrecondition":
      return "FAILED_PRECONDITION";
    case "AlreadyExists":
    case "Aborted":
      return "CONFLICT";
    case "ResourceExhausted":
      return "RATE_LIMITED";
    case "Unavailable":
      return "UNAVAILABLE";
    default:
      return "SERVER_BUSY";
  }
}

function publicError(code: PublicErrorCode, message: string, retryable: boolean): PublicError {
  return { code, message, retryable };
}

function requiredString(value: string | undefined, field: string): string {
  if (!value) {
    throw publicError("SERVER_BUSY", `BFF response missing ${field}`, false);
  }
  return value;
}

function numberValue(value: string | number | undefined): number {
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : 0;
  }
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function stripUndefined(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(stripUndefined);
  }
  if (typeof value !== "object" || value === null) {
    return value;
  }
  const output: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value)) {
    if (item !== undefined) {
      output[key] = stripUndefined(item);
    }
  }
  return output;
}
