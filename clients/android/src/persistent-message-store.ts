import {
  GlobalLocalStorageKeyValueStorage,
  KeyValueMessageStore
} from "@nexusim/client-core";

export interface AndroidPersistentMessageStoreOptions {
  namespace?: string;
}

export class AndroidPersistentMessageStore extends KeyValueMessageStore {
  constructor(options: AndroidPersistentMessageStoreOptions = {}) {
    super(new GlobalLocalStorageKeyValueStorage(), {
      namespace: options.namespace ?? "android"
    });
  }
}
