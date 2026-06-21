export interface ClientRuntimeConfig {
  apiBaseURL: string;
  pushWebSocketURL: string;
  deviceID: string;
}

export function validateRuntimeConfig(config: ClientRuntimeConfig): ClientRuntimeConfig {
  assertURL(config.apiBaseURL, "apiBaseURL");
  assertURL(config.pushWebSocketURL, "pushWebSocketURL");
  if (config.deviceID.trim() === "") {
    throw new Error("deviceID is required");
  }
  return config;
}

function assertURL(value: string, name: string): void {
  try {
    const parsed = new URL(value);
    if (!["http:", "https:", "ws:", "wss:"].includes(parsed.protocol)) {
      throw new Error("unsupported protocol");
    }
  } catch (error) {
    throw new Error(`${name} must be a valid URL`, { cause: error });
  }
}
