import type { ClientPlatformAdapter, ClientRuntimeConfig } from "@nexusim/client-core";

export interface AndroidRuntimeConfig extends ClientRuntimeConfig {
  platform: "android";
  secureStorage: "android-keystore" | "development";
  localStore: "sqlite" | "local-storage" | "memory";
  notificationProvider: "none" | "fcm";
}

export type AndroidPlatformAdapter = ClientPlatformAdapter & {
  identity: ClientPlatformAdapter["identity"] & {
    target: "android";
  };
};

export interface AndroidPackagingTarget {
  artifact: "apk" | "aab";
  signed: boolean;
  minSdk: number;
  targetSdk: number;
}

export const FIRST_ANDROID_TARGET: AndroidPackagingTarget = {
  artifact: "apk",
  signed: false,
  minSdk: 26,
  targetSdk: 35
};
