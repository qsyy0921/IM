# NexusIM Client Platform Brief

This is the short runbook entry for browser, PC desktop, and Android client
platform work. The full design lives in `docs/sdd/client-platform.md`.

## Current Scope

First slice:

- Browser-first IM client architecture with PC and Android package boundaries
  present from the beginning.
- Shared `protocol` and `client-core` packages.
- LAN-friendly config for `api-gateway` and `push-gateway`.
- Minimal MVP path: login, open conversation, send text, receive notify,
  PullInbox, AckDelivery, reconnect fallback.

## Boundaries

- Client talks to `api-gateway` and `push-gateway` only.
- `delivery-service` PullInbox remains the reliable source for displayed
  delivered messages.
- WebSocket notifications are wakeups, not durable message facts.
- Local store is cache/offline support, not server truth.
- PC desktop and Android are consumers of the same protocol/core packages.
- Runtime-specific behavior must enter through explicit platform ports:
  secure session store, local message store, network state, app lifecycle,
  wakeup notifications and runtime device identity.

## Language Choices

- Backend / client BFF: Go.
- Browser Web: TypeScript + React.
- Shared client protocol and sync core: TypeScript packages.
- Windows PC: Tauri shell; Rust is only a thin native bridge, while product UI
  and sync logic stay TypeScript.
- Android: TypeScript-first runtime shell; Kotlin is only a thin bridge for
  secure storage, notifications, lifecycle and SQLite if needed.
- AI / eval / offline tools: Python, outside the client runtime and outside
  durable business state.

## First Implementation Status

- `docs/sdd/client-platform.md` freezes the v0.1 architecture.
- `clients/` contains the first workspace skeleton and has passed focused
  workspace validation / typecheck / Web build:
  - `packages/protocol`
  - `packages/client-core`
  - `web`
  - `desktop`
  - `android`
- `api-gateway` now exposes the first client BFF HTTP/JSON surface for login,
  refresh, `me`, conversation list, PullInbox-backed conversation messages,
  send, ACK, contacts and receipt lookup. The BFF reuses the existing gateway
  facade and injects trusted downstream metadata; it does not read internal
  service tables. The BFF now also has first-stage HTTP route metrics and
  rate-limit adapter wiring, reusing api-gateway's existing rate limiter and
  low-cardinality `/debug/metrics` / `/metrics` pipeline.
  `/api/auth/logout` now revokes the current authenticated session by forwarding
  to identity-service `RevokeSession`; the BFF derives tenant / user / device /
  session only from the verified gateway token and ignores any caller-supplied
  body target.
- `clients/web` now has the first real browser adapters:
  - `BFFClient` maps HTTP/JSON BFF payloads into shared protocol types. It now
    lives in `@nexusim/client-core`; Web keeps only a compatibility re-export.
  - `BrowserPushTransport` connects to `push-gateway` WebSocket and handles
    server frames as wakeups. The shared implementation now lives in
    `@nexusim/client-core` as `WebSocketPushTransport`; Web keeps only a
    compatibility re-export.
  - `IndexedDBMessageStore` implements the local cache / cursor store behind
    the `LocalMessageStore` port.
- `clients/web` now also has `createBrowserPlatformAdapter`, which binds the
  browser shell to the shared `createClientRuntime` auth / send / ack / logout
  lifecycle. The browser session store is first-stage tab-scoped
  `sessionStorage`; production Web auth still needs an httpOnly-cookie or
  equivalent hardened session strategy.
- The Web shell accepts a narrow WebView bridge config through
  `globalThis.__NEXUSIM_CLIENT_SHELL__`. PC and Android shells can inject target,
  API / WebSocket addresses, device id, installation id, app version and session
  key so the same Web UI can identify itself as `windows-desktop` or `android`.
  This bridge is configuration-only: it does not expose file system access,
  broad native IPC or native token authority.
- `web/index.html` loads `nexusim-shell-config.js` before the app bundle.
  Browser mode uses the checked-in empty placeholder; desktop / Android shell
  builds can render their low-sensitive `shell-config.example.json` through
  `clients/tools/render-shell-config.mjs` and replace that placeholder for a
  local shell build.
- `clients/tools/prepare-shell-web-assets.mjs` is the target asset-prep
  wrapper for real shells. `npm --prefix clients run build:shell-assets:desktop`
  builds Web and injects the `windows-desktop` shell config into `web/dist`.
  `npm --prefix clients run build:shell-assets:android` builds Web, copies the
  dist into Android app assets, and injects the `android` shell config there.
- `IndexedDBMessageStore` now has a dependency-free first-stage persistence
  test harness covering cursor persistence, message ordering, pending send,
  accepted send stable-key migration, replay de-duplication and failed-send
  state.
