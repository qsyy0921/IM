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
npm --prefix clients run test:shell-asset-prep-wrapper
npm --prefix clients run validate:desktop-tauri
npm --prefix clients run validate:android-native
```

Build target-specific Web assets for a local shell:

```powershell
npm --prefix clients run build:shell-assets:desktop
npm --prefix clients run build:shell-assets:android
```

The shell asset prep step writes `nexusim-shell-assets-manifest.json` next to the
prepared Web assets. The manifest contains only target, relative file paths,
byte sizes and SHA-256 hashes; it does not record local absolute paths.
`npm --prefix clients run verify:shell-assets` verifies prepared desktop /
Android assets against that manifest before a native artifact build. Use
`--target all` only after preparing both shell targets; per-target native build
wrappers run the matching verifier automatically.

Check artifact build commands without requiring the heavy native toolchains:

```powershell
npm --prefix clients run test:artifact-builders
npm --prefix clients run test:artifact-collector
npm --prefix clients run test:artifact-install-plan
npm --prefix clients run test:artifact-readiness
npm --prefix clients run test:shell-smoke-plan
npm --prefix clients run report:artifact-readiness
node clients/tools/plan-client-shell-smoke.mjs
node clients/tools/build-desktop-artifact.mjs --dry-run
node clients/tools/build-android-apk.mjs --dry-run
node clients/tools/collect-client-artifacts.mjs --target all --dry-run
npm --prefix clients run plan:artifact-install
```

Real artifact commands are present, but they fail fast until the local toolchain
is ready:

```powershell
npm --prefix clients run build:desktop-artifact
npm --prefix clients run build:android-apk
npm --prefix clients run build:desktop-artifact:collect
npm --prefix clients run build:android-apk:collect
npm --prefix clients run collect:client-artifacts
```

After preparing both shell targets, verify both prepared asset directories:

```powershell
node clients/tools/verify-shell-assets.mjs --target all
```

`collect:client-artifacts` copies produced desktop / Android artifacts into the
ignored `clients/artifacts/<run-id>/` directory and writes a low-sensitive
`manifest.json` with file names, sizes and SHA-256 hashes. It does not record
local absolute source paths. The `*:collect` build scripts run the same
collection step automatically after a successful native build.
`plan:artifact-install` reads that collected manifest and prints a low-sensitive
Windows / Android install checklist. It does not install packages, connect to
ADB, launch installers or print local absolute paths.

Android can also be built through the local Docker builder profile when the image
is intentionally built:

```powershell
npm --prefix clients run validate:builder-profile
docker compose -f deploy/local/docker-compose.client-builders.yml --profile client-builders build client-android-apk-builder
docker compose -f deploy/local/docker-compose.client-builders.yml --profile client-builders run --rm client-android-apk-builder
```

The Docker profile is not run by default and may download the Android / Node
toolchain the first time it is built. It runs `build:android-apk:collect`, so a
successful build writes the APK and `manifest.json` under
`clients/artifacts/android/docker-android-debug/` by default.

Current packaging status:

- Browser: Vite dev/build shell exists.
- PC desktop: Tauri shell skeleton can prepare target-specific Web assets and
  has a build wrapper. Direct Tauri builds still run shell asset prep, while the
  NexusIM wrapper sets `NEXUSIM_SKIP_SHELL_ASSET_PREP=true` after it has already
  prepared and verified the manifest so wrapper builds do not run the same Web
  build twice. No installer exists yet because this machine lacks Tauri CLI.
- Android: native WebView shell can prepare target-specific Web assets and has
  an APK build wrapper plus a Docker builder profile; no APK/AAB has been
  produced yet because the local native toolchain is missing and the Docker
  builder image has not been built in this slice.
- `report:artifact-readiness` prints the current low-sensitive readiness matrix
  for local desktop, local Android and Android Docker builder paths. It also
  emits `nextActions`, including the explicit Android builder image build command
  before the run command when the image is absent, and includes whether prepared
  shell assets currently verify against their manifest.
- `plan:shell-smoke` prints a low-sensitive browser / desktop / Android shell
  smoke plan. It lists prepared asset status, artifact presence, safe build
  commands, install-plan commands, per-target manual smoke checklists and the
  shared BFF / push smoke command without launching services or installing
  toolchains.
