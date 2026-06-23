import {
  CLIENT_API_ENDPOINTS,
  type AckDeliveryRequest,
  type AckDeliveryResponse,
  type AuthSession,
  type CancelContactRequestInput,
  type ContactActionInput,
  type ContactDecision,
  type ContactItem,
  type ContactRequestItem,
  type CreateConversationRequest,
  type CreateConversationResponse,
  type CreateGroupAvatarUploadSessionRequest,
  type CompleteGroupAvatarUploadRequest,
  type CompleteGroupAvatarUploadResponse,
  type ConversationMember,
  type ConversationMemberChangeResponse,
  type ConversationProfile,
  type GroupAvatarUploadSession,
  type ListConversationMembersRequest,
  type ListConversationMembersResponse,
  type ListContactRequestsInput,
  type ListContactsInput,
  type InviteConversationMemberRequest,
  type LeaveConversationRequest,
  type RemoveConversationMemberRequest,
  type RespondContactRequestInput,
  type SendContactRequestInput,
  type TransferConversationOwnerRequest,
  type TransferConversationOwnerResponse,
  type UpdateConversationProfileRequest,
  type UpdateConversationMemberRoleRequest,
  type ConversationSummary,
  type LoginRequest,
  type LoginResponse,
  type MessageItem,
  type OpenDirectConversationRequest,
  type PublicError,
  type PublicErrorCode,
  type PullInboxRequest,
  type PullInboxResponse,
  type RegisterRequest,
  type RegisterResponse,
  type SendMessageRequest,
  type SendMessageResponse
} from "@nexusim/protocol";
import type { AuthAPI, ConversationAPI, DeliveryAPI, MessagingAPI } from "./ports";

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

interface BFFRegisterResponse {
  tenant_id?: string;
  user_id?: string;
  status?: string;
  created_at_unix_ms?: string | number;
}

interface BFFCreateConversationResponse {
  tenant_id?: string;
  conversation_id?: string;
  conversation_type?: string;
  direct_peer_user_id?: string;
  boundary_seq?: string | number;
  member_version?: string | number;
  permission_version?: string | number;
  idempotent_replay?: boolean;
}

interface BFFConversationMemberChangeResponse {
  change_id?: string;
  tenant_id?: string;
  conversation_id?: string;
  target_user_id?: string;
  change_type?: string;
  status?: string;
  boundary_seq?: string | number;
  member_version?: string | number;
  permission_version?: string | number;
  idempotent_replay?: boolean;
}

interface BFFConversationMember {
  user_id?: string;
  role?: string;
  status?: string;
  join_seq?: string | number;
  leave_seq?: string | number;
  member_version?: string | number;
  permission_version?: string | number;
  updated_at_unix_ms?: string | number;
}

interface BFFListConversationMembersResponse {
  tenant_id?: string;
  conversation_id?: string;
  member_version?: string | number;
  permission_version?: string | number;
  members?: BFFConversationMember[];
  next_page_token?: string;
}

interface BFFTransferConversationOwnerResponse {
  change_id?: string;
  tenant_id?: string;
  conversation_id?: string;
  previous_owner_user_id?: string;
  new_owner_user_id?: string;
  status?: string;
  boundary_seq?: string | number;
  member_version?: string | number;
  permission_version?: string | number;
  idempotent_replay?: boolean;
}

interface BFFConversationProfile {
  tenant_id?: string;
  conversation_id?: string;
  conversation_type?: string;
  title?: string;
  avatar_uri?: string;
  profile_version?: string | number;
  member_version?: string | number;
  permission_version?: string | number;
  updated_at_unix_ms?: string | number;
}

interface BFFConversationProfileResponse {
  profile?: BFFConversationProfile;
}

interface BFFGroupAvatarUploadSessionResponse {
  asset_id?: string;
  upload_session_id?: string;
  upload_url?: string;
  required_headers?: Record<string, string>;
  expires_at_unix_ms?: string | number;
  max_size_bytes?: string | number;
  accepted_content_types?: string[];
}

interface BFFGroupAvatarUploadCompleteResponse {
  asset_id?: string;
  avatar_uri?: string;
  profile?: BFFConversationProfile;
}

