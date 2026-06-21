import { validateRuntimeConfig } from "@nexusim/client-core";
import type { ConversationSummary, MessageItem } from "@nexusim/protocol";
import { loadRuntimeConfig } from "./runtime-config";

const runtimeConfig = validateRuntimeConfig(loadRuntimeConfig());

const conversations: ConversationSummary[] = [
  {
    tenantID: "tenant-local",
    conversationID: "conv-demo",
    type: "GROUP",
    status: "ACTIVE",
    title: "NexusIM 局域网演示群",
    lastSeq: 129,
    unreadCount: 0,
    muted: false,
    pinned: true,
    updatedAtMs: Date.now()
  }
];

const messages: MessageItem[] = [
  {
    tenantID: "tenant-local",
    conversationID: "conv-demo",
    messageID: "msg-1",
    senderUserID: "user-a",
    conversationSeq: 128,
    contentType: "TEXT",
    text: "Web client shell 已接入 NexusIM client-platform 架构。",
    status: "DELIVERED",
    createdAtMs: Date.now() - 120000
  },
  {
    tenantID: "tenant-local",
    conversationID: "conv-demo",
    messageID: "msg-2",
    senderUserID: "user-b",
    conversationSeq: 129,
    contentType: "TEXT",
    text: "下一步接 api-gateway BFF、PullInbox 和 push-gateway WebSocket。",
    status: "DELIVERED",
    createdAtMs: Date.now() - 60000
  }
];

export function App() {
  const activeConversation = conversations[0]!;

  return (
    <main className="shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">N</span>
          <div>
            <h1>NexusIM</h1>
            <p>Client Platform MVP</p>
          </div>
        </div>

        <section className="panel">
          <h2>运行入口</h2>
          <dl className="config-list">
            <div>
              <dt>API</dt>
              <dd>{runtimeConfig.apiBaseURL}</dd>
            </div>
            <div>
              <dt>WebSocket</dt>
              <dd>{runtimeConfig.pushWebSocketURL}</dd>
            </div>
            <div>
              <dt>Device</dt>
              <dd>{runtimeConfig.deviceID}</dd>
            </div>
          </dl>
        </section>

        <section className="conversation-list">
          <h2>会话</h2>
          {conversations.map(conversation => (
            <button className="conversation-item active" key={conversation.conversationID}>
              <span>{conversation.title}</span>
              <small>seq {conversation.lastSeq}</small>
            </button>
          ))}
        </section>
      </aside>

      <section className="chat">
        <header className="chat-header">
          <div>
            <h2>{activeConversation.title}</h2>
            <p>PullInbox 是事实源，WebSocket 只做在线唤醒。</p>
          </div>
          <span className="status-pill">LAN ready</span>
        </header>

        <div className="messages">
          {messages.map(message => (
            <article className="message" key={message.messageID}>
              <header>
                <strong>{message.senderUserID}</strong>
                <span>#{message.conversationSeq}</span>
              </header>
              <p>{message.text}</p>
              <footer>{message.status}</footer>
            </article>
          ))}
        </div>

        <form className="composer">
          <input placeholder="第一阶段先接真实 SendMessage，再启用输入框" disabled />
          <button type="button" disabled>
            发送
          </button>
        </form>
      </section>
    </main>
  );
}
