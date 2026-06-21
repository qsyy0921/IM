import type { AckDeliveryRequest, AuthSession, ConversationID } from "@nexusim/protocol";
import type { DeliveryAPI } from "./ports";

export interface AckQueueOptions {
  deliveryAPI: DeliveryAPI;
  requestIDFactory(): string;
}

export class AckQueue {
  #pending = new Map<ConversationID, number>();

  constructor(private readonly options: AckQueueOptions) {}

  recordReceived(conversationID: ConversationID, seq: number): void {
    const current = this.#pending.get(conversationID) ?? 0;
    if (seq > current) {
      this.#pending.set(conversationID, seq);
    }
  }

  pendingCount(): number {
    return this.#pending.size;
  }

  async flush(session: AuthSession): Promise<void> {
    const entries = Array.from(this.#pending.entries());
    this.#pending.clear();

    for (const [conversationID, lastReceivedSeq] of entries) {
      const request: AckDeliveryRequest = {
        tenantID: session.tenantID,
        userID: session.userID,
        deviceID: session.deviceID,
        conversationID,
        lastReceivedSeq,
        requestID: this.options.requestIDFactory()
      };
      await this.options.deliveryAPI.ackDelivery(request, session);
    }
  }
}