interface BFFConversationSummary {
  conversation_id?: string;
  conversation_type?: string;
  title?: string;
  last_visible_seq?: string | number;
  last_message_id?: string;
  last_sender_id?: string;
  member_version?: string | number;
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

interface BFFContactRequestItem {
  request_id?: string;
  sender_user_id?: string;
  receiver_user_id?: string;
  status?: string;
  message?: string;
  created_at_unix_ms?: string | number;
  updated_at_unix_ms?: string | number;
  decided_at_unix_ms?: string | number;
  source_type?: string;
  source_ref?: string;
  risk_level?: string;
  review_required?: boolean;
}

interface BFFListContactRequestsResponse {
  requests?: BFFContactRequestItem[];
}

export class BFFClient implements AuthAPI, ConversationAPI, MessagingAPI, DeliveryAPI {
  readonly #apiBaseURL: string;

  constructor(apiBaseURL: string) {
    this.#apiBaseURL = apiBaseURL.replace(/\/+$/, "");
  }

  async register(request: RegisterRequest): Promise<RegisterResponse> {
    const response = await this.#request<BFFRegisterResponse>("POST", CLIENT_API_ENDPOINTS.register, {
      tenant_id: request.tenantID,
      user_id: request.userID,
      password: request.password
    });
    return {
      tenantID: requiredString(response.tenant_id, "tenant_id"),
      userID: requiredString(response.user_id, "user_id"),
      status: trimEnumPrefix(response.status, "USER_STATUS_") || "UNSPECIFIED",
      createdAtMs: numberValue(response.created_at_unix_ms)
    };
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

