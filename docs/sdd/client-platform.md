# Client Platform SDD v0.1

## Status

Draft for the first NexusIM client platform slice.

This SDD defines the browser, PC desktop, and Android client architecture. Web
is implemented first, while PC and Android keep explicit package boundaries from
the beginning so synchronization, storage, and API contracts are not rewritten
later.

## Goals

- Build a LAN-runnable IM client MVP on top of the existing backend.
- Keep the first slice small while preserving production-grade boundaries.
- Build browser, PC desktop, and Android clients from the same protocol and
  client-core contracts.
- Make the browser the first executable client, then package the same UI for
  PC desktop, then bring up Android with the same sync and queue semantics.
- Keep client-facing APIs behind `api-gateway` and online wakeup behind
  `push-gateway`.
- Treat `delivery-service` PullInbox as the reliable message source; WebSocket
  notifications are only wakeups.

## Non-Goals

- Do not make the client read internal service tables.
- Do not connect the browser directly to every backend microservice.
- Do not produce full PC or Android installers in this slice.
- Do not add media upload, voice, video, complex group management, or full
  offline conflict resolution in this slice.
- Do not claim production client security until token storage, CSP, device
  binding, signed updates, crash reporting, and release governance are covered.

## External Design Baselines

The first client slice follows these stable ideas, adapted to NexusIM:

- Matrix-style separation of timeline state and sync state: the client pulls
  ordered events and does not treat push as the truth source.
- WhatsApp-style multi-device thinking: every device has its own connection and
  delivery cursor.
- Slack / Tauri style desktop path: Web UI first, audited desktop shell second.
- Android multi-device path: Android has its own device id and delivery cursor,
  not a mirror of the browser or PC session.
- Local-first/offline-first discipline: local cache improves UX, but server
  facts remain authoritative for message delivery and cursor state.
- Backend-for-Frontend boundary: client talks to a small client API surface, not
  to internal service topology.

## Topology

```text
Browser client / PC desktop shell / Android app
  -> api-gateway client API
      -> identity-service
      -> contacts-service
      -> conversation-service
      -> message-service
      -> delivery-service
      -> receipt-service
      -> policy-service
  -> push-gateway WebSocket
      -> delivery notify wakeups
```

The browser only needs two runtime base URLs:

```text
NEXUSIM_API_BASE=http://<lan-host>:<api-gateway-port>
NEXUSIM_WS_URL=ws://<lan-host>:<push-gateway-port>/ws
```

The LAN host may be a Windows or Mac machine. For the direct cable topology,
use the `172.x.x.x` wired address; for phones on Wi-Fi, use a routable LAN
address that the phone can reach.

## Repository Layout

```text
clients/
  package.json
  tsconfig.base.json
  README.md
  tools/
    validate-client-workspace.mjs
  packages/
    protocol/
      src/
    client-core/
      src/
  web/
    src/
  desktop/
    src/
    src-tauri/
  android/
    src/
```

`protocol` contains typed API and WebSocket contracts. `client-core` owns sync,
push, send queue, ack queue, and local store ports. `web` is the first UI
consumer. `desktop` records the Tauri shell boundary for PC packaging. `android`
records Android runtime and packaging constraints before the full native app is
created.

Desktop and Android shell builds use the same Web UI bundle. A target-specific
`nexusim-shell-config.js` is rendered from low-sensitive shell config JSON before
packaging. The config may identify runtime target, LAN API / WebSocket endpoints,
device id, installation id, app version and session namespace only; it must not
carry auth tokens, secrets, credentials or broad native permissions.

## Implementation Languages

The production business split is intentionally boring and explicit:

- Go: backend microservices, `api-gateway` client BFF endpoints, control-plane
  decisions, durable business state, authorization, repair and audit.
- TypeScript: browser UI, shared client protocol contracts, `client-core`
  synchronization algorithms, desktop UI reuse and Android UI/runtime shell.
- Rust: Tauri native shell and narrowly scoped OS bridge commands only. Rust
  must not reimplement PullInbox, ACK, authorization or business workflows.
- Kotlin: Android native adapter only when TypeScript cannot safely reach a
  platform capability, such as secure storage, notification registration,
  lifecycle hooks or SQLite bridge.
- Python: AI workers, model/algorithm candidates, eval and offline tooling
  only. Python must not own client product code, server business state,
  authorization, audit or delivery facts.

This lets the real business system keep Go as the state/control language while
using TypeScript for user-facing product code. Native languages stay at the
edge where the operating system requires them.

## Client Layers

```text
web / desktop / android UI
  -> client-core facades
      -> protocol API clients
      -> local-store port
      -> push WebSocket port
```

Rules:

- React components must not own PullInbox, ACK, reconnect, or send retry
  algorithms.
- `client-core` must not import React, browser DOM APIs, or UI components.
- `protocol` must stay pure TypeScript types plus transport-neutral endpoint
  descriptions.