- The Web shell is wired to login, connect push, list / manually open
  conversations, PullInbox, send text, AckDelivery and logout through those
  adapters. Logout calls BFF current-session revoke, disconnects WebSocket,
  clears IndexedDB local cache and resets UI session state.
- `clients/desktop` now has a first-stage TypeScript runtime adapter:
  `loadDesktopRuntimeConfig`, `createDesktopPlatformAdapter`, development-only
  session storage, localStorage-backed persistent message cache, static
  network/lifecycle ports and unsupported local wakeup notifications. This
  moves desktop beyond a pure contract, but it is not an installer yet.
- `clients/desktop/src-tauri` now has a first-stage Tauri v2 Rust runner
  skeleton with only a read-only `runtime_metadata` IPC command. `bundle.active`
  remains `false`, so this is not a local Windows artifact yet. The Web shell
  can invoke this command for diagnostics and fails closed on malformed metadata.
  Its `frontendDist` resolves to the shared prepared `clients/web/dist`, not a
  desktop-local duplicate Web build.
- `clients/android` now has a first-stage TypeScript runtime adapter:
  `loadAndroidRuntimeConfig`, `createAndroidPlatformAdapter`, development-only
  session storage, localStorage-backed persistent message cache, static
  network/lifecycle ports and unsupported push/local wakeup notifications. This
  moves Android beyond a pure contract, but it is not an APK yet.
- `clients/android/native` now has a first-stage Kotlin native bridge skeleton.
  It now uses an Android WebView asset shell through `WebViewAssetLoader`,
  loads the prepared local Web assets, and still does not own token storage,
  local message facts, BFF calls, or push delivery semantics.
- Android WebView now registers the read-only `NexusIMNative` JavaScript bridge.
  It exposes only the single `runtimeMetadata()` method. The Web runtime can
  parse that metadata for diagnostics, and invalid bridge payloads fail closed.
  The bridge exposes no token, storage, file-system or message APIs.
- The Web shell runtime panel now displays shell target plus PC Tauri or Android
  native bridge metadata when present. This is diagnostics only; it does not
  grant Web code native capabilities.
- PC desktop and Android can reuse the same `@nexusim/client-core` BFF adapter
  instead of copying Web-private HTTP mapping code.
- PC desktop and Android can also reuse the same `@nexusim/client-core`
  WebSocket push transport for online wakeups.
- `@nexusim/client-core` now exposes `createClientRuntime`, wiring BFF API,
  push transport, auth session manager, inbox sync, send queue and ack queue.
  Desktop and Android expose `createDesktopClientRuntime` /
  `createAndroidClientRuntime` over their platform adapters. The shared runtime
  now owns the first-stage auth lifecycle: `login` and `refresh` persist the
  returned session into the platform secure session store, `restoreSession`
  hydrates the auth manager from that store after a runtime restart, and
  `logout` calls BFF logout, disconnects push, clears secure session storage and
  clears local message cache.
- `@nexusim/client-core` now exposes `KeyValueMessageStore`, a reusable
  string-KV persistent store for non-browser targets. Desktop and Android use
  first-stage WebView `localStorage` wrappers by default; future native SQLite
  adapters can replace only the storage/platform port. Desktop and Android
  `sqlite` config is reserved and fails fast until that bridge exists.
  `LocalMessageStore.clear`
  is now part of the shared port so logout can remove cached messages, cursors
  and pending sends consistently across targets.
- `LocalMessageStore.listMessages` is now part of the shared port. Web,
  desktop and Android shell UI can read cached messages through the same local
  read-model boundary; `MemoryMessageStore`, `KeyValueMessageStore` and
  `IndexedDBMessageStore` now share the same pending / accepted-send migration
  expectations.
- `npm --prefix clients run test:runtime-lifecycle` is the first focused
  desktop / Android runtime lifecycle smoke. It compiles the TypeScript runtime
  packages locally, instantiates both platform runtime factories, and verifies
  login persistence, session restore, refresh-token persistence and logout cache
  cleanup without requiring Tauri CLI, Android SDK or network access. It also
  exercises the desktop / Android thin shell actions that a real PC or Android
  UI can call for login, refresh, restore and logout.
- `npm --prefix clients run test:web-platform` covers browser session storage,
  browser runtime identity, network/lifecycle ports and unsupported wakeup
  boundaries, plus WebView bridge target selection for desktop and Android,
  without requiring a live browser or backend.
- `npm --prefix clients run test:web-shell-actions` guards the Web shell
  lifecycle contract. It verifies the Web shell binds login, refresh, restore
  and logout through shared `ClientShellActions` and does not call runtime auth
  lifecycle methods directly, so PC / Android WebView shells can keep the same
  UI action path.
- `npm --prefix clients run test:shell-config` validates the desktop / Android
  shell config templates and renderer, and rejects unsupported targets or
  sensitive fields such as token, secret and password.
