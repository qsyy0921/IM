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
  service tables.
- `clients/web` now has the first real browser adapters:
  - `BFFClient` maps HTTP/JSON BFF payloads into shared protocol types.
  - `BrowserPushTransport` connects to `push-gateway` WebSocket and handles
    server frames as wakeups.
  - `IndexedDBMessageStore` implements the local cache / cursor store behind
    the `LocalMessageStore` port.
- The Web shell is wired to login, connect push, list / manually open
  conversations, PullInbox, send text and AckDelivery through those adapters.
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
  It does not replace existing secure mTLS gateway / push smoke coverage.
- The PC desktop and Android packages currently define runtime / packaging
  contracts only. They do not yet produce `.msi`, `.exe`, `.apk`, or `.aab`
  artifacts.
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

1. Rerun `loadtest/clientweb/run-local-smoke.ps1 -BindHost 172.31.50.1
   -ClientHost 172.31.50.1` from a clean commit and archive the wired-address
   baseline.
2. Add HTTP-layer BFF metrics / rate-limit adapter; current BFF calls the
   gateway facade directly and does not pass through gRPC interceptors.
3. Add PC desktop Tauri runner and first local Windows installer.
4. Add Android runtime shell and first unsigned local APK.