- `web` can adapt browser fetch, WebSocket, IndexedDB, and environment config.
- `desktop` must keep native IPC narrow and must not expose a broad file-system
  bridge to Web code.
- `android` must reuse the same protocol / core contracts and put native
  storage, notification, and lifecycle behavior behind adapters.

## Runtime Targets

### Browser

Browser is the first executable target.

Responsibilities:

- Render the first IM shell.
- Use `fetch` against `api-gateway` client BFF endpoints.
- Use browser `WebSocket` against `push-gateway`.
- Use IndexedDB behind `LocalMessageStore`.
- Keep tokens out of plain localStorage in production mode.

Browser constraints:

- No native file-system access.
- No direct connection to backend internal gRPC ports.
- Service Worker / Push API can be added later, but any push wakeup must still
  reconcile through PullInbox.

### Windows PC

PC desktop is the first packaged desktop target. The default shell is Tauri.

Responsibilities:

- Reuse Web UI and `client-core`.
- Store tokens through Windows Credential Manager before production release.
- First-stage runtime may use a WebView `localStorage` backed
  `KeyValueMessageStore` for cursor / cache persistence. Target production
  runtime should replace that storage port with SQLite behind `LocalMessageStore`.
- Keep native IPC as explicit commands only.
- Produce local `.msi` / `.exe` installer after the Web MVP is connected.
- After an installer is produced, archive it through the client artifact
  collector and keep only the low-sensitive SHA-256 manifest under ignored local
  artifact storage.

PC constraints:

- No broad native bridge. First-stage Tauri IPC may expose only read-only
  runtime metadata until a separate native capability ADR defines commands,
  audit and permission checks.
- Web shell may call only the fixed `runtime_metadata` command for diagnostics.
  It must not construct arbitrary native command names or pass business payloads
  into the native bridge.
- No arbitrary file-system access from Web code.
- No auto-update before signing and update-channel governance are defined.

### Android

Android is the first mobile target.

Responsibilities:

- Reuse `protocol` and `client-core`.
- Use Android Keystore / encrypted storage for auth material before production.
- First-stage runtime may use a WebView `localStorage` backed
  `KeyValueMessageStore` for cursor / cache persistence. Target production
  runtime should replace that storage port with SQLite behind `LocalMessageStore`.
- Integrate FCM later as wakeup only.
- Produce a local unsigned `.apk` before signed distribution.
- After an APK is produced, archive it through the client artifact collector and
  keep only the low-sensitive SHA-256 manifest under ignored local artifact
  storage.

Android constraints:

- Android push notification payload must not be treated as delivered message
  content.
- Background sync must use PullInbox and device cursor state.
- First-stage Android shell loads prepared local Web assets through
  `WebViewAssetLoader` instead of granting broad `file://` access to Web code.
- First-stage `NexusIMNative` JavaScript bridge is a single-method
  metadata-only bridge. It may expose runtime target, bridge version and label
  through `runtimeMetadata()` for diagnostics, but it must not expose tokens,
  storage, file-system access, message facts or write commands until a separate
  native capability ADR defines audit and permission checks.
- Offline sends must use idempotency keys and local pending queues.

## Platform Adapter Ports

`client-core` owns sync, send, push, ACK, and retry algorithms. Runtime-specific
behavior enters through these ports:

```text
SecureSessionStore
LocalMessageStore
NetworkStatePort
AppLifecyclePort
WakeupNotificationPort
RuntimeDeviceIdentity
ClientShellActions
```

Browser, PC, and Android implement those ports differently, but the core
algorithms stay shared.
`LocalMessageStore` includes a `clear` operation so logout can remove cached
messages, local cursors and pending-send records without making any local store
authoritative. Shared runtime logout must also disconnect push and clear the
platform secure session store.
`ClientShellActions` is the shared UI lifecycle contract for restore and logout;
browser, PC WebView and Android WebView shells should bind UI buttons to this
contract instead of duplicating platform-specific auth lifecycle code.

```text
browser  -> IndexedDB + fetch + WebSocket + browser lifecycle
PC       -> localStorage first-stage, then SQLite + Tauri HTTP/WebSocket + OS credential store + native lifecycle
Android  -> localStorage first-stage, then SQLite + platform HTTP/WebSocket + Android Keystore + app lifecycle
```

## First Client API Contract

The first BFF surface should be implemented by `api-gateway` as HTTP/JSON or
Connect-Web compatible endpoints. Exact transport can evolve, but the client
contract must remain stable.

```text
POST /api/auth/login
POST /api/auth/refresh
POST /api/auth/logout
GET  /api/me
GET  /api/conversations
GET  /api/conversations/{conversation_id}/messages?after_seq=&limit=
POST /api/messages/send
POST /api/delivery/ack
GET  /api/contacts
GET  /api/receipts
```

First-stage implementation note:

- `api-gateway` exposes these as HTTP/JSON client BFF endpoints while reusing
  the existing gateway facade and downstream trusted metadata injection.