- `npm --prefix clients run test:shell-web-assets` validates the target asset
  prep wrapper without requiring Tauri CLI, Android SDK or a live backend. It
  also checks stale bundle cleanup so Android / desktop shell outputs do not
  retain old Web assets across builds, and verifies the low-sensitive
  `nexusim-shell-assets-manifest.json` with relative paths, byte sizes and
  SHA-256 hashes. Native artifact wrappers call the same manifest verifier
  after preparing assets and before invoking Tauri / Gradle.
- `npm --prefix clients run test:artifact-builders` validates the first-stage
  desktop artifact / Android APK build wrappers in dry-run mode. Real build
  commands are present as `build:desktop-artifact` and `build:android-apk`, but
  they fail fast with missing-toolchain JSON until the local Tauri / Android
  toolchains are installed.
- `npm --prefix clients run test:artifact-collector` validates the first-stage
  artifact collector. Once a real desktop installer or Android APK exists,
  `npm --prefix clients run collect:client-artifacts` copies it into ignored
  `clients/artifacts/<run-id>/` storage and writes a low-sensitive manifest with
  file names, sizes and SHA-256 hashes, without recording local absolute source
  paths. `build:desktop-artifact:collect` and `build:android-apk:collect` run the
  collector automatically after a successful native build.
- `npm --prefix clients run test:artifact-install-plan` validates the
  first-stage install-plan tool. `npm --prefix clients run plan:artifact-install`
  reads a collected `clients/artifacts/<run-id>/manifest.json` and prints
  low-sensitive Windows installer / Android APK install checklist commands. It
  now also reports install-side readiness such as Android `adb` availability
  and Windows local installer launch support, while still not launching
  installers, contacting devices, installing packages or printing local
  absolute paths.
- `npm --prefix clients run report:artifact-readiness` prints a low-sensitive
  readiness matrix for local desktop, local Android and Android Docker builder
  paths. It reports missing capabilities and the exact next build command
  without printing local absolute paths. It also includes per-target prepared
  shell asset verification status. It separates the Android Docker builder image
  build command from the actual builder run command and emits low-sensitive
  `nextActions`; it never starts a download or build by itself.
- `npm --prefix clients run plan:shell-smoke` prints a low-sensitive browser /
  desktop / Android shell smoke plan. It combines toolchain readiness, prepared
  asset verification, artifact presence, collected-artifact install readiness,
  safe build commands and the shared BFF / push smoke command; it does not
  launch services, connect devices, install artifacts or install toolchains.
  Native artifact status distinguishes raw build-output discovery from the
  collected artifact manifest that drives manual install readiness.
- `npm --prefix clients run validate:builder-profile` validates the Android
  Docker builder profile without building or pulling images. The profile lives
  in `deploy/local/docker-compose.client-builders.yml` and uses
  `deploy/docker/client-android-builder.Dockerfile`; it is opt-in because the
  first image build downloads Node and Android SDK components. The profile now
  runs `build:android-apk:collect`, so a successful container build writes the
  APK and low-sensitive `manifest.json` under
  `clients/artifacts/android/docker-android-debug/` by default.
- `@nexusim/client-core` now exposes `ClientShellActions`; desktop and Android
  export thin `createDesktopShellActions` / `createAndroidShellActions` wrappers.
  These wrappers do not own business logic; they only bind shell UI actions to
  the shared runtime lifecycle. The Web shell login panel now uses the shared
  shell action contract for login, refresh, restore and logout, matching the
  desktop / Android thin shell action coverage. PC / Android WebView shells do
  not need a separate UI lifecycle path.
- `loadtest/clientweb` provides the first scriptable client-path smoke. Setup
  uses public gRPC APIs to register users, seed the conversation owner and create
  the receiver JOIN; the verified client path then uses only HTTP BFF and
  WebSocket: login, push hello, send, notify, PullInbox, conversation list and
  AckDelivery. `loadtest/clientweb/run-local-smoke.ps1` starts a local private
  non-TLS backend+BFF+push stack for this smoke. The script supports
  `-BindHost` and `-ClientHost` so the same client path can bind to a private
  wired LAN address such as `172.31.50.1` instead of loopback. The first WIP local smoke
  passed on 2026-06-21; report:
  `docs/runbook/loadtest/client-platform/loadtest-report-20260621-client-web-bff-push-smoke.md`.
  The first clean committed baseline also passed on 2026-06-21; report:
  `docs/runbook/loadtest/client-platform/loadtest-report-20260621-client-web-bff-push-clean-baseline.md`.
  A WIP `172.31.50.1` wired-address smoke also passed on 2026-06-21, but it was
  recorded with `git_dirty=true`; clean committed rerun remains required before
  using it as a baseline.
  The clean committed `172.31.50.1` wired-address baseline passed on 2026-06-21;
  report:
  `docs/runbook/loadtest/client-platform/loadtest-report-20260621-client-web-bff-push-wired-172-clean-baseline.md`.
  It does not replace existing secure mTLS gateway / push smoke coverage.