  async createConversation(
    request: CreateConversationRequest,
    session: AuthSession
  ): Promise<CreateConversationResponse> {
    const conversationID = request.conversationID ?? newClientID("group");
    const response = await this.#request<BFFCreateConversationResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.createConversation,
      {
        conversation_id: conversationID,
        conversation_type: conversationTypeToBFF(request.type),
        idempotency_key: request.idempotencyKey ?? newClientID("create-conversation")
      },
      session
    );
    return {
      tenantID: requiredString(response.tenant_id, "tenant_id"),
      conversationID: requiredString(response.conversation_id, "conversation_id"),
      type: conversationTypeFromBFF(response.conversation_type),
      boundarySeq: numberValue(response.boundary_seq),
      memberVersion: numberValue(response.member_version),
      permissionVersion: numberValue(response.permission_version),
      idempotentReplay: response.idempotent_replay === true,
      ...optionalDirectPeerUserID(response)
    };
  }

  async openDirectConversation(
    request: OpenDirectConversationRequest,
    session: AuthSession
  ): Promise<CreateConversationResponse> {
    const response = await this.#request<BFFCreateConversationResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.directConversation,
      {
        peer_user_id: request.peerUserID,
        idempotency_key: request.idempotencyKey ?? newClientID(`direct-${request.peerUserID}`)
      },
      session
    );
    return {
      tenantID: requiredString(response.tenant_id, "tenant_id"),
      conversationID: requiredString(response.conversation_id, "conversation_id"),
      type: conversationTypeFromBFF(response.conversation_type),
      boundarySeq: numberValue(response.boundary_seq),
      memberVersion: numberValue(response.member_version),
      permissionVersion: numberValue(response.permission_version),
      idempotentReplay: response.idempotent_replay === true,
      ...optionalDirectPeerUserID(response)
    };
  }

  async listConversationMembers(
    request: ListConversationMembersRequest,
    session: AuthSession
  ): Promise<ListConversationMembersResponse> {
    const query = new URLSearchParams();
    if (request.pageSize && request.pageSize > 0) {
      query.set("page_size", String(request.pageSize));
    }
    if (request.pageToken) {
      query.set("page_token", request.pageToken);
    }
    if (request.roleFilter) {
      query.set("role", request.roleFilter);
    }
    if (request.userIDPrefix) {
      query.set("user_id_prefix", request.userIDPrefix);
    }
    const suffix = query.toString();
    const endpoint = CLIENT_API_ENDPOINTS.conversationMembers(request.conversationID);
    const response = await this.#request<BFFListConversationMembersResponse>(
      "GET",
      suffix ? `${endpoint}?${suffix}` : endpoint,
      undefined,
      session
    );
    return listConversationMembersFromBFF(response);
  }

  async inviteConversationMember(
    request: InviteConversationMemberRequest,
    session: AuthSession
  ): Promise<ConversationMemberChangeResponse> {
    const response = await this.#request<BFFConversationMemberChangeResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.inviteConversationMember(request.conversationID),
      {
        target_user_id: request.targetUserID,
        expected_member_version: request.expectedMemberVersion,
        idempotency_key: request.idempotencyKey ?? newClientID("member-invite"),
        reason: request.reason
      },
      session
    );
    return memberChangeFromBFF(response);
  }

  async leaveConversation(
    request: LeaveConversationRequest,
    session: AuthSession
  ): Promise<ConversationMemberChangeResponse> {
    const response = await this.#request<BFFConversationMemberChangeResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.leaveConversation(request.conversationID),
      {
        expected_member_version: request.expectedMemberVersion,
        idempotency_key: request.idempotencyKey ?? newClientID("member-leave"),
        reason: request.reason
      },
      session
    );
    return memberChangeFromBFF(response);
  }

  async removeConversationMember(
    request: RemoveConversationMemberRequest,
    session: AuthSession
  ): Promise<ConversationMemberChangeResponse> {
    const response = await this.#request<BFFConversationMemberChangeResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.removeConversationMember(request.conversationID),
      {
        target_user_id: request.targetUserID,
        expected_member_version: request.expectedMemberVersion,
        idempotency_key: request.idempotencyKey ?? newClientID("member-remove"),
        reason: request.reason
      },
      session
    );
    return memberChangeFromBFF(response);
  }

  async updateConversationMemberRole(
    request: UpdateConversationMemberRoleRequest,
    session: AuthSession
  ): Promise<ConversationMemberChangeResponse> {
    const response = await this.#request<BFFConversationMemberChangeResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.updateConversationMemberRole(request.conversationID),
      {
        target_user_id: request.targetUserID,
        target_role: memberRoleToBFF(request.targetRole),
        expected_member_version: request.expectedMemberVersion,
        idempotency_key: request.idempotencyKey ?? newClientID("member-role"),
        reason: request.reason
      },
      session
    );
    return memberChangeFromBFF(response);
  }

  async transferConversationOwner(
    request: TransferConversationOwnerRequest,
    session: AuthSession
  ): Promise<TransferConversationOwnerResponse> {
    const response = await this.#request<BFFTransferConversationOwnerResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.transferConversationOwner(request.conversationID),
      {
        new_owner_user_id: request.newOwnerUserID,
        expected_member_version: request.expectedMemberVersion,
        idempotency_key: request.idempotencyKey ?? newClientID("owner-transfer"),
        reason: request.reason
      },
      session
    );
    return transferOwnerFromBFF(response);
  }

  async getConversationProfile(conversationID: string, session: AuthSession): Promise<ConversationProfile> {
    const response = await this.#request<BFFConversationProfileResponse>(
      "GET",
      CLIENT_API_ENDPOINTS.conversationProfile(conversationID),
      undefined,
      session
    );
    return conversationProfileFromBFF(requiredObject(response.profile, "profile"));
  }

  async updateConversationProfile(
    request: UpdateConversationProfileRequest,
    session: AuthSession
  ): Promise<ConversationProfile> {
    const response = await this.#request<BFFConversationProfileResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.conversationProfile(request.conversationID),
      {
        title: request.title,
        avatar_uri: request.avatarURI,
        expected_profile_version: request.expectedProfileVersion
      },
      session
    );
    return conversationProfileFromBFF(requiredObject(response.profile, "profile"));
  }

  async createGroupAvatarUploadSession(
    request: CreateGroupAvatarUploadSessionRequest,
    session: AuthSession
  ): Promise<GroupAvatarUploadSession> {
    const response = await this.#request<BFFGroupAvatarUploadSessionResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.groupAvatarUploadSession(request.conversationID),
      {
        file_name: request.fileName,
        content_type: request.contentType,
        size_bytes: request.sizeBytes,
        sha256: request.sha256,
        idempotency_key: request.idempotencyKey ?? newClientID("group-avatar-upload")
      },
      session
    );
    return groupAvatarUploadSessionFromBFF(response);
  }

  async completeGroupAvatarUpload(
    request: CompleteGroupAvatarUploadRequest,
    session: AuthSession
  ): Promise<CompleteGroupAvatarUploadResponse> {
    const response = await this.#request<BFFGroupAvatarUploadCompleteResponse>(
      "POST",
      CLIENT_API_ENDPOINTS.groupAvatarUploadComplete(request.conversationID),
      {
        asset_id: request.assetID,
        upload_session_id: request.uploadSessionID,
        sha256: request.sha256,
        size_bytes: request.sizeBytes,
        expected_profile_version: request.expectedProfileVersion
      },
      session
    );
    return {
      assetID: requiredString(response.asset_id, "asset_id"),
      avatarURI: requiredString(response.avatar_uri, "avatar_uri"),
      profile: conversationProfileFromBFF(requiredObject(response.profile, "profile"))
    };
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

  async listContacts(session: AuthSession, input: ListContactsInput = {}): Promise<ContactItem[]> {
    const query = new URLSearchParams();
    if (input.pageSize && input.pageSize > 0) {
      query.set("page_size", String(input.pageSize));
    }
    if (input.pageToken) {
      query.set("page_token", input.pageToken);
    }
    if (input.query) {
      query.set("query", input.query);
    }
    if (input.groupName) {
      query.set("group_name", input.groupName);
    }
    const suffix = query.toString();
    const response = await this.#request<BFFListContactsResponse>(
      "GET",
      suffix ? `${CLIENT_API_ENDPOINTS.contacts}?${suffix}` : CLIENT_API_ENDPOINTS.contacts,
      undefined,
      session
    );
    return (response.contacts ?? []).map(contactFromBFF);
  }

  async listContactRequests(session: AuthSession, input: ListContactRequestsInput): Promise<ContactRequestItem[]> {
    const query = new URLSearchParams({ direction: input.direction });
    if (input.status) {
      query.set("status", input.status);
    }
    if (input.pageSize && input.pageSize > 0) {
      query.set("page_size", String(input.pageSize));
    }
    if (input.pageToken) {
      query.set("page_token", input.pageToken);
    }
    const response = await this.#request<BFFListContactRequestsResponse>(
      "GET",
      `${CLIENT_API_ENDPOINTS.contactRequests}?${query.toString()}`,
      undefined,
      session
    );
    return (response.requests ?? []).map(contactRequestFromBFF);
  }

  async sendContactRequest(session: AuthSession, input: SendContactRequestInput): Promise<void> {
    await this.#request<unknown>(
      "POST",
      CLIENT_API_ENDPOINTS.sendContactRequest,
      {
        target_user_id: input.targetUserID,
        idempotency_key: input.idempotencyKey ?? newClientID("contact-request"),
        message: input.message,
        source_type: contactSourceTypeToBFF(input.sourceType ?? "DIRECT"),
        source_ref: input.sourceRef
      },
      session
    );
  }

  async respondContactRequest(session: AuthSession, input: RespondContactRequestInput): Promise<void> {
    await this.#request<unknown>(
      "POST",
      CLIENT_API_ENDPOINTS.respondContactRequest,
      {
        request_id: input.requestID,
        decision: contactDecisionToBFF(input.decision),
        idempotency_key: input.idempotencyKey ?? newClientID("contact-respond")
      },
      session
    );
  }

  async cancelContactRequest(session: AuthSession, input: CancelContactRequestInput): Promise<void> {
    await this.#request<unknown>(
      "POST",
      CLIENT_API_ENDPOINTS.cancelContactRequest,
      {
        request_id: input.requestID,
        idempotency_key: input.idempotencyKey ?? newClientID("contact-cancel")
      },
      session
    );
  }

  async deleteContact(session: AuthSession, input: ContactActionInput): Promise<void> {
    await this.#contactAction(session, CLIENT_API_ENDPOINTS.deleteContact, input);
  }

  async blockContact(session: AuthSession, input: ContactActionInput): Promise<void> {
    await this.#contactAction(session, CLIENT_API_ENDPOINTS.blockContact, input);
  }

  async unblockContact(session: AuthSession, input: ContactActionInput): Promise<void> {
    await this.#contactAction(session, CLIENT_API_ENDPOINTS.unblockContact, input);
  }

  async updateContactRemark(session: AuthSession, input: ContactActionInput): Promise<void> {
    await this.#contactAction(session, CLIENT_API_ENDPOINTS.updateContactRemark, input);
  }

  async updateContactGroup(session: AuthSession, input: ContactActionInput): Promise<void> {
    await this.#contactAction(session, CLIENT_API_ENDPOINTS.updateContactGroup, input);
  }

  async #contactAction(session: AuthSession, path: string, input: ContactActionInput): Promise<void> {
    await this.#request<unknown>(
      "POST",
      path,
      {
        contact_user_id: input.contactUserID,
        idempotency_key: input.idempotencyKey ?? newClientID(`contact-${input.contactUserID}`),
        reason: input.reason,
        remark: input.remark,
        group_name: input.groupName
      },
      session
    );
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
  const conversationID = requiredString(item.conversation_id, "conversation_id");
  return {
    tenantID: session.tenantID,
    conversationID,
    type: conversationSummaryTypeFromBFF(item.conversation_type),
    status: archived ? "ARCHIVED" : "ACTIVE",
    title: item.title?.trim() || `Conversation ${conversationID}`,
    lastSeq: numberValue(item.last_visible_seq),
    memberVersion: numberValue(item.member_version),
    unreadCount: numberValue(item.unread_count),
    muted: item.muted === true,
    pinned: item.pinned === true,
    updatedAtMs: numberValue(item.updated_at_unix_ms)
  };
}

