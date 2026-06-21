export const CLIENT_API_ENDPOINTS = {
  login: "/api/auth/login",
  refresh: "/api/auth/refresh",
  logout: "/api/auth/logout",
  me: "/api/me",
  conversations: "/api/conversations",
  conversationMessages: (conversationID: string) =>
    `/api/conversations/${encodeURIComponent(conversationID)}/messages`,
  sendMessage: "/api/messages/send",
  ackDelivery: "/api/delivery/ack",
  contacts: "/api/contacts",
  receipts: "/api/receipts"
} as const;

export const PUSH_WS_OPS = {
  clientHello: "client.hello",
  serverHello: "server.hello",
  clientPing: "client.ping",
  serverPong: "server.pong",
  deliveryNotify: "delivery.notify",
  deliveryAck: "delivery.ack",
  deliveryAckOK: "delivery.ack.ok",
  serverResumeHint: "server.resume_hint",
  error: "error"
} as const;
