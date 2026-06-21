import type { AuthAPI } from "./ports";
import type { AuthSession, LoginRequest } from "@nexusim/protocol";

export class AuthSessionManager {
  #session: AuthSession | null = null;

  constructor(private readonly api: AuthAPI) {}

  current(): AuthSession | null {
    return this.#session;
  }

  requireSession(): AuthSession {
    if (!this.#session) {
      throw new Error("not authenticated");
    }
    return this.#session;
  }

  async login(request: LoginRequest): Promise<AuthSession> {
    const response = await this.api.login(request);
    this.#session = response.session;
    return response.session;
  }

  async refresh(): Promise<AuthSession> {
    const session = this.requireSession();
    const response = await this.api.refresh(session);
    this.#session = response.session;
    return response.session;
  }

  async logout(): Promise<void> {
    const session = this.#session;
    this.#session = null;
    if (session) {
      await this.api.logout(session);
    }
  }
}
