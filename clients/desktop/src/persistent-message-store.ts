import {
  GlobalLocalStorageKeyValueStorage,
  KeyValueMessageStore
} from "@nexusim/client-core";

export interface DesktopPersistentMessageStoreOptions {
  namespace?: string;
}

export class DesktopPersistentMessageStore extends KeyValueMessageStore {
  constructor(options: DesktopPersistentMessageStoreOptions = {}) {
    super(new GlobalLocalStorageKeyValueStorage(), {
      namespace: options.namespace ?? "desktop"
    });
  }
}
