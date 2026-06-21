# NexusIM Clients

This workspace contains the first client-platform slice for browser, PC desktop,
and Android.

## Scope

- Browser first.
- PC desktop and Android reuse the same `protocol` and `client-core` packages.
- The client talks to `api-gateway` and `push-gateway` only.
- `delivery-service` PullInbox is the reliable source of delivered messages.
- WebSocket `delivery.notify` frames are wakeups, not durable message facts.

## Languages

- Go remains the backend and client BFF language.
- TypeScript owns client product code: protocol contracts, sync core and UI.
- Rust is only for the Tauri shell and small OS bridge commands.
- Kotlin is only for Android platform adapters when TypeScript needs native
  access.
- Python is not part of the client runtime; it remains for AI workers, eval and
  offline tooling.

## Layout

```text
clients/
  packages/
    protocol/       typed client-facing API and WebSocket contracts
    client-core/    auth, push, sync, send queue, ack queue and store ports
  web/              first React + Vite shell
  desktop/          PC desktop shell contract, Tauri target
  android/          Android runtime and packaging contract
```

## Shared Core Contract

The three targets share:

- `@nexusim/protocol`: public API and WebSocket frame types.
- `@nexusim/client-core`: auth session, push connection, PullInbox sync,
  send queue, ACK queue and platform adapter ports.

Platform-specific code implements:

- secure session storage;
- local message store;
- network state;
- app lifecycle;
- wakeup notification bridge;
- runtime device identity.

## LAN Configuration

Use the wired `172.x.x.x` address when Windows and Mac communicate over the
direct cable.

```powershell
$env:VITE_NEXUSIM_API_BASE="http://172.16.10.1:8080"
$env:VITE_NEXUSIM_WS_URL="ws://172.16.10.1:8088/ws"
```

The exact ports must match the local `api-gateway` and `push-gateway` runtime.

## Validation

The current skeleton can be validated after installing the local workspace
dependencies:

```powershell
npm --prefix clients run validate
npm --prefix clients run test:shell-config
npm --prefix clients run test:shell-web-assets
npm --prefix clients run validate:desktop-tauri
npm --prefix clients run validate:android-native
```

Build target-specific Web assets for a local shell:

```powershell
npm --prefix clients run build:shell-assets:desktop
npm --prefix clients run build:shell-assets:android
```

Current packaging status:

- Browser: Vite dev/build shell exists.
- PC desktop: Tauri shell skeleton can prepare target-specific Web assets, no installer yet.
- Android: native WebView shell can prepare target-specific Web assets, no APK/AAB yet.
