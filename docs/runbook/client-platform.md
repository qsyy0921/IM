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
- `clients/web` now has the first real browser adapters:
  - `BFFClient` maps HTTP/JSON BFF payloads into shared protocol types. It now
    lives in `@nexusim/client-core`; Web keeps only a compatibility re-export.
  - `BrowserPushTransport` connects to `push-gateway` WebSocket and handles
    server frames as wakeups.
  - `IndexedDBMessageStore` implements the local cache / cursor store behind
    the `LocalMessageStore` port.
- `IndexedDBMessageStore` now has a dependency-free first-stage persistence
  test harness covering cursor persistence, message ordering, pending send,
  accepted send stable-key migration, replay de-duplication and failed-send
  state.
- The Web shell is wired to login, connect push, list / manually open
  conversations, PullInbox, send text and AckDelivery through those adapters.
- `clients/desktop` now has a first-stage TypeScript runtime adapter:
  `loadDesktopRuntimeConfig`, `createDesktopPlatformAdapter`, development-only
  session storage, in-memory message cache, static network/lifecycle ports and
  unsupported local wakeup notifications. This moves desktop beyond a pure
  contract, but it is not a Tauri runner or installer yet.
- `clients/desktop/src-tauri` now has a first-stage Tauri v2 Rust runner
  skeleton with no IPC commands. `bundle.active` remains `false`, so this is not
  a local Windows artifact yet.
- `clients/android` now has a first-stage TypeScript runtime adapter:
  `loadAndroidRuntimeConfig`, `createAndroidPlatformAdapter`, development-only
  session storage, in-memory message cache, static network/lifecycle ports and
  unsupported push/local wakeup notifications. This moves Android beyond a pure
  contract, but it is not a native bridge or APK yet.
- `clients/android/native` now has a first-stage Kotlin native bridge skeleton.
  It owns only launch shell / metadata and does not own token storage, local
  message facts, BFF calls, or push delivery semantics.
- PC desktop and Android can reuse the same `@nexusim/client-core` BFF adapter
  instead of copying Web-private HTTP mapping code.
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
  bridge skeleton. Neither target produces `.msi`, `.exe`, `.apk`, or `.aab`
  artifacts yet.
- `/api/auth/logout` is reserved and currently returns `UNIMPLEMENTED`; identity
  still needs a user self-session revoke contract before server-side logout is
  real.

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
3. Move desktop / Android from in-memory development stores to durable platform
   stores and add cursor replay tests for those stores.

## Local Build Prerequisites

- PC artifact build needs Tauri CLI / `cargo-tauri`; the current repository has
  the runner skeleton and validator, not the local CLI dependency.
- Android APK build needs JDK 17+ plus Gradle / Android SDK. If those are not
  installed locally, use a Docker / CI builder profile instead of claiming an
  APK baseline.
