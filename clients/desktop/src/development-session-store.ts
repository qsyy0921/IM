import type { AuthSession } from "@nexusim/protocol";
import type { SecureSessionStore } from "@nexusim/client-core";

export class DesktopDevelopmentSessionStore implements SecureSessionStore {
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
