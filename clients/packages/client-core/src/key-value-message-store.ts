import type { ConversationID, MessageItem, SendMessageResponse } from "@nexusim/protocol";
import type { LocalMessageStore } from "./ports";

export interface StringKeyValueStorage {
  getItem(key: string): string | null | Promise<string | null>;
  setItem(key: string, value: string): void | Promise<void>;
  removeItem(key: string): void | Promise<void>;
}

export interface KeyValueMessageStoreOptions {
  namespace?: string;
  keyPrefix?: string;
}

interface StoredSnapshot {
  version: 2;
  messagesByKey: Record<string, MessageItem>;
  pendingKeyByLocalID: Record<string, string>;
  lastReceivedSeqByConversation: Record<string, number>;
}

export class KeyValueMessageStore implements LocalMessageStore {
  readonly #storage: StringKeyValueStorage;
  readonly #storageKey: string;

  constructor(storage: StringKeyValueStorage, options: KeyValueMessageStoreOptions = {}) {
    this.#storage = storage;
    const keyPrefix = options.keyPrefix ?? "nexusim:client-message-store:v2";
    const namespace = options.namespace ?? "default";
    this.#storageKey = `${keyPrefix}:${namespace}`;
  }

  async getLastReceivedSeq(conversationID: ConversationID): Promise<number> {
    const snapshot = await this.#load();
    return snapshot.lastReceivedSeqByConversation[conversationID] ?? 0;
  }

  async upsertMessages(messages: MessageItem[]): Promise<void> {
    if (messages.length === 0) {
      return;
    }
    const snapshot = await this.#load();
    for (const message of messages) {
      this.#upsert(snapshot, message);
      this.#recordReceivedSeq(snapshot, message);
    }
    await this.#save(snapshot);
  }

  async markPending(message: MessageItem): Promise<void> {
    const snapshot = await this.#load();
    const pending = { ...message, status: "PENDING" as const };
    const key = messageKey(pending);
    snapshot.messagesByKey[key] = pending;
    snapshot.pendingKeyByLocalID[localMessageID(pending)] = key;
    await this.#save(snapshot);
  }

  async markSendAccepted(localID: string, response: SendMessageResponse): Promise<void> {
    const snapshot = await this.#load();
    const pendingKey = snapshot.pendingKeyByLocalID[localID];
    const pending = pendingKey
      ? snapshot.messagesByKey[pendingKey]
      : Object.values(snapshot.messagesByKey).find(message => localMessageID(message) === localID);
    if (!pending) {
      return;
    }

    if (pendingKey) {
      delete snapshot.messagesByKey[pendingKey];
    }
    delete snapshot.pendingKeyByLocalID[localID];

    const accepted: MessageItem = {
      ...pending,
      messageID: response.messageID,
      conversationID: response.conversationID,
      conversationSeq: response.conversationSeq,
      status: "SENT"
    };
    this.#upsert(snapshot, accepted);
    this.#recordReceivedSeq(snapshot, accepted);
    await this.#save(snapshot);
  }

  async markSendFailed(localID: string, _reason: string): Promise<void> {
    const snapshot = await this.#load();
    const pendingKey = snapshot.pendingKeyByLocalID[localID];
    const pending = pendingKey ? snapshot.messagesByKey[pendingKey] : undefined;
    if (!pending || !pendingKey) {
      return;
    }
    snapshot.messagesByKey[pendingKey] = { ...pending, status: "FAILED" };
    await this.#save(snapshot);
  }

  async listConversationsNeedingSync(): Promise<ConversationID[]> {
    const snapshot = await this.#load();
    return Object.keys(snapshot.lastReceivedSeqByConversation).sort();
  }

  async listMessages(conversationID: ConversationID): Promise<MessageItem[]> {
    const snapshot = await this.#load();
    return Object.values(snapshot.messagesByKey)
      .filter(message => message.conversationID === conversationID)
      .sort((left, right) => left.conversationSeq - right.conversationSeq)
      .map(message => ({ ...message }));
  }

  async clear(): Promise<void> {
    await this.#storage.removeItem(this.#storageKey);
  }

  #upsert(snapshot: StoredSnapshot, message: MessageItem): void {
    const localID = localMessageID(message);
    const pendingKey = snapshot.pendingKeyByLocalID[localID];
    if (message.conversationSeq > 0 && pendingKey) {
      delete snapshot.messagesByKey[pendingKey];
      delete snapshot.pendingKeyByLocalID[localID];
    }
    snapshot.messagesByKey[messageKey(message)] = { ...message };
  }

  #recordReceivedSeq(snapshot: StoredSnapshot, message: MessageItem): void {
    if (message.conversationSeq <= 0) {
      return;
    }
    const current = snapshot.lastReceivedSeqByConversation[message.conversationID] ?? 0;
    if (message.conversationSeq > current) {
      snapshot.lastReceivedSeqByConversation[message.conversationID] = message.conversationSeq;
    }
  }

  async #load(): Promise<StoredSnapshot> {
    const raw = await this.#storage.getItem(this.#storageKey);
    if (!raw) {
      return emptySnapshot();
    }
    const parsed = JSON.parse(raw) as Partial<StoredSnapshot>;
    if (parsed.version !== 2) {
      return emptySnapshot();
    }
    return {
      version: 2,
      messagesByKey: parsed.messagesByKey ?? {},
      pendingKeyByLocalID: parsed.pendingKeyByLocalID ?? {},
      lastReceivedSeqByConversation: parsed.lastReceivedSeqByConversation ?? {}
    };
  }

  async #save(snapshot: StoredSnapshot): Promise<void> {
    await this.#storage.setItem(this.#storageKey, JSON.stringify(snapshot));
  }
}

export class GlobalLocalStorageKeyValueStorage implements StringKeyValueStorage {
  getItem(key: string): string | null {
    return localStorageRef().getItem(key);
  }

  setItem(key: string, value: string): void {
    localStorageRef().setItem(key, value);
  }

  removeItem(key: string): void {
    localStorageRef().removeItem(key);
  }
}

export class MemoryStringKeyValueStorage implements StringKeyValueStorage {
  readonly #items = new Map<string, string>();

  getItem(key: string): string | null {
    return this.#items.get(key) ?? null;
  }

  setItem(key: string, value: string): void {
    this.#items.set(key, value);
  }

  removeItem(key: string): void {
    this.#items.delete(key);
  }
}

function emptySnapshot(): StoredSnapshot {
  return {
    version: 2,
    messagesByKey: {},
    pendingKeyByLocalID: {},
    lastReceivedSeqByConversation: {}
  };
}

function messageKey(message: MessageItem): string {
  if (message.conversationSeq > 0) {
    return `${message.conversationID}:seq:${message.conversationSeq}`;
  }
  return `${message.conversationID}:pending:${localMessageID(message)}`;
}

function localMessageID(message: MessageItem): string {
  return message.clientMessageID ?? message.messageID;
}

function localStorageRef(): Storage {
  if (!globalThis.localStorage) {
    throw new Error("localStorage is not available for this runtime");
  }
  return globalThis.localStorage;
}
