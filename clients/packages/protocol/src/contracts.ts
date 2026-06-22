export const CLIENT_API_ENDPOINTS = {
  register: "/api/auth/register",
  login: "/api/auth/login",
  refresh: "/api/auth/refresh",
  logout: "/api/auth/logout",
  me: "/api/me",
  conversations: "/api/conversations",
  createConversation: "/api/conversations/create",
  directConversation: "/api/conversations/direct",
  conversationMessages: (conversationID: string) =>
    `/api/conversations/${encodeURIComponent(conversationID)}/messages`,
  sendMessage: "/api/messages/send",
  ackDelivery: "/api/delivery/ack",
  contactRequests: "/api/contact-requests",
  sendContactRequest: "/api/contact-requests/send",
  respondContactRequest: "/api/contact-requests/respond",
  cancelContactRequest: "/api/contact-requests/cancel",
  contacts: "/api/contacts",
  contactState: "/api/contacts/state",
  deleteContact: "/api/contacts/delete",
  blockContact: "/api/contacts/block",
  unblockContact: "/api/contacts/unblock",
  updateContactRemark: "/api/contacts/remark",
  updateContactGroup: "/api/contacts/group",
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
