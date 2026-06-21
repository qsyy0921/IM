import type { AuthSession, LoginRequest } from "@nexusim/protocol";
import type { ClientRuntime } from "./runtime";

export interface ClientShellSessionState {
  authenticated: boolean;
  tenantID?: string;
  userID?: string;
  deviceID?: string;
  sessionID?: string;
}

export interface ClientShellActions {
  currentSession(): ClientShellSessionState;
  login(request: LoginRequest): Promise<ClientShellSessionState>;
  refresh(): Promise<ClientShellSessionState>;
  restoreSession(): Promise<ClientShellSessionState>;
  logout(): Promise<ClientShellSessionState>;
}

export function createClientShellActions(
  runtime: Pick<ClientRuntime, "auth" | "login" | "refresh" | "restoreSession" | "logout">
): ClientShellActions {
  return {
    currentSession(): ClientShellSessionState {
      return sessionState(runtime.auth.current());
    },
    async login(request: LoginRequest): Promise<ClientShellSessionState> {
      return sessionState(await runtime.login(request));
    },
    async refresh(): Promise<ClientShellSessionState> {
      return sessionState(await runtime.refresh());
    },
    async restoreSession(): Promise<ClientShellSessionState> {
      return sessionState(await runtime.restoreSession());
    },
    async logout(): Promise<ClientShellSessionState> {
      await runtime.logout();
      return sessionState(runtime.auth.current());
    }
  };
}

function sessionState(session: AuthSession | null): ClientShellSessionState {
  if (!session) {
    return { authenticated: false };
  }
  return {
    authenticated: true,
    tenantID: session.tenantID,
    userID: session.userID,
    deviceID: session.deviceID,
    sessionID: session.sessionID
  };
}
