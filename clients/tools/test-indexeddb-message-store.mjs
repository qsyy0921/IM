import { readFileSync } from "node:fs";
import { Buffer } from "node:buffer";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import ts from "typescript";

const root = fileURLToPath(new URL("..", import.meta.url));

async function main() {
installFakeIndexedDB();
const sourcePath = join(root, "web/src/adapters/indexeddb-message-store.ts");
const source = readFileSync(sourcePath, "utf8");
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022
  }
}).outputText;

const moduleURL = `data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`;
const { IndexedDBMessageStore } = await import(moduleURL);

const store = new IndexedDBMessageStore();

assertEqual(await store.getLastReceivedSeq("conv-1"), 0, "new conversation cursor starts at 0");

await store.upsertMessages([
  message("conv-1", "msg-2", 2),
  message("conv-1", "msg-1", 1),
  message("conv-2", "msg-5", 5)
]);

assertEqual(await store.getLastReceivedSeq("conv-1"), 2, "upsert updates max cursor");
assertDeepEqual(
  (await store.listMessages("conv-1")).map(item => item.messageID),
  ["msg-1", "msg-2"],
  "messages are sorted by conversation seq"
);
assertDeepEqual(
  await store.listConversationsNeedingSync(),
  ["conv-1", "conv-2"],
  "cursor store lists conversations needing sync"
);

await store.markPending({
  tenantID: "tenant-1",
  conversationID: "conv-1",
  messageID: "",
  senderUserID: "user-1",
  conversationSeq: 0,
  contentType: "TEXT",
  text: "pending",
  clientMessageID: "local-1",
  status: "PENDING",
  createdAtMs: 3
});

assertDeepEqual(
  (await store.listMessages("conv-1")).map(item => `${item.status}:${item.clientMessageID ?? item.messageID}`),
  ["PENDING:local-1", "SENT:msg-1", "SENT:msg-2"],
  "pending message is stored under local key"
);

await store.markSendAccepted("local-1", {
  messageID: "msg-3",
  conversationID: "conv-1",
  conversationSeq: 3
});
await store.upsertMessages([message("conv-1", "msg-3", 3)]);

assertEqual(await store.getLastReceivedSeq("conv-1"), 3, "accepted send advances cursor");
assertDeepEqual(
  (await store.listMessages("conv-1")).map(item => item.messageID),
  ["msg-1", "msg-2", "msg-3"],
  "accepted send migrates pending key to stable seq key without replay duplicate"
);

await store.markPending({
  tenantID: "tenant-1",
  conversationID: "conv-1",
  messageID: "",
  senderUserID: "user-1",
  conversationSeq: 0,
  contentType: "TEXT",
  text: "will fail",
  clientMessageID: "local-2",
  status: "PENDING",
  createdAtMs: 4
});
await store.markSendFailed("local-2", "network failed");

assertEqual(
  (await store.listMessages("conv-1")).find(item => item.clientMessageID === "local-2")?.status,
  "FAILED",
  "failed send keeps local record with FAILED status"
);

console.log("indexeddb message store persistence ok");
}

function message(conversationID, messageID, seq) {
  return {
    tenantID: "tenant-1",
    conversationID,
    messageID,
    senderUserID: "user-1",
    conversationSeq: seq,
    contentType: "TEXT",
    text: `message ${seq}`,
    status: "SENT",
    createdAtMs: seq
  };
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) {
    throw new Error(`${message}: expected ${expected}, got ${actual}`);
  }
}

function assertDeepEqual(actual, expected, message) {
  const actualJSON = JSON.stringify(actual);
  const expectedJSON = JSON.stringify(expected);
  if (actualJSON !== expectedJSON) {
    throw new Error(`${message}: expected ${expectedJSON}, got ${actualJSON}`);
  }
}

function installFakeIndexedDB() {
  globalThis.indexedDB = new FakeIndexedDB();
}

class FakeIndexedDB {
  #databases = new Map();

  open(name) {
    const request = new FakeRequest();
    queueMicrotask(() => {
      let database = this.#databases.get(name);
      const needsUpgrade = !database;
      if (!database) {
        database = new FakeDatabase();
        this.#databases.set(name, database);
      }
      request.result = database;
      if (needsUpgrade) {
        request.onupgradeneeded?.();
      }
      request.onsuccess?.();
    });
    return request;
  }
}

class FakeDatabase {
  #stores = new Map();
  objectStoreNames = {
    contains: name => this.#stores.has(name)
  };

  createObjectStore(name, options) {
    const store = new FakeStore(name, options.keyPath);
    this.#stores.set(name, store);
    return new FakeObjectStore(store, null);
  }

  transaction(storeNames) {
    const names = Array.isArray(storeNames) ? storeNames : [storeNames];
    for (const name of names) {
      if (!this.#stores.has(name)) {
        throw new Error(`missing store ${name}`);
      }
    }
    return new FakeTransaction(this.#stores);
  }
}

class FakeTransaction {
  #stores;
  #completeQueued = false;
  oncomplete = null;
  onerror = null;
  onabort = null;
  error = null;

  constructor(stores) {
    this.#stores = stores;
  }

  objectStore(name) {
    const store = this.#stores.get(name);
    if (!store) {
      throw new Error(`missing store ${name}`);
    }
    return new FakeObjectStore(store, this);
  }

  queueComplete() {
    if (this.#completeQueued) {
      return;
    }
    this.#completeQueued = true;
    setTimeout(() => {
      this.oncomplete?.();
    }, 0);
  }
}

class FakeStore {
  constructor(name, keyPath) {
    this.name = name;
    this.keyPath = keyPath;
    this.records = new Map();
    this.indexes = new Map();
  }
}

class FakeObjectStore {
  constructor(store, transaction) {
    this.store = store;
    this.transaction = transaction;
  }

  createIndex(name, keyPath) {
    this.store.indexes.set(name, keyPath);
  }

  put(value) {
    const key = value[this.store.keyPath];
    this.store.records.set(key, structuredClone(value));
    this.transaction?.queueComplete();
  }

  delete(key) {
    this.store.records.delete(key);
    this.transaction?.queueComplete();
  }

  get(key) {
    const request = new FakeRequest();
    queueMicrotask(() => {
      request.result = cloneOrUndefined(this.store.records.get(key));
      request.onsuccess?.();
      this.transaction?.queueComplete();
    });
    return request;
  }

  getAll() {
    const request = new FakeRequest();
    queueMicrotask(() => {
      request.result = Array.from(this.store.records.values()).map(value => structuredClone(value));
      request.onsuccess?.();
      this.transaction?.queueComplete();
    });
    return request;
  }

  index(name) {
    const keyPath = this.store.indexes.get(name);
    if (!keyPath) {
      throw new Error(`missing index ${name}`);
    }
    return {
      get: value => {
        const request = new FakeRequest();
        queueMicrotask(() => {
          const record = Array.from(this.store.records.values()).find(item => item[keyPath] === value);
          request.result = cloneOrUndefined(record);
          request.onsuccess?.();
          this.transaction?.queueComplete();
        });
        return request;
      }
    };
  }
}

class FakeRequest {
  result = undefined;
  error = null;
  onsuccess = null;
  onerror = null;
  onupgradeneeded = null;
}

function cloneOrUndefined(value) {
  return value === undefined ? undefined : structuredClone(value);
}

await main();
