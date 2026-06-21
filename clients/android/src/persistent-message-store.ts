import {
  GlobalLocalStorageKeyValueStorage,
  KeyValueMessageStore
} from "@nexusim/client-core";
import type { StringKeyValueStorage } from "@nexusim/client-core";

export interface AndroidPersistentMessageStoreOptions {
  namespace?: string;
  storage?: StringKeyValueStorage;
}

export class AndroidPersistentMessageStore extends KeyValueMessageStore {
  constructor(options: AndroidPersistentMessageStoreOptions = {}) {
    super(options.storage ?? new GlobalLocalStorageKeyValueStorage(), {
      namespace: options.namespace ?? "android"
    });
  }
}
