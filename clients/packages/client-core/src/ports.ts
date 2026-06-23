import type {
  AckDeliveryRequest,
  AckDeliveryResponse,
  AuthSession,
  ConversationID,
  DeliveryNotifyFrame,
  LoginRequest,
  LoginResponse,
  RegisterRequest,
  RegisterResponse,
  CreateConversationRequest,
  CreateConversationResponse,
  CreateGroupAvatarUploadSessionRequest,
  CompleteGroupAvatarUploadRequest,
  CompleteGroupAvatarUploadResponse,
  GetGroupAvatarDownloadURLRequest,
  GroupAvatarDownloadURL,
  GroupAvatarUploadSession,
  ListConversationMembersRequest,
  ListConversationMembersResponse,
  ConversationMemberChangeResponse,
  ConversationProfile,
  InviteConversationMemberRequest,
  LeaveConversationRequest,
  RemoveConversationMemberRequest,
  TransferConversationOwnerRequest,
  TransferConversationOwnerResponse,
  UpdateConversationProfileRequest,
  UpdateConversationMemberRoleRequest,
  OpenDirectConversationRequest,
  MessageItem,
  PullInboxRequest,
  PullInboxResponse,
  SendMessageRequest,
  SendMessageResponse,
  ServerFrame
} from "@nexusim/protocol";

export interface AuthAPI {
  register(request: RegisterRequest): Promise<RegisterResponse>;
  login(request: LoginRequest): Promise<LoginResponse>;
  refresh(session: AuthSession): Promise<LoginResponse>;
  logout(session: AuthSession): Promise<void>;
}

export interface ConversationAPI {
  createConversation(request: CreateConversationRequest, session: AuthSession): Promise<CreateConversationResponse>;
  openDirectConversation(
    request: OpenDirectConversationRequest,
    session: AuthSession
  ): Promise<CreateConversationResponse>;
  listConversationMembers(
    request: ListConversationMembersRequest,
    session: AuthSession
  ): Promise<ListConversationMembersResponse>;
  inviteConversationMember(
    request: InviteConversationMemberRequest,
    session: AuthSession
  ): Promise<ConversationMemberChangeResponse>;
  leaveConversation(
    request: LeaveConversationRequest,
    session: AuthSession
  ): Promise<ConversationMemberChangeResponse>;
  removeConversationMember(
    request: RemoveConversationMemberRequest,
    session: AuthSession
  ): Promise<ConversationMemberChangeResponse>;
  updateConversationMemberRole(
    request: UpdateConversationMemberRoleRequest,
    session: AuthSession
  ): Promise<ConversationMemberChangeResponse>;
  transferConversationOwner(
    request: TransferConversationOwnerRequest,
    session: AuthSession
  ): Promise<TransferConversationOwnerResponse>;
  getConversationProfile(conversationID: ConversationID, session: AuthSession): Promise<ConversationProfile>;
  updateConversationProfile(
    request: UpdateConversationProfileRequest,
    session: AuthSession
  ): Promise<ConversationProfile>;
  createGroupAvatarUploadSession(
    request: CreateGroupAvatarUploadSessionRequest,
    session: AuthSession
  ): Promise<GroupAvatarUploadSession>;
  completeGroupAvatarUpload(
    request: CompleteGroupAvatarUploadRequest,
    session: AuthSession
  ): Promise<CompleteGroupAvatarUploadResponse>;
  getGroupAvatarDownloadURL(
    request: GetGroupAvatarDownloadURLRequest,
    session: AuthSession
  ): Promise<GroupAvatarDownloadURL>;
}

export interface MessagingAPI {
  sendMessage(request: SendMessageRequest, session: AuthSession): Promise<SendMessageResponse>;
}

export interface DeliveryAPI {
  pullInbox(request: PullInboxRequest, session: AuthSession): Promise<PullInboxResponse>;
  ackDelivery(request: AckDeliveryRequest, session: AuthSession): Promise<AckDeliveryResponse>;
}

export interface LocalMessageStore {
  getLastReceivedSeq(conversationID: ConversationID): Promise<number>;
  upsertMessages(messages: MessageItem[]): Promise<void>;
  markPending(message: MessageItem): Promise<void>;
  markSendAccepted(localID: string, response: SendMessageResponse): Promise<void>;
  markSendFailed(localID: string, reason: string): Promise<void>;
  listMessages(conversationID: ConversationID): Promise<MessageItem[]>;
  listConversationsNeedingSync(): Promise<ConversationID[]>;
  clear(): Promise<void>;
}

export interface PushTransport {
  connect(input: PushConnectInput): Promise<PushConnection>;
}

export interface PushConnectInput {
  url: string;
  session: AuthSession;
  resumeToken?: string;
  onFrame(frame: ServerFrame): void;
  onClose(reason: string): void;
}

export interface PushConnection {
  send(frame: unknown): void;
  close(): void;
}

export interface NotifyScheduler {
  scheduleFromNotify(frame: DeliveryNotifyFrame): void;
  scheduleConversation(conversationID: ConversationID): void;
}