function conversationTypeToBFF(value: string): string {
  return value.startsWith("CONVERSATION_TYPE_") ? value : `CONVERSATION_TYPE_${value}`;
}

function conversationTypeFromBFF(value: string | undefined): "DIRECT" | "GROUP" {
  const trimmed = trimEnumPrefix(value, "CONVERSATION_TYPE_");
  return trimmed === "DIRECT" ? "DIRECT" : "GROUP";
}

function conversationSummaryTypeFromBFF(value: string | undefined): ConversationSummary["type"] {
  const trimmed = trimEnumPrefix(value, "CONVERSATION_TYPE_");
  if (trimmed === "DIRECT" || trimmed === "GROUP") {
    return trimmed;
  }
  return "UNKNOWN";
}

function memberChangeFromBFF(response: BFFConversationMemberChangeResponse): ConversationMemberChangeResponse {
  return {
    changeID: requiredString(response.change_id, "change_id"),
    tenantID: requiredString(response.tenant_id, "tenant_id"),
    conversationID: requiredString(response.conversation_id, "conversation_id"),
    targetUserID: requiredString(response.target_user_id, "target_user_id"),
    changeType: trimEnumPrefix(response.change_type, "MEMBER_CHANGE_TYPE_") || "UNSPECIFIED",
    status: trimEnumPrefix(response.status, "MEMBER_CHANGE_STATUS_") || "UNSPECIFIED",
    boundarySeq: numberValue(response.boundary_seq),
    memberVersion: numberValue(response.member_version),
    permissionVersion: numberValue(response.permission_version),
    idempotentReplay: response.idempotent_replay === true
  };
}