- `GET /api/conversations/{conversation_id}/messages` is backed by
  `delivery-service.PullInbox`, not by a direct message table read.
- `/api/auth/logout` revokes only the current gateway-token session. The BFF
  derives tenant, user, device and session from the verified gateway token,
  forwards identity-service `RevokeSession`, and ignores caller-supplied target
  fields. Arbitrary device / session management remains an identity/admin
  capability, not a client BFF selector.
- HTTP-layer BFF metrics / rate-limit adapter is first-stage implemented. It
  reuses the api-gateway rate limiter and records fixed-route HTTP metrics into
  `/debug/metrics` and `/metrics`; labels do not include raw URL,
  conversation_id, tenant, user, token, request id or trace id.

`push-gateway` remains WebSocket-based:

```text
WS /ws

client.hello
server.hello
delivery.notify
server.resume_hint
delivery.ack
delivery.ack.ok
client.ping
server.pong
error
```

## Source of Truth

- Message order: `conversation_seq`.
- Durable delivery: `delivery-service.user_inbox` exposed through PullInbox.
- Per-device delivery cursor: `AckDelivery`.
- Online notification: `push-gateway` WebSocket, best-effort only.
- Local cache: UX cache and offline queue, not a server truth source.

## Sync Engine

The first sync engine is pull-after-notify:

```text
1. WebSocket receives delivery.notify.
2. InboxSyncEngine schedules PullInbox for that conversation.
3. PullInbox returns ordered items after local last_received_seq.
4. Local store merges items by conversation_id + conversation_seq.
5. AckQueue records the highest displayed / received seq.
6. AckQueue flushes AckDelivery.
```

On reconnect:

```text
1. Reconnect push-gateway.
2. Use local durable cursor / last_received_seq, not resume_hint seq, as the
   PullInbox start.
3. PullInbox all conversations with gaps or stale sync state.
4. Deduplicate by event_id and conversation_id + conversation_seq.
```

## Send Queue

All client writes use idempotency keys.

```text
1. User creates local pending message.
2. MessageSendQueue persists pending state.
3. SendMessage runs through api-gateway.
4. On success, local state records message_id and conversation_seq if returned.
5. Delivery notify / PullInbox remains the final display reconciliation path.
```

The UI can optimistically show pending messages, but PullInbox is the final
server-accepted view.

## Local Storage

First Web implementation uses IndexedDB through a `LocalMessageStore` port.
PC desktop and Android now use shared `KeyValueMessageStore` with WebView
`localStorage` as the first-stage durable cache. Production packaging should
replace only the storage port with SQLite/native adapters while keeping
`client-core` sync, send queue and ACK semantics shared. PC desktop and Android
`sqlite` configuration is reserved and must fail fast until a real native bridge
exists.

Minimum local entities:

```text
auth_session
conversation_summary
message_item
delivery_cursor
pending_send
pending_ack
sync_checkpoint
```

No refresh token or long-lived secret should be stored in plain localStorage.
The first LAN demo may use a development storage adapter, but production clients
need hardened token storage per platform.

## Security Boundaries

- The client must not store service-to-service credentials.
- The client must not call internal service ports.
- WebSocket auth must bind tenant, user, device, session, and token.
- API errors must map to stable public codes.
- Sensitive data should not appear in local logs or telemetry.
- PC desktop shell must restrict IPC and file-system access; Web code cannot get
  a broad native bridge.
- PC desktop Tauri command surface must stay single-command metadata-only until
  a dedicated native capability contract exists.
- PC Web code may display Tauri metadata for local diagnostics, but the metadata
  bridge must not become a storage, token, filesystem or message API.
- PC / Android WebView UI should use shared `ClientShellActions` for restore and
  logout so native shells do not grow separate auth lifecycle behavior.
- Android must use encrypted platform storage before production and must treat
  FCM/APNs-style push as wakeup only, never as delivered message truth.
- Android native WebView bridge must remain single-method read-only
  metadata-only until a dedicated native capability contract exists.

## MVP Acceptance

The first implementation is accepted when it can run on LAN and demonstrate:

```text
login
-> open one conversation
-> send TEXT message
-> receive delivery.notify
-> PullInbox returns the message
-> AckDelivery succeeds
-> reconnect triggers PullInbox fallback
```

This is not a full product client. It is the architectural base for Web,
PC desktop, and Android clients.

## Later Slices

- LAN smoke for the Web MVP path against `api-gateway` BFF and `push-gateway`.
- PC desktop shell with Tauri and Windows `.msi` / `.exe` packaging.
- Native SQLite store bridge and platform replay smoke for PC / Android.
- Android runtime implementation and unsigned local `.apk` packaging.
- Full group creation / group profile / invite / member-management UI.
- Media upload and preview after `media-service` provider path is ready.
- Client e2e smoke over local LAN and wired `172.x.x.x` network.
