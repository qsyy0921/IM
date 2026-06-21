import type { AuthSession, DeliveryNotifyFrame, ServerFrame } from "@nexusim/protocol";
import type { NotifyScheduler, PushConnection, PushTransport } from "./ports";

export interface PushConnectionManagerOptions {
  url: string;
  transport: PushTransport;
  scheduler: NotifyScheduler;
}

export class PushConnectionManager {
  #connection: PushConnection | null = null;
  #resumeToken: string | undefined;

  constructor(private readonly options: PushConnectionManagerOptions) {}

  async connect(session: AuthSession): Promise<void> {
    this.disconnect();
    const input = {
      url: this.options.url,
      session,
      onFrame: (frame: ServerFrame) => this.#handleFrame(frame),
      onClose: () => {
        this.#connection = null;
      }
    };
    this.#connection = await this.options.transport.connect(
      this.#resumeToken ? { ...input, resumeToken: this.#resumeToken } : input
    );
  }

  disconnect(): void {
    this.#connection?.close();
    this.#connection = null;
  }

  #handleFrame(frame: ServerFrame): void {
    switch (frame.op) {
      case "server.hello":
        this.#resumeToken = frame.resume_token;
        return;
      case "delivery.notify":
        this.options.scheduler.scheduleFromNotify(frame as DeliveryNotifyFrame);
        return;
      case "server.resume_hint":
        for (const cursor of frame.conversations ?? []) {
          this.options.scheduler.scheduleConversation(cursor.conversation_id);
        }
        return;
      default:
        return;
    }
  }
}
