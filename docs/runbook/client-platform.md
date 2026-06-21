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
- The Web app is a shell and dependency declaration. It is not yet connected to
  the real backend until the client BFF endpoints are implemented.
- The PC desktop and Android packages currently define runtime / packaging
  contracts only. They do not yet produce `.msi`, `.exe`, `.apk`, or `.aab`
  artifacts.

## Next Work

1. Add `api-gateway` client BFF v0.1 endpoints for auth, conversations, messages,
   PullInbox and ACK.
2. Implement browser fetch / WebSocket adapters in `clients/web`.
3. Add IndexedDB local store adapter.
4. Add PC desktop Tauri runner and first local Windows installer.
5. Add Android runtime shell and first unsigned local APK.
6. Run LAN smoke against Windows / Mac backend IP.
