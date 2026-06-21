import type {
  AckDeliveryRequest,
  AckDeliveryResponse,
  AuthSession,
  ConversationID,
  DeliveryNotifyFrame,
  LoginRequest,
  LoginResponse,
  MessageItem,
  PullInboxRequest,
  PullInboxResponse,
  SendMessageRequest,
  SendMessageResponse,
  ServerFrame
} from "@nexusim/protocol";

export interface AuthAPI {
  login(request: LoginRequest): Promise<LoginResponse>;
  refresh(session: AuthSession): Promise<LoginResponse>;
  logout(session: AuthSession): Promise<void>;
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
  listConversationsNeedingSync(): Promise<ConversationID[]>;
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
