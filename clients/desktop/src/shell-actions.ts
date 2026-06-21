import { createClientShellActions } from "@nexusim/client-core";
import type { ClientShellActions } from "@nexusim/client-core";
import type { DesktopClientRuntime } from "./runtime";

export function createDesktopShellActions(
  runtime: DesktopClientRuntime
): ClientShellActions {
  return createClientShellActions(runtime);
}
