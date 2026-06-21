import type {
  AuthSession,
  ConversationID,
  MessageItem,
  SendMessageResponse
} from "@nexusim/protocol";
import type { LocalMessageStore } from "./ports";
import type { SecureSessionStore } from "./platform";

type StoredMessages = Map<string, MessageItem>;

export class DevelopmentSessionStore implements SecureSessionStore {
  #session: AuthSession | null = null;

  async loadSession(): Promise<AuthSession | null> {
    return this.#session;
  }

  async saveSession(session: AuthSession): Promise<void> {
    this.#session = { ...session };
  }

  async clearSession(): Promise<void> {
    this.#session = null;
  }
}

export class MemoryMessageStore implements LocalMessageStore {
  #messagesByConversation = new Map<ConversationID, StoredMessages>();
  #pendingByLocalID = new Map<string, MessageItem>();
  #lastReceivedSeq = new Map<ConversationID, number>();

  async getLastReceivedSeq(conversationID: ConversationID): Promise<number> {
    return this.#lastReceivedSeq.get(conversationID) ?? 0;
  }

  async upsertMessages(messages: MessageItem[]): Promise<void> {
    for (const message of messages) {
      this.#upsert(message);
      this.#recordReceivedSeq(message);
    }
  }

  async markPending(message: MessageItem): Promise<void> {
    const localID = localMessageID(message);
    this.#pendingByLocalID.set(localID, { ...message, status: "PENDING" });
    this.#upsert({ ...message, status: "PENDING" });
  }

  async markSendAccepted(localID: string, response: SendMessageResponse): Promise<void> {
    const pending = this.#pendingByLocalID.get(localID);
    if (!pending) {
      return;
    }

    const accepted: MessageItem = {
      ...pending,
      messageID: response.messageID,
      conversationID: response.conversationID,
      conversationSeq: response.conversationSeq,
      status: "SENT"
    };
    this.#pendingByLocalID.delete(localID);
    this.#upsert(accepted);
    this.#recordReceivedSeq(accepted);
  }

  async markSendFailed(localID: string, reason: string): Promise<void> {
    const pending = this.#pendingByLocalID.get(localID);
    if (!pending) {
      return;
    }

    void reason;
    const failed: MessageItem = { ...pending, status: "FAILED" };
    this.#pendingByLocalID.set(localID, failed);
    this.#upsert(failed);
  }

  async listConversationsNeedingSync(): Promise<ConversationID[]> {
    return Array.from(this.#messagesByConversation.keys()).sort();
  }

  async clear(): Promise<void> {
    this.#messagesByConversation.clear();
    this.#pendingByLocalID.clear();
    this.#lastReceivedSeq.clear();
  }

  #upsert(message: MessageItem): void {
    let messages = this.#messagesByConversation.get(message.conversationID);
    if (!messages) {
      messages = new Map<string, MessageItem>();
      this.#messagesByConversation.set(message.conversationID, messages);
    }
    messages.set(messageKey(message), { ...message });
  }

  #recordReceivedSeq(message: MessageItem): void {
    if (message.conversationSeq <= 0) {
      return;
    }
    const current = this.#lastReceivedSeq.get(message.conversationID) ?? 0;
    if (message.conversationSeq > current) {
      this.#lastReceivedSeq.set(message.conversationID, message.conversationSeq);
    }
  }
}

function messageKey(message: MessageItem): string {
  if (message.conversationSeq > 0) {
    return `seq:${message.conversationSeq}`;
  }
  return `local:${localMessageID(message)}`;
}

function localMessageID(message: MessageItem): string {
  return message.clientMessageID ?? message.messageID;
}
