# NexusIM Clients

This workspace contains the first client-platform slice for browser, PC desktop,
and Android.

## Scope

- Browser first.
- PC desktop and Android reuse the same `protocol` and `client-core` packages.
- The client talks to `api-gateway` and `push-gateway` only.
- `delivery-service` PullInbox is the reliable source of delivered messages.
- WebSocket `delivery.notify` frames are wakeups, not durable message facts.
- Browser mode ships a first-stage PWA install shell. Its service worker is not
  registered in PC desktop or Android WebView targets, and it never caches API,
  WebSocket or shell-config traffic.

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
- `LocalMessageStore`: shared cache/read model port for cursor, pending send,
  accepted-send migration and `listMessages`; Web uses IndexedDB while PC /
  Android first-stage shells use a key-value-backed implementation.

Platform-specific code implements:

- secure session storage;
- local message store;
- network state;
- app lifecycle;
- wakeup notification bridge;
- runtime device identity.

## Current Product Surface

The first client product surface is deliberately thin and service-backed:

- account/password register, login, refresh, restore and logout go through `api-gateway`
  BFF auth endpoints;
- group creation goes through the BFF `CreateConversation` path and creates the
  current user as the initial OWNER;
- message display uses PullInbox-backed conversation messages;
- sending text messages goes through the BFF `SendMessage` path;
- WebSocket push is only an online wakeup path;
- ACK uses the BFF `AckDelivery` path;
- contacts/friends now use contacts-service through api-gateway BFF, covering
  request send, accept/decline, cancel, list, remark, group, delete, block and
  unblock actions;
- direct friend messaging uses the BFF `/api/conversations/direct` path: the BFF
  verifies an ACTIVE contact edge through contacts-service, then creates or
  reuses a conversation-service `DIRECT` conversation before normal SendMessage /
  PullInbox / AckDelivery flow continues.
- group member add / leave, member list, member search, role filter,
  page-token pagination, member removal, ADMIN / MEMBER role change and owner
  transfer first paths use dedicated BFF endpoints backed by conversation-service
  `CreateMemberChange`, `ListConversationMembers` and `TransferConversationOwner`;
  the Web / PC shell never calls conversation-service private APIs directly.
- group profile summary and invite source hints are first-stage UI only. They
  render fields from `ConversationSummary` and the public member-list BFF
  response, without inventing editable title / avatar state.
- the Web / PC shell preserves user-facing local display titles produced by
  click-to-direct-chat and group creation. Unknown server summaries are shown as
  explicit short conversation IDs; the client does not treat those display names
  as server facts.
- empty states and common public errors are mapped to user-facing Chinese copy
  while preserving fail-closed behavior. Missing endpoints, expired tokens,
  missing login and permission errors do not trigger hidden alternate paths.

Full group settings are still first-stage. Remaining work is richer group title /
avatar read models and editing, plus real multi-user client smoke coverage for
member removal, role changes and owner transfer.

## LAN Configuration

For local Windows desktop debugging, start the backend and Web UI explicitly in
separate terminals:

```powershell
.\clients\start-local-backend.ps1
.\clients\start-local-web.ps1
```

The backend wrapper delegates to `loadtest/clientweb/run-local-dev.ps1` and keeps
the BFF and push listeners alive on:

```text
http://127.0.0.1:8080
ws://127.0.0.1:8088/ws
```

The seeded local login is:

```text
tenant_id: tenant-client-local
user_id: user-a
password: ClientWebReceiverPassw0rd!
conversation_id: conv-client-local
```

