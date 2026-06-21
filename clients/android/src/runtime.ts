import { createClientRuntime } from "@nexusim/client-core";
import type { ClientRuntime, CreateClientRuntimeOptions } from "@nexusim/client-core";
import { createAndroidPlatformAdapter } from "./platform-adapter";
import type { AndroidPlatformAdapter, AndroidRuntimeConfig } from "./platform-contract";
import { loadAndroidRuntimeConfig } from "./runtime-config";

export interface AndroidClientRuntimeOptions
  extends Omit<CreateClientRuntimeOptions, "config" | "platform"> {
  config?: AndroidRuntimeConfig;
  platform?: AndroidPlatformAdapter;
}

export type AndroidClientRuntime = ClientRuntime & {
  config: AndroidRuntimeConfig;
  platform: AndroidPlatformAdapter;
};

export function createAndroidClientRuntime(
  options: AndroidClientRuntimeOptions = {}
): AndroidClientRuntime {
  const config = options.config ?? loadAndroidRuntimeConfig();
  const platform = options.platform ?? createAndroidPlatformAdapter({ config });
  const runtime = createClientRuntime({
    ...options,
    config,
    platform
  });
  return runtime as AndroidClientRuntime;
}
