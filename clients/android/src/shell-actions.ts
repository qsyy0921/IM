import { createClientShellActions } from "@nexusim/client-core";
import type { ClientShellActions } from "@nexusim/client-core";
import type { AndroidClientRuntime } from "./runtime";

export function createAndroidShellActions(
  runtime: AndroidClientRuntime
): ClientShellActions {
  return createClientShellActions(runtime);
}
