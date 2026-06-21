import type { StringKeyValueStorage } from "./key-value-message-store";

export interface NativeStringKeyValueBridge {
  getItem(key: string): string | null | Promise<string | null>;
  setItem(key: string, value: string): void | Promise<void>;
  removeItem(key: string): void | Promise<void>;
}

export class NativeBridgeStringKeyValueStorage implements StringKeyValueStorage {
  readonly #bridge: NativeStringKeyValueBridge;

  constructor(bridge: NativeStringKeyValueBridge) {
    this.#bridge = bridge;
  }

  getItem(key: string): string | null | Promise<string | null> {
    return this.#bridge.getItem(key);
  }

  setItem(key: string, value: string): void | Promise<void> {
    return this.#bridge.setItem(key, value);
  }

  removeItem(key: string): void | Promise<void> {
    return this.#bridge.removeItem(key);
  }
}
