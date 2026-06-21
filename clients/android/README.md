# NexusIM Android Client

Android target for the client platform.

The Android client will reuse `@nexusim/protocol` and `@nexusim/client-core`.
The UI/runtime choice remains React Native first unless a later ADR chooses
Flutter or native Kotlin for a concrete reason.

## Current Status

- Architecture and package boundary exist.
- No APK or AAB is produced yet.
- `app.config.json` records the intended first Android package metadata.

## Security Rules

- Access tokens must move to Android Keystore / encrypted storage before a
  production release.
- Local message cache should use SQLite behind `LocalMessageStore`.
- Push notification integration must not bypass PullInbox reconciliation.
- Background sync must use server cursors and idempotency keys.
