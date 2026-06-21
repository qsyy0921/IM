# NexusIM Android Client

Android target for the client platform.

The Android client will reuse `@nexusim/protocol` and `@nexusim/client-core`.
The UI/runtime choice remains React Native first unless a later ADR chooses
Flutter or native Kotlin for a concrete reason.

## Current Status

- Architecture and package boundary exist.
- First TypeScript runtime shell exists through `createAndroidPlatformAdapter`.
- The shell includes development-only session storage, an in-memory message
  cache, static lifecycle/network ports, and unsupported push/local wakeup
  notifications.
- No APK or AAB is produced yet.
- `app.config.json` records the intended first Android package metadata.

## Security Rules

- `AndroidDevelopmentSessionStore` is local-development only; access tokens must
  move to Android Keystore / encrypted storage before a production release.
- Current local message cache is in-memory only; production cache should use
  SQLite behind `LocalMessageStore`.
- Push notification integration must not bypass PullInbox reconciliation.
- Background sync must use server cursors and idempotency keys.
