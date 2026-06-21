import {
  GlobalLocalStorageKeyValueStorage,
  KeyValueMessageStore
} from "@nexusim/client-core";
import type { StringKeyValueStorage } from "@nexusim/client-core";

export interface DesktopPersistentMessageStoreOptions {
  namespace?: string;
  storage?: StringKeyValueStorage;
}

export class DesktopPersistentMessageStore extends KeyValueMessageStore {
  constructor(options: DesktopPersistentMessageStoreOptions = {}) {
    super(options.storage ?? new GlobalLocalStorageKeyValueStorage(), {
      namespace: options.namespace ?? "desktop"
    });
  }
}
