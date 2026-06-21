export type TenantID = string;
export type UserID = string;
export type DeviceID = string;
export type SessionID = string;
export type ConversationID = string;
export type MessageID = string;
export type EventID = string;

export type PublicErrorCode =
  | "UNAUTHENTICATED"
  | "PERMISSION_DENIED"
  | "INVALID_ARGUMENT"
  | "FAILED_PRECONDITION"
  | "CONFLICT"
  | "RATE_LIMITED"
  | "SERVER_BUSY"
  | "UNAVAILABLE";

export interface PublicError {
  code: PublicErrorCode;
  message: string;
  retryable: boolean;
  requestID?: string;
}

export interface AuthSession {
  tenantID: TenantID;
  userID: UserID;
  deviceID: DeviceID;
  sessionID: SessionID;
  accessToken: string;
  refreshToken?: string;
  expiresAtMs?: number;
}

export interface LoginRequest {
  tenantID: TenantID;
  userID: UserID;
  password: string;
  deviceID: DeviceID;
  mfaFactorID?: string;
  mfaCode?: string;
  recoveryCode?: string;
}

export interface LoginResponse {
  session: AuthSession;
}

export type ConversationType = "DIRECT" | "GROUP";
export type ConversationStatus = "ACTIVE" | "ARCHIVED" | "DELETED";

export interface ConversationSummary {
  tenantID: TenantID;
  conversationID: ConversationID;
  type: ConversationType;
  status: ConversationStatus;
  title: string;
  lastSeq: number;
  unreadCount: number;
  muted: boolean;
  pinned: boolean;
  updatedAtMs: number;
}

export interface ContactItem {
  contactUserID: UserID;
  status: string;
  remark: string;
  groupName: string;
  updatedAtMs: number;
}

export type MessageStatus = "PENDING" | "SENT" | "DELIVERED" | "FAILED";

export interface MessageItem {
  tenantID: TenantID;
  conversationID: ConversationID;
  messageID: MessageID;
  senderUserID: UserID;
  conversationSeq: number;
  contentType: "TEXT";
  text: string;
  sourceEventID?: EventID;
  clientMessageID?: string;
  status: MessageStatus;
  createdAtMs: number;
}

export interface PullInboxRequest {
  tenantID: TenantID;
  userID: UserID;
  deviceID: DeviceID;
  conversationID: ConversationID;
  afterSeq: number;
  limit: number;
}

export interface PullInboxResponse {
  conversationID: ConversationID;
  items: MessageItem[];
  nextSeq: number;
}

export interface SendMessageRequest {
  tenantID: TenantID;
  conversationID: ConversationID;
  senderUserID: UserID;
  clientMessageID: string;
  idempotencyKey: string;
  text: string;
}

export interface SendMessageResponse {
  messageID: MessageID;
  conversationID: ConversationID;
  conversationSeq: number;
}

export interface AckDeliveryRequest {
  tenantID: TenantID;
  userID: UserID;
  deviceID: DeviceID;
  conversationID: ConversationID;
  lastReceivedSeq: number;
  requestID: string;
}

export interface AckDeliveryResponse {
  conversationID: ConversationID;
  lastReceivedSeq: number;
}

export interface ClientHelloFrame {
  op: "client.hello";
  request_id: string;
  tenant_id: TenantID;
  user_id: UserID;
  device_id: DeviceID;
  session_id: SessionID;
  resume_token?: string;
}

export interface ServerHelloFrame {
  op: "server.hello";
  request_id: string;
  session_id: SessionID;
  resume_token?: string;
  server_time_ms: number;
}

export interface DeliveryNotifyFrame {
  op: "delivery.notify";
  event_id: EventID;
  tenant_id: TenantID;
  user_id: UserID;
  conversation_id: ConversationID;
  conversation_seq: number;
  message_id?: MessageID;
  source_event_id?: EventID;
  trace_id?: string;
}

export interface DeliveryAckFrame {
  op: "delivery.ack";
  request_id: string;
  conversation_id: ConversationID;
  last_received_seq: number;
}

export interface DeliveryAckOKFrame {
  op: "delivery.ack.ok";
  request_id: string;
  conversation_id: ConversationID;
  last_received_seq: number;
}

export interface ServerResumeHintFrame {
  op: "server.resume_hint";
  reason: string;
  conversations?: Array<{
    conversation_id: ConversationID;
    seq: number;
  }>;
}

export interface ClientPingFrame {
  op: "client.ping";
  request_id: string;
  client_time_ms: number;
}

export interface ServerPongFrame {
  op: "server.pong";
  request_id: string;
  server_time_ms: number;
}

export interface ErrorFrame {
  op: "error";
  request_id?: string;
  code: PublicErrorCode;
  message: string;
  retryable: boolean;
}

export type ClientFrame = ClientHelloFrame | DeliveryAckFrame | ClientPingFrame;

export type ServerFrame =
  | ServerHelloFrame
  | DeliveryNotifyFrame
  | DeliveryAckOKFrame
  | ServerResumeHintFrame
  | ServerPongFrame
  | ErrorFrame;