Use the wired `172.x.x.x` address only when another device or another machine
must reach the Windows host over the direct cable.

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
npm --prefix clients run check:no-toolchain
npm --prefix clients run test:web-pwa
npm --prefix clients run test:shell-web-assets
npm --prefix clients run test:shell-asset-prep-wrapper
npm --prefix clients run test:desktop-shell-action-assets
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
npm --prefix clients run test:desktop-bundle
npm --prefix clients run test:desktop-installer-builder
npm --prefix clients run test:desktop-installer-plan
npm --prefix clients run test:desktop-signing-executor
npm --prefix clients run test:desktop-signing-plan
npm --prefix clients run test:desktop-signing-readiness
npm --prefix clients run test:desktop-signature-verifier
npm --prefix clients run test:web-shell-actions
npm --prefix clients run test:shell-smoke-plan
npm --prefix clients run report:artifact-readiness
node clients/tools/plan-client-shell-smoke.mjs
node clients/tools/build-desktop-artifact.mjs --dry-run
node clients/tools/build-android-apk.mjs --dry-run
node clients/tools/collect-client-artifacts.mjs --target all --dry-run
npm --prefix clients run plan:artifact-install
```

Real artifact commands are present. Windows desktop is ready on this machine
after `npm --prefix clients install`; Android still fails fast until the Android
toolchain or Docker builder image exists:

```powershell
npm --prefix clients run build:desktop-artifact
npm --prefix clients run build:android-apk
npm --prefix clients run build:desktop-artifact:collect
npm --prefix clients run build:android-apk:collect
npm --prefix clients run collect:client-artifacts
npm --prefix clients run bundle:desktop:dry-run
npm --prefix clients run bundle:desktop
npm --prefix clients run build:desktop-installer
npm --prefix clients run plan:desktop-installer
npm --prefix clients run plan:desktop-signing
npm --prefix clients run report:desktop-signing-readiness
npm --prefix clients run sign:desktop-artifact
npm --prefix clients run verify:desktop-signature
```

After preparing both shell targets, verify both prepared asset directories:

```powershell
node clients/tools/verify-shell-assets.mjs --target all
```

`collect:client-artifacts` copies produced desktop / Android artifacts into the
ignored `clients/artifacts/<run-id>/` directory and writes a low-sensitive
`manifest.json` with file names, sizes, SHA-256 hashes and a low-sensitive
artifact kind such as `desktop-executable`, `desktop-installer` or
`android-debug-apk`. It does not record local absolute source paths. For Windows desktop artifacts it also writes
`README-windows-desktop.txt`; when the collected desktop artifact is a
standalone `.exe`, it writes `launch-nexusim-windows.ps1` that starts the exe
through a package-relative path. The `*:collect` build scripts run the same
collection step automatically after a successful native build.
`plan:artifact-install` reads that collected manifest and prints a low-sensitive
Windows desktop artifact / Android install checklist. It validates any collected
support files and, for standalone Windows desktop exe packages, points the
manual launch step at `launch-nexusim-windows.ps1`. Desktop installer artifacts
now stay on an install-oriented checklist and are not treated as portable
launchable packages. Stale manifests without an explicit `artifactKind` fail
closed and must be recollected. It also reports local install prerequisites such
as Android `adb` availability and Windows artifact launch support, but it does
not install packages, connect to devices, launch artifacts or print local
absolute paths.
`plan:shell-smoke` consumes the same install plan, so native shell smoke
readiness is not marked ready until a collected artifact exists and its
install-side prerequisites are available. For Windows, direct shell-smoke
readiness requires `desktop-executable`; `desktop-installer` requires the signed
installer path first. Its artifact status distinguishes raw native build-output
discovery from the collected artifact manifest used for manual install and
smoke.
`bundle:desktop` reads the collected Windows desktop manifest, requires the
package-local README and launcher support files, and writes a portable
`nexusim-windows-desktop-bundle.zip` plus `desktop-bundle-summary.json` under
ignored `clients/artifacts/desktop-bundles/<run-id>/`. This bundle is explicitly
`unsigned-local-dev`; it only accepts `desktop-executable` artifacts and does
not package installer artifacts, sign, install or launch anything. If the latest
collected manifest is for Android, pass the desktop manifest explicitly:
`npm --prefix clients run bundle:desktop -- --manifest clients/artifacts/<desktop-run>/manifest.json`.
`plan:desktop-signing` reads the latest collected `windows-desktop` manifest for
the requested `artifactKind` and reports whether explicit signing inputs are
present: `signtool`, one certificate source and a timestamp URL. It defaults to
`desktop-executable`; pass `--artifact-kind desktop-installer` only when signing
an installer artifact. It skips newer Android-only or wrong-kind manifests,
validates the selected artifact hash and prints a low-sensitive command template
only when ready. It does not sign artifacts, download tools, install packages,
launch the desktop app or print local absolute paths. Missing artifact kind or
signing inputs fail closed as `readyToSign=false`; there is no placeholder
signature path. Signing and installer wrappers also accept a local signing
profile through `--signing-profile <profile.json>` or
`NEXUSIM_DESKTOP_SIGNING_PROFILE`. The profile may reference a local PFX file or
Windows certificate-store thumbprint, `signtool`, timestamp URL and the name of
the PFX password environment variable. It must not contain certificate
passwords, private key material or provider credentials. The timestamp URL must
be plain `http` / `https` without embedded credentials, query string or
fragment. When a PFX file is used, the plan performs a read-only local PFX check
with the named environment variable and remains not ready unless the PFX can be
read, has a usable signing key and is not expired. When a Windows
certificate-store thumbprint is used, the plan performs a read-only local
certificate-store check and remains not ready unless the certificate exists, has
a usable signing key and is not expired. A profile may also declare an expected
public signer subject substring; read-only signature verification and installer
readiness remain blocked if a valid signature does not match it.
`sign:desktop-artifact` is the explicit execution wrapper for that plan. By
default it is also plan-only and prints a low-sensitive execution policy. It
only invokes `signtool` when run with `--execute` and when the collected desktop
artifact hash, explicit `signtool`, timestamp URL and certificate source are all
ready. It does not install artifacts, launch the app, start services or download
toolchains. Release signing can add `--require-valid`; in that mode the wrapper
reruns read-only Authenticode verification after signing and fails closed unless
the collected artifact is valid.
`verify:desktop-signature` is the read-only verification wrapper after signing.
It reads the collected desktop manifest for the requested `artifactKind`,
validates the selected artifact hash and reads Windows Authenticode public
status. It defaults to `desktop-executable`; use `--artifact-kind
desktop-installer` when verifying an installer. It does not sign, install,
launch, start services or download toolchains. Use `--require-valid` in a
release profile to fail closed when the selected artifact is not
Authenticode-valid. If an expected signer subject is configured, a merely valid
signature is not enough; the signer subject must match that public policy.
`report:desktop-signing-readiness` is the release-readiness summary over the
same controls. It combines the signing plan, plan-only signing execution output,
read-only executable Authenticode verification, MSI / NSIS installer plan and
post-build installer-artifact signature verification into one low-sensitive JSON
report. It does not sign, build installers, install, launch, start services,
start Docker or download toolchains. It may include low-sensitive local
`signtool` candidate hints, but those candidates are never used for readiness;
the selected path must still be provided through an explicit signing profile,
CLI argument or environment variable. If a collected `desktop-installer`
artifact is available, the report verifies that artifact separately; use
`--installer-manifest` when the executable baseline and installer artifact live
in different collected manifests. Unsigned / invalid artifacts stay blocked
until real signing input and valid signatures exist.
`plan:desktop-installer` reads the repository installer Tauri profile, the
collected Windows desktop manifest, the signing readiness plan and the
read-only signature verification report, then reports whether MSI / NSIS
installer bundling can run. It is also plan-only: it does not run Tauri, sign,
install, launch or download anything. The default development Tauri config
stays `bundle.active=false`; installer bundling uses the separate
`src-tauri/tauri.installer.conf.json` MSI + NSIS profile. The plan selects the
latest collected manifest containing a `desktop-executable` baseline
automatically; if a manifest contains both installer and executable artifacts it
still uses the executable baseline. Stale manifests without `artifactKind` or
installer-only artifacts fail closed. The plan remains not ready until signing
readiness is true and the executable baseline verifies as Authenticode-valid.
`build:desktop-installer` wraps that plan as the explicit execution entry. By
default it is still plan-only. It runs Tauri with the explicit bundle target and
then collects the resulting Windows desktop artifact only when run with
`--execute` and the installer plan is ready; otherwise it fails closed and
prints the missing readiness gates. It does not sign artifacts, install
installers, launch the app, start services or download toolchains.
Execution uses the repository installer Tauri profile through
`--config src-tauri/tauri.installer.conf.json`; custom `--tauri-config` is
accepted for planning fixtures only and blocks real `--execute`.

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
  build twice. The desktop workspace now declares `@tauri-apps/cli`; run
  `npm --prefix clients install` to download the local Tauri CLI, then run
  `build:desktop-artifact:collect`. The first-stage output is a standalone
  `nexusim-windows-desktop.exe` collected under ignored
  `clients/artifacts/<run-id>/` with a low-sensitive manifest,
  `README-windows-desktop.txt` and `launch-nexusim-windows.ps1`. A portable
  unsigned local zip bundle can be produced with `bundle:desktop`.
  `plan:desktop-signing` now checks explicit code-signing readiness and produces
  only a low-sensitive plan. `report:desktop-signing-readiness` now summarizes
  signing input readiness, read-only executable signature validity, installer
  build blockers and post-build `desktop-installer` signature validity without
  signing or building; any local `signtool` candidate remains a hint and must be
  copied into explicit signing config before use. `sign:desktop-artifact` is the explicit
  `--execute`-gated signing wrapper over that plan and fails closed until real
  signing inputs are present. `verify:desktop-signature` reads Authenticode
  public status. A new `desktop-executable` artifact was recollected at
  `clients/artifacts/2026-06-22T214826Z/manifest.json`; it verifies artifact kind
  and hash correctly, and its current Authenticode status is `NotSigned`.
  A 2026-06-23 read-only signing plan confirmed local Windows Kits `signtool`
  can be selected explicitly; after passing a timestamp URL, the remaining
  signing blocker is a real code-signing certificate source and a valid
  Authenticode signature.
  `plan:desktop-installer` now checks the repository
  installer Tauri profile, MSI / NSIS target, artifact baseline, signing
  readiness and valid signature status; actual `build:desktop-installer` now provides the explicit
  `--execute`-gated build entry and runs Tauri with that profile only when
  readiness is true. Real signed installer execution, install and launch remain
  future hardening. Use
  `npm --prefix clients run smoke:desktop-artifact-launch` for the first launch
  sanity check; it starts the collected exe, waits briefly, then terminates it.
- Android: native WebView shell can prepare target-specific Web assets and has
  an APK build wrapper plus a Docker builder profile. A first debug APK manifest
  exists from an earlier local build, but the current PowerShell environment
  still reports Java 8 and missing Gradle / `ANDROID_HOME` / `ANDROID_SDK_ROOT`;
  reload the F-drive toolchain environment or explicitly use the Docker builder
  before running the next Android build / WebView login smoke.
- `report:artifact-readiness` prints the current low-sensitive readiness matrix
  for local desktop, local Android and Android Docker builder paths. It also
  emits `nextActions`, including the explicit Android builder image build command
  before the run command when the image is absent, and includes whether prepared
  shell assets currently verify against their manifest.
- `plan:shell-smoke` prints a low-sensitive browser / desktop / Android shell
  smoke plan. It lists prepared asset status, artifact presence, safe build
  commands, install-plan commands, per-target manual smoke checklists and the
  shared lifecycle guard / BFF / push smoke command without launching services
  or installing toolchains. It also exposes `check:no-toolchain` as the default
  focused client gate before any APK, Docker or device-install path. The Android
  checklist starts with
  `report:android-platform-readiness`, so local JDK / Gradle / Android SDK,
  Docker builder image and ADB device state are visible before any APK or
  WebView smoke step.
- `plan:android-webview-login-smoke` prints the Android login-level WebView
  smoke contract and now includes a `safePreflight` block for
  `check:no-toolchain`, `report:android-platform-readiness` and
  `plan:artifact-install`. It remains a plan artifact only: it does not build
  an APK, launch Docker, install packages, start an Activity or open
  `adb reverse`.
- `check:no-toolchain` runs the fast client shell guard set without launching
  Docker, building APKs, installing APKs, starting Android activities, opening
  `adb reverse` or installing toolchains. It first validates its own dry-run plan
  for unsafe operations, then runs client workspace validation, workspace
  TypeScript, the clientweb smoke hook contract, shell config, Web platform,
  desktop / Android native skeleton validation, shared runtime /
  local-store / IndexedDB contracts, Web shell lifecycle / automation /
  smoke-report contracts, shell asset prep, desktop artifact launch / composed
  smoke dry-run contracts, artifact readiness / install-plan / builder /
  collector / desktop installer builder / installer readiness / signing plan /
  signing readiness report
  contracts, Android builder profile / wrapper contracts, desktop
  WebView metadata / login dry-run contracts, Android metadata / login dry-run
  contracts, Android device / WebView devtools readiness and parser contracts and
  reads low-sensitive ADB / device readiness state through the Android platform
  readiness report. Use it as the default focused client gate before reaching for
  a broader local gate.
