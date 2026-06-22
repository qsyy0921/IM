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
- First Kotlin native bridge skeleton exists under `native/`; it owns the
  launch shell, runtime metadata and a fixed-prefix SQLite key-value local-cache
  bridge, not session storage, server-side message facts or BFF calls.
- The WebView registers `NexusIMNative` as a narrow JavaScript bridge. It
  exposes `runtimeMetadata()` plus fixed `localStoreGetItem` /
  `localStoreSetItem` / `localStoreRemoveItem` methods. The local-store methods
  are restricted to the shared `nexusim:client-message-store:v1:` cache prefix
  and do not expose token, file-system, content-provider or message API access.
- The WebView uses `WebViewAssetLoader`, loads `appassets.androidplatform.net`,
  enables DOM storage for the shared TypeScript runtime, and keeps raw file /
  content access disabled.
- The reserved `sqlite` local store config fails closed through shared
  `NativeStoreReadiness` with reason `sqlite-native-bridge-unavailable` and
  expected bridge `android-sqlite` unless an explicit native key-value bridge is
  injected. The TypeScript-side bridge contract is in place and covered by
  focused tests. Web runtime discovery now only enables it when metadata reports
  ready and `NexusIMNative` exposes all `localStore*` methods; the current
  WebView shell can pass that ready bridge into the shared `KeyValueMessageStore`.
  The Kotlin source bridge now reports ready metadata; APK build and real-device
  WebView smoke are still pending before this is treated as a runtime baseline.
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
- `NexusIMNative` may expose only `runtimeMetadata` and the fixed local-store
  key-value methods. Local-store keys must stay constrained to the shared client
  message cache prefix; no token, file-system, content-provider or arbitrary
  native commands are allowed.
- File and content access must stay disabled in the native WebView shell.
  Local LAN API / WebSocket endpoints are supplied through shell config and must
  still flow through `api-gateway` and `push-gateway`.
- Cleartext traffic is a debug-only allowance for local LAN testing. Release
  Android builds must keep cleartext disabled unless a later ADR introduces a
  narrower production transport policy.
- Current Android source includes the fixed-prefix SQLite local-store bridge,
  but APK build and real-device WebView smoke still have to prove it as a
  runtime baseline. Until then, Android uses the explicitly configured shared
  TypeScript localStorage-backed store and must not silently switch storage
  backends.
- Push notification integration must not bypass PullInbox reconciliation.
- Background sync must use server cursors and idempotency keys.

## Focused Checks

```powershell
npm --prefix clients run typecheck:android
npm --prefix clients run validate:android-native
npm --prefix clients run test:shell-config
npm --prefix clients run test:android-shell-action-assets
npm --prefix clients run test:artifact-builders
npm --prefix clients run test:android-platform-readiness
npm --prefix clients run report:android-platform-readiness
node clients/tools/render-shell-config.mjs --input clients/android/shell-config.example.json
node clients/tools/build-android-apk.mjs --dry-run
```
