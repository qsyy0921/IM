import { createClientRuntime } from "@nexusim/client-core";
import type { ClientRuntime, CreateClientRuntimeOptions } from "@nexusim/client-core";
import { createDesktopPlatformAdapter } from "./platform-adapter";
import type { DesktopPlatformAdapter, DesktopRuntimeConfig } from "./platform-contract";
import { loadDesktopRuntimeConfig } from "./runtime-config";

export interface DesktopClientRuntimeOptions
  extends Omit<CreateClientRuntimeOptions, "config" | "platform"> {
  config?: DesktopRuntimeConfig;
  platform?: DesktopPlatformAdapter;
}

export type DesktopClientRuntime = ClientRuntime & {
  config: DesktopRuntimeConfig;
  platform: DesktopPlatformAdapter;
};

export function createDesktopClientRuntime(
  options: DesktopClientRuntimeOptions = {}
): DesktopClientRuntime {
  const config = options.config ?? loadDesktopRuntimeConfig();
  const platform = options.platform ?? createDesktopPlatformAdapter({ config });
  const runtime = createClientRuntime({
    ...options,
    config,
    platform
  });
  return runtime as DesktopClientRuntime;
}
