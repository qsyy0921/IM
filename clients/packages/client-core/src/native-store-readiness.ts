export type NativeStoreTarget = "windows-desktop" | "android";

export type RequestedLocalStore = "memory" | "local-storage" | "sqlite";

export type NativeStoreBridge = "none" | "tauri-sqlite" | "android-sqlite";

export type NativeStoreReadinessReason = "" | "sqlite-native-bridge-unavailable";

export interface NativeStoreReadiness {
  target: NativeStoreTarget;
  requestedStore: RequestedLocalStore;
  ready: boolean;
  reason: NativeStoreReadinessReason;
  bridge: NativeStoreBridge;
  nextAction: string;
}

export interface NativeStoreReadinessInput {
  target: NativeStoreTarget;
  requestedStore: RequestedLocalStore;
  nativeBridgeAvailable?: boolean;
}

export class NativeStoreUnavailableError extends Error {
  readonly readiness: NativeStoreReadiness;

  constructor(readiness: NativeStoreReadiness) {
    super(nativeStoreUnavailableMessage(readiness));
    this.name = "NativeStoreUnavailableError";
    this.readiness = readiness;
  }
}

export function nativeStoreReadiness(
  input: NativeStoreReadinessInput
): NativeStoreReadiness {
  if (input.requestedStore !== "sqlite") {
    return {
      target: input.target,
      requestedStore: input.requestedStore,
      ready: true,
      reason: "",
      bridge: "none",
      nextAction: ""
    };
  }

  const bridge = sqliteBridgeForTarget(input.target);
  const ready = input.nativeBridgeAvailable === true;
  return {
    target: input.target,
    requestedStore: "sqlite",
    ready,
    reason: ready ? "" : "sqlite-native-bridge-unavailable",
    bridge,
    nextAction: ready
      ? ""
      : `${bridge} is required before ${input.target} can use sqlite local store`
  };
}

export function assertNativeStoreReady(
  input: NativeStoreReadinessInput
): NativeStoreReadiness {
  const readiness = nativeStoreReadiness(input);
  if (!readiness.ready) {
    throw new NativeStoreUnavailableError(readiness);
  }
  return readiness;
}

function sqliteBridgeForTarget(target: NativeStoreTarget): NativeStoreBridge {
  if (target === "windows-desktop") {
    return "tauri-sqlite";
  }
  return "android-sqlite";
}

function nativeStoreUnavailableMessage(readiness: NativeStoreReadiness): string {
  return [
    `Native sqlite local store is not ready for ${readiness.target}`,
    `reason=${readiness.reason}`,
    `bridge=${readiness.bridge}`
  ].join("; ");
}
