import type { AuthSession, ConversationID, DeliveryNotifyFrame } from "@nexusim/protocol";
import type { DeliveryAPI, LocalMessageStore, NotifyScheduler } from "./ports";

export interface InboxSyncEngineOptions {
  deliveryAPI: DeliveryAPI;
  store: LocalMessageStore;
  pageSize: number;
}

export class InboxSyncEngine implements NotifyScheduler {
  #scheduled = new Set<ConversationID>();

  constructor(private readonly options: InboxSyncEngineOptions) {}

  scheduleFromNotify(frame: DeliveryNotifyFrame): void {
    this.scheduleConversation(frame.conversation_id);
  }

  scheduleConversation(conversationID: ConversationID): void {
    this.#scheduled.add(conversationID);
  }

  scheduledCount(): number {
    return this.#scheduled.size;
  }

  async flush(session: AuthSession): Promise<void> {
    const conversations = Array.from(this.#scheduled);
    this.#scheduled.clear();

    for (const conversationID of conversations) {
      const afterSeq = await this.options.store.getLastReceivedSeq(conversationID);
      const response = await this.options.deliveryAPI.pullInbox(
        {
          tenantID: session.tenantID,
          userID: session.userID,
          deviceID: session.deviceID,
          conversationID,
          afterSeq,
          limit: this.options.pageSize
        },
        session
      );
      await this.options.store.upsertMessages(response.items);
    }
  }
}