function listConversationMembersFromBFF(response: BFFListConversationMembersResponse): ListConversationMembersResponse {
  return {
    tenantID: requiredString(response.tenant_id, "tenant_id"),
    conversationID: requiredString(response.conversation_id, "conversation_id"),
    memberVersion: numberValue(response.member_version),
    permissionVersion: numberValue(response.permission_version),
    members: (response.members ?? []).map(memberFromBFF),
    nextPageToken: response.next_page_token ?? ""
  };
}

function memberFromBFF(item: BFFConversationMember): ConversationMember {
  return {
    userID: requiredString(item.user_id, "user_id"),
    role: trimEnumPrefix(item.role, "MEMBER_ROLE_") || "UNSPECIFIED",
    status: trimEnumPrefix(item.status, "MEMBER_STATUS_") || "UNSPECIFIED",
    joinSeq: numberValue(item.join_seq),
    leaveSeq: numberValue(item.leave_seq),
    memberVersion: numberValue(item.member_version),
    permissionVersion: numberValue(item.permission_version),
    updatedAtMs: numberValue(item.updated_at_unix_ms)
  };
}

function transferOwnerFromBFF(response: BFFTransferConversationOwnerResponse): TransferConversationOwnerResponse {
  return {
    changeID: requiredString(response.change_id, "change_id"),
    tenantID: requiredString(response.tenant_id, "tenant_id"),
    conversationID: requiredString(response.conversation_id, "conversation_id"),
    previousOwnerUserID: requiredString(response.previous_owner_user_id, "previous_owner_user_id"),
    newOwnerUserID: requiredString(response.new_owner_user_id, "new_owner_user_id"),
    status: trimEnumPrefix(response.status, "MEMBER_CHANGE_STATUS_") || "UNSPECIFIED",
    boundarySeq: numberValue(response.boundary_seq),
    memberVersion: numberValue(response.member_version),
    permissionVersion: numberValue(response.permission_version),
    idempotentReplay: response.idempotent_replay === true
  };
}

