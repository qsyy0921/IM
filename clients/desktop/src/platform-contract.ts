import type { ClientPlatformAdapter, ClientRuntimeConfig } from "@nexusim/client-core";

export type DesktopOS = "windows" | "macos" | "linux";

export interface DesktopRuntimeConfig extends ClientRuntimeConfig {
  os: DesktopOS;
  secureStorage: "windows-credential-manager" | "keychain" | "secret-service" | "development";
  localStore: "sqlite" | "local-storage" | "memory";
  shell: "tauri";
}

export type DesktopPlatformAdapter = ClientPlatformAdapter & {
  identity: ClientPlatformAdapter["identity"] & {
    target: "windows-desktop";
  };
};

export interface DesktopPackagingTarget {
  os: DesktopOS;
  artifact: "msi" | "nsis" | "dmg" | "app" | "deb" | "appimage";
  signed: boolean;
}

export const FIRST_DESKTOP_TARGET: DesktopPackagingTarget = {
  os: "windows",
  artifact: "msi",
  signed: false
};
