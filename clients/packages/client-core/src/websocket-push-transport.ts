import type { PushConnectInput, PushConnection, PushTransport } from "./ports";
import type { ClientHelloFrame, ServerFrame } from "@nexusim/protocol";

export class WebSocketPushTransport implements PushTransport {
  async connect(input: PushConnectInput): Promise<PushConnection> {
    const url = pushURL(input);
    const socket = new WebSocket(url);

    socket.addEventListener("message", event => {
      try {
        input.onFrame(JSON.parse(String(event.data)) as ServerFrame);
      } catch {
        input.onFrame({
          op: "error",
          code: "SERVER_BUSY",
          message: "invalid websocket frame",
          retryable: true
        });
      }
    });
    socket.addEventListener("close", event => {
      input.onClose(event.reason || `closed:${event.code}`);
    });
    socket.addEventListener("error", () => {
      input.onClose("websocket error");
    });

    await waitForOpen(socket);
    socket.send(JSON.stringify(clientHello(input)));

    return {
      send(frame: unknown): void {
        if (socket.readyState === WebSocket.OPEN) {
          socket.send(JSON.stringify(frame));
        }
      },
      close(): void {
        socket.close(1000, "client close");
      }
    };
  }
}

function pushURL(input: PushConnectInput): string {
  const url = new URL(input.url);
  url.searchParams.set("token", input.session.pushToken ?? input.session.accessToken);
  url.searchParams.set("tenant_id", input.session.tenantID);
  url.searchParams.set("user_id", input.session.userID);
  url.searchParams.set("device_id", input.session.deviceID);
  return url.toString();
}

function clientHello(input: PushConnectInput): ClientHelloFrame {
  const frame: ClientHelloFrame = {
    op: "client.hello",
    request_id: requestID(),
    tenant_id: input.session.tenantID,
    user_id: input.session.userID,
    device_id: input.session.deviceID,
    session_id: input.session.sessionID
  };
  if (input.resumeToken) {
    frame.resume_token = input.resumeToken;
  }
  return frame;
}

function waitForOpen(socket: WebSocket): Promise<void> {
  return new Promise((resolve, reject) => {
    const timeout = globalThis.setTimeout(() => {
      cleanup();
      reject(new Error("websocket open timeout"));
    }, 5000);
    const cleanup = () => {
      globalThis.clearTimeout(timeout);
      socket.removeEventListener("open", onOpen);
      socket.removeEventListener("error", onError);
    };
    const onOpen = () => {
      cleanup();
      resolve();
    };
    const onError = () => {
      cleanup();
      reject(new Error("websocket open failed"));
    };
    socket.addEventListener("open", onOpen);
    socket.addEventListener("error", onError);
  });
}

function requestID(): string {
  if (globalThis.crypto?.randomUUID) {
    return `ws-${globalThis.crypto.randomUUID()}`;
  }
  return `ws-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}
