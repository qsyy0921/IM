import type { ConversationID, MessageItem, SendMessageResponse } from "@nexusim/protocol";
import type { LocalMessageStore } from "@nexusim/client-core";

const DB_NAME = "nexusim-web-client";
const DB_VERSION = 1;
const MESSAGE_STORE = "messages";
const CURSOR_STORE = "cursors";

interface StoredMessage extends MessageItem {
  localKey: string;
}

interface StoredCursor {
  conversationID: ConversationID;
  lastReceivedSeq: number;
  updatedAtMs: number;
}

export class IndexedDBMessageStore implements LocalMessageStore {
  #dbPromise: Promise<IDBDatabase> | null = null;

  async getLastReceivedSeq(conversationID: ConversationID): Promise<number> {
    const db = await this.#db();
    const cursor = await idbGet<StoredCursor>(db, CURSOR_STORE, conversationID);
    return cursor?.lastReceivedSeq ?? 0;
  }

  async upsertMessages(messages: MessageItem[]): Promise<void> {
    if (messages.length === 0) {
      return;
    }
    const db = await this.#db();
    await txDone(db, [MESSAGE_STORE, CURSOR_STORE], "readwrite", transaction => {
      const messagesStore = transaction.objectStore(MESSAGE_STORE);
      const cursorStore = transaction.objectStore(CURSOR_STORE);
      const maxSeqByConversation = new Map<ConversationID, number>();
      for (const message of messages) {
        messagesStore.put(storedMessage(message));
        const current = maxSeqByConversation.get(message.conversationID) ?? 0;
        if (message.conversationSeq > current) {
          maxSeqByConversation.set(message.conversationID, message.conversationSeq);
        }
      }
      for (const [conversationID, seq] of maxSeqByConversation.entries()) {
        cursorStore.put({
          conversationID,
          lastReceivedSeq: seq,
          updatedAtMs: Date.now()
        } satisfies StoredCursor);
      }
    });
  }

  async markPending(message: MessageItem): Promise<void> {
    const db = await this.#db();
    await txDone(db, [MESSAGE_STORE], "readwrite", transaction => {
      transaction.objectStore(MESSAGE_STORE).put(storedMessage(message));
    });
  }

  async markSendAccepted(localID: string, response: SendMessageResponse): Promise<void> {
    const db = await this.#db();
    await txDone(db, [MESSAGE_STORE], "readwrite", transaction => {
      const store = transaction.objectStore(MESSAGE_STORE);
      const index = store.index("clientMessageID");
      const request = index.get(localID);
      request.onsuccess = () => {
        const existing = request.result as StoredMessage | undefined;
        if (!existing) {
          return;
        }
        store.put({
          ...existing,
          messageID: response.messageID,
          conversationSeq: response.conversationSeq,
          status: "SENT"
        } satisfies StoredMessage);
      };
    });
  }

  async markSendFailed(localID: string, _reason: string): Promise<void> {
    const db = await this.#db();
    await txDone(db, [MESSAGE_STORE], "readwrite", transaction => {
      const store = transaction.objectStore(MESSAGE_STORE);
      const index = store.index("clientMessageID");
      const request = index.get(localID);
      request.onsuccess = () => {
        const existing = request.result as StoredMessage | undefined;
        if (!existing) {
          return;
        }
        store.put({
          ...existing,
          status: "FAILED"
        } satisfies StoredMessage);
      };
    });
  }

  async listConversationsNeedingSync(): Promise<ConversationID[]> {
    const db = await this.#db();
    const cursors = await idbGetAll<StoredCursor>(db, CURSOR_STORE);
    return cursors.map(cursor => cursor.conversationID);
  }

  async listMessages(conversationID: ConversationID): Promise<MessageItem[]> {
    const db = await this.#db();
    const messages = await idbGetAll<StoredMessage>(db, MESSAGE_STORE);
    return messages
      .filter(message => message.conversationID === conversationID)
      .sort((left, right) => left.conversationSeq - right.conversationSeq)
      .map(stripLocalKey);
  }

  async #db(): Promise<IDBDatabase> {
    if (!this.#dbPromise) {
      this.#dbPromise = openDB();
    }
    return this.#dbPromise;
  }
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(MESSAGE_STORE)) {
        const store = db.createObjectStore(MESSAGE_STORE, { keyPath: "localKey" });
        store.createIndex("clientMessageID", "clientMessageID", { unique: false });
        store.createIndex("conversationID", "conversationID", { unique: false });
      }
      if (!db.objectStoreNames.contains(CURSOR_STORE)) {
        db.createObjectStore(CURSOR_STORE, { keyPath: "conversationID" });
      }
    };
    request.onerror = () => reject(request.error ?? new Error("open indexeddb failed"));
    request.onsuccess = () => resolve(request.result);
  });
}

function txDone(
  db: IDBDatabase,
  storeNames: string[],
  mode: IDBTransactionMode,
  operation: (transaction: IDBTransaction) => void
): Promise<void> {
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(storeNames, mode);
    transaction.oncomplete = () => resolve();
    transaction.onerror = () => reject(transaction.error ?? new Error("indexeddb transaction failed"));
    transaction.onabort = () => reject(transaction.error ?? new Error("indexeddb transaction aborted"));
    operation(transaction);
  });
}

function idbGet<T>(db: IDBDatabase, storeName: string, key: IDBValidKey): Promise<T | undefined> {
  return new Promise((resolve, reject) => {
    const request = db.transaction(storeName, "readonly").objectStore(storeName).get(key);
    request.onerror = () => reject(request.error ?? new Error("indexeddb get failed"));
    request.onsuccess = () => resolve(request.result as T | undefined);
  });
}

function idbGetAll<T>(db: IDBDatabase, storeName: string): Promise<T[]> {
  return new Promise((resolve, reject) => {
    const request = db.transaction(storeName, "readonly").objectStore(storeName).getAll();
    request.onerror = () => reject(request.error ?? new Error("indexeddb getAll failed"));
    request.onsuccess = () => resolve((request.result as T[] | undefined) ?? []);
  });
}

function storedMessage(message: MessageItem): StoredMessage {
  return {
    ...message,
    localKey: message.messageID
      ? `${message.conversationID}:seq:${message.conversationSeq}`
      : `${message.conversationID}:pending:${message.clientMessageID ?? Date.now()}`
  };
}

function stripLocalKey(message: StoredMessage): MessageItem {
  const { localKey: _localKey, ...rest } = message;
  return rest;
}
