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
  pushToken?: string;
  refreshToken?: string;
  expiresAtMs?: number;
  pushExpiresAtMs?: number;
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

export interface RegisterRequest {
  tenantID: TenantID;
  userID: UserID;
  password: string;
}

export interface RegisterResponse {
  tenantID: TenantID;
  userID: UserID;
  status: string;
  createdAtMs: number;
}

export type ConversationType = "DIRECT" | "GROUP";
export type ConversationStatus = "ACTIVE" | "ARCHIVED" | "DELETED";

export interface CreateConversationRequest {
  tenantID: TenantID;
  conversationID?: ConversationID;
  type: ConversationType;
  idempotencyKey?: string;
}

export interface OpenDirectConversationRequest {
  peerUserID: UserID;
  idempotencyKey?: string;
}

export type MemberChangeType = "JOIN" | "LEAVE" | "REMOVE" | "ROLE_CHANGED" | "OWNER_TRANSFER" | string;
export type MemberRole = "OWNER" | "ADMIN" | "MEMBER" | "UNSPECIFIED" | string;
export type MemberStatus = "ACTIVE" | "LEFT" | "BANNED" | "UNSPECIFIED" | string;
export type MemberChangeStatus =
  | "PENDING_BOUNDARY"
  | "BOUNDARY_ALLOCATED"
  | "MEMBER_UPDATED"
  | "OUTBOX_ENQUEUED"
  | "EVENT_PUBLISHED"
  | "DONE"
  | "FAILED_COMPENSATED"
  | string;

export interface InviteConversationMemberRequest {
  conversationID: ConversationID;
  targetUserID: UserID;
  expectedMemberVersion?: number;
  idempotencyKey?: string;
  reason?: string;
}

export interface LeaveConversationRequest {
  conversationID: ConversationID;
  expectedMemberVersion?: number;
  idempotencyKey?: string;
  reason?: string;
}

export interface ListConversationMembersRequest {
  conversationID: ConversationID;
  pageSize?: number;
  pageToken?: string;
  roleFilter?: MemberRole;
  userIDPrefix?: string;
}

export interface ConversationMember {
  userID: UserID;
  role: MemberRole;
  status: MemberStatus;
  joinSeq: number;
  leaveSeq: number;
  memberVersion: number;
  permissionVersion: number;
  updatedAtMs: number;
}

export interface ListConversationMembersResponse {
  tenantID: TenantID;
  conversationID: ConversationID;
  memberVersion: number;
  permissionVersion: number;
  members: ConversationMember[];
  nextPageToken: string;
}

export interface RemoveConversationMemberRequest {
  conversationID: ConversationID;
  targetUserID: UserID;
  expectedMemberVersion?: number;
  idempotencyKey?: string;
  reason?: string;
}

export interface UpdateConversationMemberRoleRequest {
  conversationID: ConversationID;
  targetUserID: UserID;
  targetRole: Exclude<MemberRole, "OWNER">;
  expectedMemberVersion?: number;
  idempotencyKey?: string;
  reason?: string;
}

export interface TransferConversationOwnerRequest {
  conversationID: ConversationID;
  newOwnerUserID: UserID;
  expectedMemberVersion?: number;
  idempotencyKey?: string;
  reason?: string;
}

export interface TransferConversationOwnerResponse {
  changeID: string;
  tenantID: TenantID;
  conversationID: ConversationID;
  previousOwnerUserID: UserID;
  newOwnerUserID: UserID;
  status: MemberChangeStatus;
  boundarySeq: number;
  memberVersion: number;
  permissionVersion: number;
  idempotentReplay: boolean;
}

export interface ConversationMemberChangeResponse {
  changeID: string;
  tenantID: TenantID;
  conversationID: ConversationID;
  targetUserID: UserID;
  changeType: MemberChangeType;
  status: MemberChangeStatus;
  boundarySeq: number;
  memberVersion: number;
  permissionVersion: number;
  idempotentReplay: boolean;
}

export interface CreateConversationResponse {
  tenantID: TenantID;
  conversationID: ConversationID;
  type: ConversationType;
  directPeerUserID?: UserID;
  boundarySeq: number;
  memberVersion: number;
  permissionVersion: number;
  idempotentReplay: boolean;
}

export interface ConversationSummary {
  tenantID: TenantID;
  conversationID: ConversationID;
  type: ConversationType;
  status: ConversationStatus;
  title: string;
  lastSeq: number;
  memberVersion: number;
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

export type ContactRequestStatus =
  | "PENDING"
  | "ACCEPTED"
  | "DECLINED"
  | "CANCELED"
  | "EXPIRED"
  | "REVIEW_REQUIRED"
  | string;

export type ContactRequestDirection = "INCOMING" | "OUTGOING";
export type ContactDecision = "ACCEPT" | "DECLINE";
export type ContactRequestSourceType = "DIRECT" | "SEARCH" | "GROUP" | "INVITE_LINK" | "QR_CODE" | "IMPORT";
export type ContactRequestRiskLevel = "LOW" | "MEDIUM" | "HIGH" | string;

export interface ContactRequestItem {
  requestID: string;
  senderUserID: UserID;
  receiverUserID: UserID;
  status: ContactRequestStatus;
  message: string;
  createdAtMs: number;
  updatedAtMs: number;
  decidedAtMs: number;
  sourceType: ContactRequestSourceType | string;
  sourceRef: string;
  riskLevel: ContactRequestRiskLevel;
  reviewRequired: boolean;
}

export interface ListContactsInput {
  pageSize?: number;
  pageToken?: string;
  query?: string;
  groupName?: string;
}

export interface ListContactRequestsInput {
  direction: ContactRequestDirection;
  status?: ContactRequestStatus;
  pageSize?: number;
  pageToken?: string;
}

export interface SendContactRequestInput {
  targetUserID: UserID;
  message?: string;
  sourceType?: ContactRequestSourceType;
  sourceRef?: string;
  idempotencyKey?: string;
}

export interface RespondContactRequestInput {
  requestID: string;
  decision: ContactDecision;
  idempotencyKey?: string;
}

export interface CancelContactRequestInput {
  requestID: string;
  idempotencyKey?: string;
}

export interface ContactActionInput {
  contactUserID: UserID;
  idempotencyKey?: string;
  reason?: string;
  remark?: string;
  groupName?: string;
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