function conversationProfileFromBFF(response: BFFConversationProfile): ConversationProfile {
  return {
    tenantID: requiredString(response.tenant_id, "tenant_id"),
    conversationID: requiredString(response.conversation_id, "conversation_id"),
    type: conversationTypeFromBFF(response.conversation_type),
    title: response.title ?? "",
    avatarURI: response.avatar_uri ?? "",
    profileVersion: numberValue(response.profile_version),
    memberVersion: numberValue(response.member_version),
    permissionVersion: numberValue(response.permission_version),
    updatedAtMs: numberValue(response.updated_at_unix_ms)
  };
}

function groupAvatarUploadSessionFromBFF(response: BFFGroupAvatarUploadSessionResponse): GroupAvatarUploadSession {
  return {
    assetID: requiredString(response.asset_id, "asset_id"),
    uploadSessionID: requiredString(response.upload_session_id, "upload_session_id"),
    uploadURL: requiredString(response.upload_url, "upload_url"),
    requiredHeaders: response.required_headers ?? {},
    expiresAtMs: numberValue(response.expires_at_unix_ms),
    maxSizeBytes: numberValue(response.max_size_bytes),
    acceptedContentTypes: response.accepted_content_types ?? []
  };
}

function memberRoleToBFF(role: string): string {
  return role.startsWith("MEMBER_ROLE_") ? role : `MEMBER_ROLE_${role}`;
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
    status: trimEnumPrefix(item.status, "CONTACT_EDGE_STATUS_") || "UNSPECIFIED",
    remark: item.remark ?? "",
    groupName: item.group_name ?? "",
    updatedAtMs: numberValue(item.updated_at_unix_ms)
  };
}

function contactRequestFromBFF(item: BFFContactRequestItem): ContactRequestItem {
  return {
    requestID: requiredString(item.request_id, "request_id"),
    senderUserID: requiredString(item.sender_user_id, "sender_user_id"),
    receiverUserID: requiredString(item.receiver_user_id, "receiver_user_id"),
    status: trimEnumPrefix(item.status, "CONTACT_REQUEST_STATUS_") || "UNSPECIFIED",
    message: item.message ?? "",
    createdAtMs: numberValue(item.created_at_unix_ms),
    updatedAtMs: numberValue(item.updated_at_unix_ms),
    decidedAtMs: numberValue(item.decided_at_unix_ms),
    sourceType: trimEnumPrefix(item.source_type, "CONTACT_REQUEST_SOURCE_TYPE_") || "UNSPECIFIED",
    sourceRef: item.source_ref ?? "",
    riskLevel: trimEnumPrefix(item.risk_level, "CONTACT_REQUEST_RISK_LEVEL_") || "UNSPECIFIED",
    reviewRequired: item.review_required === true
  };
}

function contactSourceTypeToBFF(sourceType: string): string {
  return sourceType.startsWith("CONTACT_REQUEST_SOURCE_TYPE_")
    ? sourceType
    : `CONTACT_REQUEST_SOURCE_TYPE_${sourceType}`;
}

function contactDecisionToBFF(decision: ContactDecision): string {
  return decision.startsWith("CONTACT_DECISION_") ? decision : `CONTACT_DECISION_${decision}`;
}

function trimEnumPrefix(value: string | undefined, prefix: string): string {
  if (!value) {
    return "";
  }
  return value.startsWith(prefix) ? value.slice(prefix.length) : value;
}

function newClientID(prefix: string): string {
  const normalizedPrefix = prefix.replace(/[^a-zA-Z0-9_-]+/g, "-").replace(/^-+|-+$/g, "") || "client";
  if (globalThis.crypto?.randomUUID) {
    return `${normalizedPrefix}-${globalThis.crypto.randomUUID()}`;
  }
  return `${normalizedPrefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function decodePayloadJSON(value: string | undefined): unknown {
  if (!value) {
    return {};
  }
  try {
    return JSON.parse(utf8FromBase64(value));
  } catch {
    try {
      return JSON.parse(value);
    } catch {
      return {};
    }
  }
}

function utf8FromBase64(value: string): string {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return new TextDecoder("utf-8", { fatal: false }).decode(bytes);
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

function requiredObject<T>(value: T | undefined, field: string): T {
  if (value === undefined || value === null) {
    throw publicError("SERVER_BUSY", `BFF response missing ${field}`, false);
  }
  return value;
}

function optionalString(value: string | undefined): string | undefined {
  if (!value || value.trim() === "") {
    return undefined;
  }
  return value;
}

function optionalDirectPeerUserID(response: BFFCreateConversationResponse): { directPeerUserID: string } | Record<string, never> {
  const directPeerUserID = optionalString(response.direct_peer_user_id);
  return directPeerUserID ? { directPeerUserID } : {};
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
