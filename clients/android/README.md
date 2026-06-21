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
- First Kotlin native bridge skeleton exists under `native/`; it owns only the
  launch shell and bridge metadata, not session storage, local message facts or
  BFF calls.
- The WebView registers `NexusIMNative` as a read-only JavaScript bridge. It
  exposes only one method, `runtimeMetadata()`, with runtime metadata (`target`,
  bridge version and label), and does not expose token, storage, file-system or
  message APIs.
- The WebView uses `WebViewAssetLoader`, loads `appassets.androidplatform.net`,
  enables DOM storage for the shared TypeScript runtime, and keeps raw file /
  content access disabled.
- The reserved `sqlite` local store config fails closed through shared
  `NativeStoreReadiness` with reason `sqlite-native-bridge-unavailable` and
  expected bridge `android-sqlite`. The current shell still uses localStorage
  persistence until the Android SQLite bridge is implemented.
- `shell-config.example.json` records the low-permission WebView config bridge
  for local LAN endpoints and Android runtime identity. It can be rendered to
  `web/public/nexusim-shell-config.js` before a shell build.
- `npm --prefix clients run build:android-apk` is the first-stage APK wrapper.
  It prepares Web assets and then runs Gradle `:app:assembleDebug` when JDK 17+
  and Android SDK are available. Use
  `node clients/tools/build-android-apk.mjs --dry-run` to inspect the command
  and missing toolchain without building.
- Debug APKs explicitly allow cleartext HTTP / WebSocket traffic for local LAN
  smoke. Release builds set the same manifest placeholder to disallow
  cleartext by default.
- No APK or AAB is produced yet.
- `app.config.json` records the intended first Android package metadata.

## Security Rules

- `AndroidDevelopmentSessionStore` is local-development only; access tokens must
  move to Android Keystore / encrypted storage before a production release.
- Shell config is endpoint and identity metadata only. It must not contain
  gateway tokens, refresh tokens, passwords, private keys, or arbitrary native
  capability flags.
- `NexusIMNative` is a single-method metadata bridge only. It must stay
  read-only until a separate ADR defines native capability commands and their
  audit boundary.
- File and content access must stay disabled in the native WebView shell.
  Local LAN API / WebSocket endpoints are supplied through shell config and must
  still flow through `api-gateway` and `push-gateway`.
- Cleartext traffic is a debug-only allowance for local LAN testing. Release
  Android builds must keep cleartext disabled unless a later ADR introduces a
  narrower production transport policy.
- Current local message cache is in-memory only; production cache should use
  SQLite behind `LocalMessageStore`.
- Push notification integration must not bypass PullInbox reconciliation.
- Background sync must use server cursors and idempotency keys.

## Focused Checks

```powershell
npm --prefix clients run typecheck:android
npm --prefix clients run validate:android-native
npm --prefix clients run test:shell-config
npm --prefix clients run test:artifact-builders
node clients/tools/render-shell-config.mjs --input clients/android/shell-config.example.json
node clients/tools/build-android-apk.mjs --dry-run
```