- PC desktop and Android now both have first-stage TypeScript runtime adapters.
  PC desktop also has a Tauri runner skeleton, and Android has a Kotlin native
  WebView asset shell skeleton. PC exposes only read-only runtime metadata IPC
  and Web can read it for diagnostics; Android exposes only a single-method
  read-only metadata JavaScript bridge. Both targets reserve native SQLite store
  config and fail closed until native bridges exist. Neither target produces
  `.msi`, `.exe`, `.apk`, or `.aab` artifacts yet; the repository now has
  dry-run-tested build wrappers for those future artifacts.
- `/api/auth/logout` performs first-stage server-side logout for the current
  authenticated session only. Broader device/session management remains an
  identity/admin capability, not a client BFF target selector.

## BFF Runtime Config

The BFF is disabled by default and starts inside `api-gateway` `grpc` mode when
`NEXUSIM_API_GATEWAY_BFF_ADDR` is set.

```powershell
$env:NEXUSIM_API_GATEWAY_BFF_ADDR="172.31.50.10:8080"
$env:NEXUSIM_API_GATEWAY_BFF_ALLOWED_ORIGINS="http://localhost:5173,http://172.31.50.10:5173"
```

Binding to `0.0.0.0` or another non-private listener requires
`NEXUSIM_API_GATEWAY_BFF_ALLOW_PUBLIC=true`, and public mock auth is still
rejected. For local LAN client smoke, prefer the wired `172.x.x.x` address.

## Next Work

1. Add first local Windows artifact from the PC desktop Tauri runner.
2. Add first unsigned local APK from the Android native bridge.
3. Wire logout UI controls into the real desktop and Android shells, and run a
   platform-shell smoke once packaging/runtime tooling is ready.
4. Replace first-stage desktop / Android localStorage stores with native SQLite
   bridge adapters when packaging/runtime tooling is ready.

## Local Build Prerequisites

- PC artifact build needs Tauri CLI / `cargo-tauri`; the current repository has
  the runner skeleton and validator, not the local CLI dependency.
- Android APK build needs JDK 17+ plus Gradle / Android SDK. If those are not
  installed locally, use a Docker / CI builder profile instead of claiming an
  APK baseline.
- The repository now includes an Android Docker builder profile wired to the
  same artifact collector, but it has not been run in this slice and therefore
  does not prove an APK baseline yet.

Focused local check:

```powershell
npm --prefix clients run check:build-prereqs
npm --prefix clients run test:shell-config
npm --prefix clients run test:shell-web-assets
npm --prefix clients run test:artifact-builders
npm --prefix clients run test:artifact-collector
npm --prefix clients run test:artifact-install-plan
npm --prefix clients run test:artifact-readiness
npm --prefix clients run test:shell-smoke-plan
npm --prefix clients run report:artifact-readiness
npm --prefix clients run validate:builder-profile
```

This command reports readiness as JSON and exits non-zero when artifact / APK
toolchains are missing. It is local-only: it does not install dependencies, pull
packages, or use `npx` to resolve remote CLIs.

The readiness report is non-failing and is useful before deciding whether to
install native toolchains or run the Docker builder. It also reports whether
current prepared shell assets verify against `nexusim-shell-assets-manifest.json`:

```powershell
npm --prefix clients run report:artifact-readiness
npm --prefix clients run plan:shell-smoke
npm --prefix clients run plan:artifact-install
```

The report includes `nextActions`. When the Android Docker builder image is
missing, the first Android next action is the explicit image build command. After
the image exists, the next action becomes the builder run command that writes the
APK and manifest.

Artifact wrappers:

```powershell
node clients/tools/build-desktop-artifact.mjs --dry-run
node clients/tools/build-android-apk.mjs --dry-run
node clients/tools/collect-client-artifacts.mjs --target all --dry-run
npm --prefix clients run build:desktop-artifact
npm --prefix clients run build:android-apk
npm --prefix clients run build:desktop-artifact:collect
npm --prefix clients run build:android-apk:collect
npm --prefix clients run collect:client-artifacts
docker compose -f deploy/local/docker-compose.client-builders.yml --profile client-builders build client-android-apk-builder
docker compose -f deploy/local/docker-compose.client-builders.yml --profile client-builders run --rm client-android-apk-builder
```

After preparing both shell targets, `node clients/tools/verify-shell-assets.mjs --target all`
checks both prepared asset directories. Per-target native build wrappers run the
matching verifier automatically after asset prep.
