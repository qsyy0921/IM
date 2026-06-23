import type { AuthSession, MessageItem, SendMessageRequest } from "@nexusim/protocol";
import type { LocalMessageStore, MessagingAPI } from "./ports";

export interface SendQueueOptions {
  messagingAPI: MessagingAPI;
  store: LocalMessageStore;
  idempotencyKeyFactory(): string;
  clientMessageIDFactory(): string;
  nowMs(): number;
}

export class MessageSendQueue {
  constructor(private readonly options: SendQueueOptions) {}

  async sendText(input: {
    session: AuthSession;
    conversationID: string;
    text: string;
    onPendingStored?(message: MessageItem): void | Promise<void>;
  }): Promise<MessageItem> {
    const clientMessageID = this.options.clientMessageIDFactory();
    const pending: MessageItem = {
      tenantID: input.session.tenantID,
      conversationID: input.conversationID,
      messageID: "",
      senderUserID: input.session.userID,
      conversationSeq: 0,
      contentType: "TEXT",
      text: input.text,
      clientMessageID,
      status: "PENDING",
      createdAtMs: this.options.nowMs()
    };

    await this.options.store.markPending(pending);
    await input.onPendingStored?.({ ...pending });

    const request: SendMessageRequest = {
      tenantID: input.session.tenantID,
      conversationID: input.conversationID,
      senderUserID: input.session.userID,
      clientMessageID,
      idempotencyKey: this.options.idempotencyKeyFactory(),
      text: input.text
    };

    try {
      const response = await this.options.messagingAPI.sendMessage(request, input.session);
      await this.options.store.markSendAccepted(clientMessageID, response);
      return {
        ...pending,
        messageID: response.messageID,
        conversationSeq: response.conversationSeq,
        status: "SENT"
      };
    } catch (error) {
      await this.options.store.markSendFailed(clientMessageID, String(error));
      throw error;
    }
  }
}
