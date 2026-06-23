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
  PullInbox, AckDelivery, reconnect recovery.

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
- The same shell config now supports a first-stage metadata smoke callback:
  `smokeCallbackURL`, `smokeRunID` and `smokeMode=metadata`. The callback URL
  must be loopback `http://127.0.0.1`, `http://localhost` or `http://[::1]`.
  When enabled, the WebView posts a low-sensitive report after reading native
  bridge metadata. The report proves metadata wiring only; it does not submit
  login form data or run a message flow.
- Android login-level WebView smoke now has a low-sensitive plan entry
  `npm --prefix clients run plan:android-webview-login-smoke` and a real runner
  entry `npm --prefix clients run smoke:android-webview-login -- --fixture <clientweb-fixture.json>`.
  The plan includes a `safePreflight` block pointing at `check:no-toolchain`,
  `report:android-platform-readiness` and `plan:artifact-install` before any
  APK, Docker or device execution path. Dry-run tests cover the runner contract,
  including the `native-store-readiness` UI selector, without building an APK or
  touching a device; real execution still waits on a collected debuggable APK,
  ADB, WebView devtools and a clientweb fixture. The real runner now expects
  the WebView to display the current Android native local-store bridge as
  `android-sqlite` ready evidence; stale `sqlite-native-bridge-unavailable`
  evidence remains a failure. The plan now also exposes top-level
  `executionPolicy.planOnly=true`, explicitly stating that listed APK build,
  `adb install`, Activity start, `adb forward` and runner commands are not
  executed by the plan script.
- `loadtest/clientweb/run-local-smoke.ps1` can now opt into Android login-level
  WebView smoke with `-RunAndroidWebViewLoginSmoke`; the default path still runs
  only the shared Web/BFF/push smoke and does not build/install an Android app.
- `web/index.html` loads `nexusim-shell-config.js` before the app bundle.
  Browser mode uses the checked-in empty placeholder; desktop / Android shell
  builds can render their low-sensitive `shell-config.example.json` through
  `clients/tools/render-shell-config.mjs` and replace that placeholder for a
  local shell build.
- Browser mode now ships a first-stage PWA install boundary:
  `manifest.webmanifest`, `pwa-icon.svg` and `nexusim-sw.js`. Registration is
  skipped for `windows-desktop` and `android` WebView targets. The service
  worker only caches same-origin shell static assets and bypasses `/api/`,
  `/ws`, `nexusim-shell-config.js` and itself, so BFF responses, WebSocket data,
  auth/session material and target-specific shell config remain network-only.
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
- The Web / PC shell now has first-stage product interactions for clickable
  friend-to-direct-chat, clickable conversation selection, group creation,
  group member invite / leave, member list, member search, role filter,
  page-token pagination, member removal, ADMIN / MEMBER role changes and owner
  transfer through api-gateway BFF conversation contracts,
  selected-conversation preservation during refresh, local message-state refresh
  after send, and gateway-token-expired cleanup that clears the local session
  and asks the user to log in again. Group membership facts still come from
  conversation-service through BFF; the shell does not keep a fake member list.
  `loadtest/clientweb` now also verifies group profile read / update, group
  member list, role change, owner transfer, remove member and final member list
  through the same public BFF surface. The clean committed real smoke for that path passed on 2026-06-23
  with `commit=3b13c5c6` and `git_dirty=false`. The Web / PC shell now also
  exposes BFF-backed member search / role filter / page-token pagination,
  group profile summary, invite source hints and first-stage group title /
  avatar URI read-update. Group settings now use the caller's current
  conversation-member role from the public BFF member-list contract to enable
  OWNER / ADMIN management actions; unknown role state remains read-only.
  Conversation profile facts are owned by
  `conversation-service`; Web / PC shell only uses the api-gateway BFF. Remaining
  group product work is richer group settings, media-service-backed avatar upload
  and richer real multi-user UI smoke coverage.
- The Web / PC shell now also keeps explicit display-title and UX copy rules:
  direct / group titles learned from user actions survive conversation refresh,
  unknown server summaries render as short explicit conversation IDs, empty
  message states point users to the next action, and common public errors are
  mapped to user-facing Chinese copy without adding hidden fallback paths.
- `clients/start-local-backend.ps1` and `clients/start-local-web.ps1` are the
  current local startup pair. They start backend and frontend explicitly in
  separate terminals instead of hiding backend startup behind the Web launcher.
- `loadtest/clientweb` now verifies the two-user client first path through
  public client-facing surfaces: register/login, contact request send/accept,
  BFF direct-conversation open, direct message notify/PullInbox/ACK, BFF group
  creation, receiver JOIN, group message notify/PullInbox/ACK, and BFF group
  profile read/update plus member actions: active member list, role change,
  owner transfer, remove previous owner and final active member list. The 2026-06-23 clean run passed
  the direct + group message first path with `commit=6a08fb14` and
  `git_dirty=false`; the 2026-06-23 clean run passed the extended group member
  action path with `commit=3b13c5c6` and `git_dirty=false`.
- The visible Web / desktop shell now presents an account-password first IM
  surface: tenant, device, endpoint and native diagnostic controls are kept out
  of the normal user path, while smoke selectors remain stable. The
  2026-06-22 Windows Tauri WebView smoke proved this shell can still log in,
  receive `delivery.notify`, PullInbox and AckDelivery through public BFF /
  push paths.
- `clients/desktop` now has a first-stage TypeScript runtime adapter:
  `loadDesktopRuntimeConfig`, `createDesktopPlatformAdapter`, development-only
  session storage, localStorage-backed persistent message cache, static
  network/lifecycle ports and unsupported local wakeup notifications. This
  moves desktop beyond a pure contract, but it is not an installer yet.
- `clients/desktop/src-tauri` now has a first-stage Tauri v2 Rust runner
  skeleton with a read-only `runtime_metadata` IPC command plus fixed
  `local_store_get_item` / `local_store_set_item` / `local_store_remove_item`
  commands. Those local-store commands use an app-local-data SQLite key-value
  table and accept only `nexusim:client-message-store:v1:` keys, so the bridge
  cannot become a token store, filesystem bridge or arbitrary SQL surface.
  default `tauri.conf.json` keeps `bundle.active=false`, so normal native output
  is a standalone exe rather than an MSI / NSIS installer. Installer builds use
  the separate repository `tauri.installer.conf.json` profile. The Web shell can
  invoke this command set for diagnostics and local message cache persistence,
  and fails closed on malformed metadata.
  Its `frontendDist` resolves to the shared prepared `clients/web/dist`, not a
  desktop-local duplicate Web build.
- `clients/android` now has a first-stage TypeScript runtime adapter:
  `loadAndroidRuntimeConfig`, `createAndroidPlatformAdapter`, development-only
  session storage, localStorage-backed persistent message cache, static
  network/lifecycle ports and unsupported push/local wakeup notifications. This
  moves Android beyond a pure contract, and the first local debug APK baseline
  now builds from the native shell.
- `clients/android/native` now has a first-stage Kotlin native bridge skeleton.
  It now uses an Android WebView asset shell through `WebViewAssetLoader`,
  loads the prepared local Web assets, and still does not own token storage,
  local message facts, BFF calls, or push delivery semantics.
- Android WebView now registers a narrow `NexusIMNative` JavaScript bridge.
  It exposes `runtimeMetadata()` plus fixed local-store key-value methods. The
  Web runtime can parse metadata for diagnostics, including low-sensitive
  local-store bridge readiness, and invalid bridge payloads fail closed. The
  local-store methods are limited to the shared client message cache prefix and
  expose no token, file-system, content-provider or arbitrary message API.
- Android WebView inspection is explicitly gated by the platform debuggable
  flag. This keeps dev / smoke automation possible while avoiding an
  unconditional release debugging path.
- The Web shell runtime panel now displays shell target plus PC Tauri or Android
  native bridge metadata when present. When metadata includes local-store
  capability readiness, the panel shows the low-sensitive current store, target
  store, bridge and unavailable reason. This is diagnostics only; it does not
  grant Web code native capabilities.
- Desktop and Android login-level WebView smoke runners now require the same
  `native-store-readiness` selector and record low-sensitive readiness evidence
  when the real smoke can run.
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
  first-stage WebView `localStorage` wrappers by default, but both source trees
  now have native SQLite key-value bridge implementations. Desktop exposes
  fixed Tauri `local_store_*` commands backed by app-local-data SQLite; Android
  exposes fixed-prefix `NexusIMNative.localStore*` methods. The readiness
  contract emits stable low-sensitive `reason`, expected bridge and next action
  fields, so tools and runtime adapters do not need target-specific error
  strings. Web shell adapter wiring accepts ready native key-value bridges and
  routes them through shared `KeyValueMessageStore`; desktop now has fresh
  WebView `tauri-sqlite` ready evidence, while Android still needs APK build
  plus real-device WebView smoke before that runtime path becomes a packaged
  baseline.
  `LocalMessageStore.clear` is now part of the shared port so logout can remove
  cached messages, cursors and pending sends consistently across targets.
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
- `npm --prefix clients run test:web-pwa` validates the Browser PWA contract:
  manifest, icon, service worker registration, WebView target skip and
  cache-bypass rules for API / WebSocket / shell config paths.
- `npm --prefix clients run check:no-toolchain` first validates its own dry-run
  plan for unsafe operations, then runs the no-toolchain client shell guard set
  in one command: client workspace validation, shell smoke plan, build
  prerequisite report contract, workspace TypeScript, Web PWA, shell web assets
  / prep wrapper, shell config,
  desktop / Android native skeleton validation, Web platform, shared runtime /
  local-store / IndexedDB contracts, Web shell lifecycle / automation /
  smoke-report contracts, clientweb smoke hook contract, desktop artifact launch
  / composed smoke dry-run contracts, artifact readiness / install-plan / builder
  / collector contracts, desktop installer builder / installer readiness /
  signing profile / signing readiness plan / signing readiness report contracts,
  Android builder profile / wrapper contracts, desktop / Android action assets, desktop WebView
  metadata / login dry-run contracts,
  Android metadata / login smoke dry-run contracts, Android device / WebView
  devtools readiness and parser contracts and Android platform readiness. It does not build native
  artifacts or APKs, launch Docker, install APKs, start activities, open
  `adb reverse` or install toolchains; it does read low-sensitive ADB / device readiness state through
  `report:android-platform-readiness`. Its `--dry-run` output now carries a
  plan-only execution policy proving that the dry-run itself only describes the
  focused gate and does not execute checks, run npm scripts or read device
  state. Run the individual scripts only when a specific guard fails.
- `npm --prefix clients run test:web-shell-actions` guards the Web shell
  lifecycle contract. It verifies the Web shell binds login, refresh, restore
  and logout through shared `ClientShellActions` and does not call runtime auth
  lifecycle methods directly, so PC / Android WebView shells can keep the same
  UI action path.
- `npm --prefix clients run test:desktop-shell-action-assets` builds the Web
  shell into a temporary directory, prepares temporary Windows desktop WebView
  assets with the desktop shell config, verifies the shell asset manifest, and
  checks that the packaged desktop bundle contains the login / refresh /
  restore / logout selectors plus `native-store-readiness` and PWA static
  assets. It requires Node/Vite only; it does not require Tauri CLI, an
  installer or a live backend.
- `npm --prefix clients run test:android-shell-action-assets` builds the Web
  shell into a temporary directory, prepares temporary Android WebView assets
  with the Android shell config, verifies the shell asset manifest, and checks
  that the packaged Android bundle contains the login / refresh / restore /
  logout selectors plus `native-store-readiness`. It requires Node/Vite only;
  it does not require Gradle, Android SDK, ADB, an APK or a device.
- `npm --prefix clients run test:shell-config` validates the desktop / Android
  shell config templates and renderer, and rejects unsupported targets or
  sensitive fields such as token, secret and password.
- `npm --prefix clients run test:shell-web-assets` validates the target asset
  prep wrapper without requiring Tauri CLI, Android SDK or a live backend. It
  also checks stale bundle cleanup so Android / desktop shell outputs do not
  retain old Web assets across builds, and verifies the low-sensitive
  `nexusim-shell-assets-manifest.json` with relative paths, byte sizes and
  SHA-256 hashes. The focused fixture includes the Browser PWA manifest,
  service worker and icon so target shell asset manifests cannot silently drop
  those files. Native artifact wrappers call the same manifest verifier after
  preparing assets and before invoking Tauri / Gradle.
- `npm --prefix clients run test:artifact-builders` validates the first-stage
  desktop artifact / Android APK build wrappers in dry-run mode. Windows
  desktop can now build through the repo-local Tauri CLI after
  `npm --prefix clients install`; Android now also builds locally when the
  F-drive JDK / Gradle / Android SDK environment is present. Both desktop and
  Android wrappers accept a custom shell config path for metadata-smoke builds;
  the path is never printed in dry-run output.
  Both wrapper dry-runs now emit an execution policy proving they do not execute
  Tauri / Gradle builds, prepare or verify shell assets, collect artifacts,
  start Docker, install artifacts, contact devices or download toolchains.
- `npm --prefix clients run test:android-docker-builder` validates the safe
  Android Docker builder wrapper. `build:android-apk:docker` runs only when
  the local builder image already exists; `build:android-apk:docker:bootstrap`
  is the explicit opt-in command that may download Node / Android SDK toolchains
  to build the image.
- `npm --prefix clients run test:artifact-collector` validates the first-stage
  artifact collector. Once a real desktop artifact or Android APK exists,
  `npm --prefix clients run collect:client-artifacts` copies it into ignored
  `clients/artifacts/<run-id>/` storage and writes a low-sensitive manifest with
  file names, sizes, SHA-256 hashes and low-sensitive artifact kind
  (`desktop-executable`, `desktop-installer` or `android-debug-apk`), without
  recording local absolute source paths. Windows desktop collection also writes `README-windows-desktop.txt`;
  standalone `.exe` packages additionally get `launch-nexusim-windows.ps1`
  with package-relative launch logic. `build:desktop-artifact:collect` and
  `build:android-apk:collect` run the collector automatically after a successful
  native build. The collector
  `--dry-run` output carries an execution policy proving it only discovers
  candidate sources and reads metadata; it does not copy artifacts, create
  output directories, write manifests, install artifacts, contact devices or
  download toolchains.
- `npm --prefix clients run test:artifact-install-plan` validates the
  first-stage install-plan tool. `npm --prefix clients run plan:artifact-install`
  reads a collected `clients/artifacts/<run-id>/manifest.json` and prints
  low-sensitive Windows desktop artifact / Android APK install checklist commands.
  It validates collected support files and, for standalone Windows `.exe`
  packages, points the manual launch command at `launch-nexusim-windows.ps1`.
  Desktop installer artifacts must be requested with
  `--artifact-kind desktop-installer`; the default Windows path stays
  `desktop-executable`, so mixed manifests do not accidentally enter the
  installer path. Installer artifacts stay on an install-oriented checklist and
  are not treated as portable launchable packages; stale manifests without
  explicit `artifactKind` fail closed and must be recollected. Installer install
  readiness also requires read-only Authenticode verification to report a valid
  signed installer; unsigned or unverifiable installer artifacts fail closed.
  It now also reports install-side readiness such as Android `adb` availability
  and Windows local artifact launch support, while still not launching
  artifacts, contacting devices, installing packages or printing local absolute
  paths.
- `npm --prefix clients run test:desktop-bundle` validates the first-stage
  portable Windows desktop bundle tool. `npm --prefix clients run bundle:desktop`
  reads a collected Windows desktop artifact manifest, requires the package-local
  README / launcher support files for standalone exe packages, accepts only
  `desktop-executable` artifacts, writes
  `nexusim-windows-desktop-bundle.zip` and `desktop-bundle-summary.json` under
  ignored `clients/artifacts/desktop-bundles/<run-id>/`, and marks the result
  as `unsigned-local-dev`. It does not package installer artifacts, sign,
  install, launch, start services, contact devices or download toolchains. If
  the latest collected manifest is Android-only, pass
  `--manifest clients/artifacts/<desktop-run>/manifest.json`
  explicitly.
- `npm --prefix clients run report:android-device-readiness` prints a
  low-sensitive ADB/device readiness report for Android shell smoke. It runs
  `adb devices -l`, hashes raw serials, omits model names, and reports whether
  an authorized device is visible. It does not install APKs, start activities,
  or contact network services. Its execution policy marks the report as an
  actual local readiness probe that only reads the ADB device list and never
  downloads toolchains, builds artifacts, installs packages, starts activities,
  opens adb reverse / forward or exposes raw device identifiers.
- `npm --prefix clients run report:android-platform-readiness` prints a
  combined low-sensitive Android readiness report across local JDK / Gradle /
  Android SDK, Docker builder profile / image and ADB device state. It is the
  preferred preflight before deciding whether to spend bandwidth on
  `build:android-apk:docker:bootstrap`; it does not download, build, install or
  expose raw serials / model names / local absolute paths. Its execution policy
  declares that it reads local toolchain state, Docker builder state and ADB
  device readiness only; it does not start Docker, build a Docker image, build
  an APK, install, launch activities or open adb tunnels.
- `npm --prefix clients run report:artifact-readiness` prints a low-sensitive
  readiness matrix for local desktop, local Android and Android Docker builder
  paths. It reports missing capabilities and the exact next build command
  without printing local absolute paths. It also includes per-target prepared
  shell asset verification status and per-target local store readiness. The
  local store section records the current first-stage `local-storage` cache,
  target `sqlite` production store and native bridge readiness. Desktop source
  now reports `tauri-sqlite` ready, and the 2026-06-22 desktop WebView metadata
  / login rerun captured that updated runtime evidence. Android source now
  reports `android-sqlite` ready, but APK build and real-device smoke are still
  required before treating it as a runtime baseline. It separates the Android Docker builder image
  build command from the actual builder run command and emits low-sensitive
  `nextActions`; it never starts a download or build by itself. When the image
  is missing, the next action points at `build:android-apk:docker:bootstrap`,
  making the toolchain download explicit instead of accidental. Its execution
  policy declares it as report-only: it may read local toolchain state, Docker
  builder state, shell asset manifests and native-store source readiness, but it
  does not build native artifacts, prepare or collect shell assets, write
  artifact manifests, start services / Docker, build Docker images, install
  artifacts, contact devices or download toolchains.
- `npm --prefix clients run plan:shell-smoke` prints a low-sensitive browser /
  desktop / Android shell smoke plan. It combines toolchain readiness, prepared
  asset verification, artifact presence, collected-artifact install readiness,
  safe build commands, the shared BFF / push smoke command and the default
  `check:no-toolchain` focused gate; it does not launch services, connect
  devices, install artifacts or install toolchains. Native artifact status
  distinguishes raw build-output discovery from the collected artifact manifest
  that drives manual install readiness. When a Windows desktop artifact is
  collected, the plan includes `smoke:desktop-artifact-launch` as a process
  launch sanity check before the fuller login-level shell smoke.
  Browser plans include `test:web-pwa` before manual shell smoke, so the PWA
  manifest and service-worker cache boundary are verified before browser
  install testing. Desktop plans include `test:desktop-shell-action-assets`
  before Tauri build, and Android plans include `test:android-shell-action-assets`
  before any APK build step, so shared lifecycle selectors are verified in target
  WebView assets without native toolchains. Android plans also include
  `report:android-platform-readiness` before APK / WebView smoke, so local
  toolchain, Docker builder image and ADB state are visible in one low-sensitive
  report before any download-heavy or device-touching step.
  `smoke:desktop-composed` can combine an existing `loadtest/clientweb`
  BFF/push summary with desktop artifact launch evidence into one low-sensitive
  JSON result. It is useful as an intermediate PC evidence bundle, but it is not
  GUI automation and does not prove login inside the Tauri WebView.
  For Windows desktop, the plan now includes the explicit
  `install-declared-desktop-tauri-cli` step when the repo-declared local Tauri
  CLI has not been installed.
- `npm --prefix clients run smoke:android-webview-metadata` is the first Android
  WebView metadata smoke runner. In dry-run mode it emits only low-sensitive
  package / Activity / adb-reverse intent plus an execution policy proving the
  dry-run does not build an APK, install, start an Activity, open `adb reverse`
  or contact a device. In real mode it builds an APK with a temporary loopback
  metadata shell config, installs it through `adb`, starts
  `com.nexusim.android/.MainActivity`, and waits for the WebView to POST the
  `NexusIMNative.runtimeMetadata()` report through `adb reverse`. It proves only
  appassets + metadata bridge wiring, not login, PullInbox, WebSocket, or ACK.
  This smoke intentionally requires a fresh APK build because the callback URL
  is injected into shell assets before packaging; a previously collected normal
  APK cannot prove the callback path unless it was built for the same callback.
- `npm --prefix clients run smoke:android-webview-login -- --dry-run` emits a
  low-sensitive login-level Android WebView smoke plan with an execution policy
  proving the dry-run does not build or collect an APK, install, start an
  Activity, open adb forward, drive WebView automation, contact the BFF or send
  messages. The real runner still requires a built APK, ADB install / Activity
  launch, WebView CDP over adb forward, public BFF login / SendMessage and
  AckDelivery observation. `report:android-webview-devtools-readiness` is a
  separate report-only preflight for that CDP step: it can read fixture data or
  query `/proc/net/unix` through adb to detect WebView devtools sockets, but its
  execution policy forbids opening adb forward / reverse, installing APKs,
  starting activities, downloading toolchains or exposing raw socket names.
- `npm --prefix clients run validate:builder-profile` validates the Android
  Docker builder profile without building or pulling images. The profile lives
  in `deploy/local/docker-compose.client-builders.yml` and uses
  `deploy/docker/client-android-builder.Dockerfile`; it is opt-in because the
  first image build downloads Node, Gradle and Android SDK components. The
  image build context is intentionally limited to `deploy/docker`; the repository
  is mounted only at container runtime under `/workspace`. The profile now runs
  `build:android-apk:collect`, so a successful container build writes the APK and low-sensitive `manifest.json` under
  `clients/artifacts/android/docker-android-debug/` by default.
- `@nexusim/client-core` now exposes `ClientShellActions`; desktop and Android
  export thin `createDesktopShellActions` / `createAndroidShellActions` wrappers.
  These wrappers do not own business logic; they only bind shell UI actions to
  the shared runtime lifecycle. The Web shell login panel now uses the shared
  shell action contract for login, refresh, restore and logout, matching the
  desktop / Android thin shell action coverage. PC / Android WebView shells do
  not need a separate UI lifecycle path. The Android temporary asset contract
  test now verifies that those selectors survive the Web build and Android
  asset preparation path before any APK is built.
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
  WebView asset shell skeleton. PC exposes read-only runtime metadata IPC and
  fixed SQLite-backed local-store key-value commands; Android exposes metadata
  plus fixed-prefix SQLite local-store methods. Both targets keep native storage
  behind shared readiness and do not expose broad filesystem, token or message
  APIs. Windows desktop now
  produces a first-stage standalone `.exe` artifact, low-sensitive collected
  manifest and package-local README / launcher support files;
  `bundle:desktop` can package those collected files into an unsigned local
  portable zip with a low-sensitive summary;
  `plan:desktop-signing` can read the collected desktop manifest for the
  requested `artifactKind`, verify the selected artifact hash and report whether
  explicit `signtool`, certificate and timestamp URL inputs are present. The
  signing / installer wrappers now also accept `--signing-profile` or
  `NEXUSIM_DESKTOP_SIGNING_PROFILE` as a local-only profile contract for those
  inputs. The profile references a local PFX file or Windows certificate-store
  thumbprint plus a timestamp URL and PFX password environment variable name; it
  must not contain certificate passwords, private key material or provider
  credentials. The timestamp URL must be plain `http` / `https` without
  embedded credentials, query string or fragment. When a PFX file is used, the
  plan performs a read-only local PFX check with the named environment variable
  and remains not ready unless the PFX can be read, has a usable signing key and
  is not expired. When a Windows certificate-store thumbprint is used, the plan
  performs a read-only local certificate-store check and remains not ready unless
  the certificate exists, has a usable signing key and is not expired. A profile
  may also declare an expected public signer subject substring; read-only
  signature verification and installer readiness remain blocked if a valid
  signature does not match it. It
  defaults to
  `desktop-executable`; `desktop-installer` must be requested
  explicitly. It is plan-only: it does not sign, download tools, install
  packages, launch the desktop app or print local absolute paths. Missing kind
  or signing inputs remain fail-closed as `readyToSign=false`.
  `report:desktop-signing-readiness` combines that signing plan, plan-only
  signing execution state, read-only executable Authenticode verification,
  MSI / NSIS installer blockers and post-build `desktop-installer` signature
  verification into one low-sensitive release-readiness JSON report. It does
  not sign, build installers, install, launch, start services, start Docker or
  download toolchains. It may include low-sensitive local `signtool` candidate
  hints, but those hints are never used for readiness; the selected path must
  still be copied into explicit signing config before use. If the executable
  baseline and collected installer artifact are in different manifests, pass
  `--installer-manifest` for the installer signature check. Unsigned or invalid
  artifacts remain blocked until real signatures verify.
  `sign:desktop-artifact` is the explicit execution wrapper for that plan. Its
  default output remains plan-only and low-sensitive; it invokes `signtool` only
  with `--execute` after the collected artifact hash, explicit `signtool`,
  timestamp URL and certificate source are ready. It uses the same
  `--artifact-kind` selector as the signing plan, so installer signing must pass
  `--artifact-kind desktop-installer` explicitly instead of relying on the
  default executable path. It does not install artifacts, launch the app, start
  services or download toolchains. Release signing may add `--require-valid`;
  then the wrapper reruns read-only Authenticode verification after signing and
  fails closed if the artifact is still not valid.
  `verify:desktop-signature` is the read-only post-signing verification wrapper.
  It validates the selected artifact hash and reads Windows Authenticode public
  status without signing, installing, launching, starting services or downloading
  toolchains. It also defaults to `desktop-executable`; installer verification
  must pass `--artifact-kind desktop-installer`. If an expected signer subject
  is configured, a merely valid Authenticode signature is not enough; the signer
  subject must match that public release policy. A new `desktop-executable`
  artifact was recollected on 2026-06-23 at
  `clients/artifacts/2026-06-22T214826Z/manifest.json`; it now passes artifact
  kind and hash selection, and read-only Authenticode verification reports
  `NotSigned`. A 2026-06-23 read-only signing plan confirmed that local Windows
  Kits `signtool` can be selected explicitly; after passing a timestamp URL, the
  remaining release blocker is a real code-signing certificate source plus a
  valid Authenticode signature. Release profiles must provide real signing
  inputs, sign first and then rerun verification with `--require-valid`.
  `plan:desktop-signing`,
  `verify:desktop-signature` and `plan:desktop-installer` now select artifacts by
  `artifactKind` instead of blindly using the first `windows-desktop` artifact,
  so newer Android artifacts and mixed desktop manifests do not hide or swap the
  intended executable baseline.
  `plan:desktop-installer` can read the repository installer Tauri profile, the
  collected desktop manifest, signing readiness and signature verification
  status, then report whether MSI / NSIS installer bundling is ready. The
  default dev config remains inactive, while `tauri.installer.conf.json` is the
  explicit MSI + NSIS profile. The plan still reports not ready until signing
  readiness and a valid Authenticode signature are present on an explicit
  `desktop-executable` baseline instead of treating the unsigned portable zip,
  a stale no-kind manifest, or an already collected installer as an installer
  build input.
  The generic client install plan now also keeps collected `desktop-installer`
  artifacts out of the portable launcher path, so `plan:shell-smoke` only marks
  Windows direct shell smoke ready for `desktop-executable` artifacts. It also
  keeps installer install readiness blocked until the collected installer
  verifies as Authenticode-valid, and `plan:shell-smoke` now carries that
  installer signature summary forward so shell-smoke preflight can show whether
  the blocker is unsigned / unverifiable installer state.
  `build:desktop-installer` is the explicit execution wrapper over that plan.
  Its default output is plan-only; `--execute` is required before it runs Tauri
  with the explicit bundle target and installer profile, then collects the
  resulting Windows desktop artifact. It still fails closed while installer
  readiness, signing input readiness or executable signature validity is false.
  The release-readiness report then verifies any collected `desktop-installer`
  artifact separately before distribution. It does not sign, install, launch,
  start services or download toolchains. Real execution uses the repository
  installer profile; custom `--tauri-config`
  remains a planning fixture input and blocks `--execute`.
  `smoke:desktop-artifact-launch` has verified the exe starts, stays alive
  during the smoke hold window and terminates cleanly.
  Its dry-run output carries an execution policy proving it only reads the
  collected manifest and artifact bytes for hash validation; it does not start
  or terminate the artifact process, open network connections, install
  artifacts, contact devices or download toolchains.
  `smoke:desktop-composed` can also combine that launch proof with a clientweb
  BFF / push summary without leaking absolute paths or sensitive fields. It
  now emits an execution policy: with `--clientweb-summary + --launch-dry-run`
  it only reads the existing clientweb summary, validates the manifest /
  artifact bytes through the nested launch dry-run, and does not start services,
  launch the desktop artifact, open network connections, start Docker, install
  or contact devices, or download toolchains. It still does not produce MSI /
  NSIS installer bundles. `npm --prefix clients run
  smoke:desktop-webview-metadata` now proves the Tauri WebView can load the
  prepared shell, read the PC `runtime_metadata` IPC and POST a low-sensitive
  loopback report from inside the rendered shell. The report includes the
  native local-store readiness diagnostic and the fixed storage command surface.
  Its dry-run output carries an execution policy proving it does not build or
  launch the desktop artifact, start callback / WebView automation, open network
  connections or download toolchains. The login-level desktop WebView smoke
  dry-run carries the same plan-only boundary, plus explicit markers that it
  does not contact the BFF or send messages.
  The fuller login-level
  desktop UI smoke has also passed on clean commit `c72ea512`, covering WebView
  login, externally triggered `delivery.notify`, PullInbox, message observe and
  AckDelivery. The desktop SQLite bridge rerun on commit `2b67b0e1` also proves
  `tauri-sqlite` ready evidence in both metadata-only and login-level WebView
  smoke. The 2026-06-22 account-password shell UI rerun passed the same
  login-level Windows WebView path and is recorded in
  `docs/runbook/loadtest/client-platform/loadtest-report-20260622-client-web-desktop-login-ui.md`.
  Android now has the same metadata-smoke runner shape, a collected debug APK
  artifact, and an installed-device metadata smoke pass after Android asset URL
  rewriting. Real Android login WebView smoke is deferred while the active
  priority stays on browser / Windows PC.
- `plan:shell-smoke` now marks the Android Docker builder path with
  machine-readable risk flags. If the local builder image is missing, the
  `build-android-builder-image` step carries `downloadsToolchain=true` and
  `requiresExplicitUserOptIn=true`, plus a safe dry-run command. This keeps the
  shell plan useful while preventing Codex or automation from treating image
  bootstrap as part of the no-toolchain client gate. The plan also exposes
  top-level `executionPolicy.planOnly=true` so automation can reject accidental
  service-start, Docker, install or device-touching paths before reading
  per-target checklists.
- `plan:artifact-install` now marks install / launch checklist entries as
  `manualOnly=true` with explicit device, install, Activity-start or desktop
  process risk flags. The plan can show `adb install` and `Start-Process`
  commands for humans, but the script does not install APKs, launch desktop
  artifacts, start Android activities or contact devices by itself. For
  `desktop-installer` artifacts it also reads only public Authenticode status
  and refuses `readyForInstall` until the installer is validly signed. The plan
  also exposes a top-level `executionPolicy.planOnly=true` block so automation
  can reject accidental execution paths before reading per-step checklist data.
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

1. Continue browser / Windows PC first: polish the account-password IM shell,
   keep complex endpoint / tenant / device controls hidden, and preserve the
   public BFF / push client path for login, contacts, direct chat, group chat,
   permission-aware group settings, group profile editing, send, PullInbox and
   ACK.
2. Produce the next Windows package step when needed: MSI / NSIS installer
   script and real code-signing pipeline on top of the existing standalone exe,
   package-local launcher, unsigned local zip bundle, explicit installer /
   signing readiness plans and login-level WebView smoke.
3. Return to Android only when explicitly prioritized: run login-level Android
   WebView smoke on the installed APK, then record the Android baseline.

## Local Build Prerequisites

- PC artifact build needs Tauri CLI / `cargo-tauri`; the current repository has
  the runner skeleton, validator and repo-declared `@tauri-apps/cli`
  dependency. `npm --prefix clients install` installs the repo-local Tauri CLI;
  `build:desktop-artifact:collect` then produces a first-stage standalone exe
  plus collected manifest, README and launcher support files.
- Android APK build needs JDK 17+ plus Gradle / Android SDK. The Windows local
  baseline now uses `F:\IM\toolchains`: Temurin JDK 17, Gradle 8.10.2,
  Android SDK commandline-tools, platform-tools, `platforms;android-35` and
  `build-tools;35.0.0`. `GRADLE_USER_HOME` is also pinned under `F:\IM` so
  Gradle dependency caches do not default to C drive.
- `npm --prefix clients run build:android-apk:collect` produced the first
  collected Android debug APK manifest at
  `clients/artifacts/2026-06-22T034017Z/manifest.json` with
  `nexusim-android-debug.apk`
  (`sha256=f931053736f0e4168417b1187fe2c6058b86dc8db8dbca7a1e9b1fec1a901dba`).
- The repository still includes an Android Docker builder profile wired to the
  same artifact collector, but it is no longer the default next step for this
  Windows host. Use it only when containerized Android builds are explicitly
  requested; the first image build downloads Node, Gradle and Android SDK
  components.

Focused local check:

```powershell
npm --prefix clients run check:build-prereqs
npm --prefix clients run test:shell-config
npm --prefix clients run check:no-toolchain
npm --prefix clients run test:web-pwa
npm --prefix clients run test:shell-web-assets
npm --prefix clients run test:desktop-shell-action-assets
npm --prefix clients run test:android-shell-action-assets
npm --prefix clients run test:artifact-builders
npm --prefix clients run test:artifact-collector
npm --prefix clients run test:artifact-install-plan
npm --prefix clients run test:artifact-readiness
npm --prefix clients run test:desktop-bundle
npm --prefix clients run test:desktop-installer-builder
npm --prefix clients run test:desktop-installer-plan
npm --prefix clients run test:desktop-signing-profile
npm --prefix clients run test:desktop-signing-plan
npm --prefix clients run test:desktop-signing-readiness
npm --prefix clients run test:android-docker-builder
npm --prefix clients run test:native-store-readiness
npm --prefix clients run test:android-webview-metadata-smoke
npm --prefix clients run test:android-device-readiness
npm --prefix clients run test:desktop-artifact-launch-smoke
npm --prefix clients run test:shell-smoke-plan
npm --prefix clients run report:artifact-readiness
npm --prefix clients run validate:builder-profile
```

This command reports readiness as JSON and exits non-zero when artifact / APK
toolchains are missing. It is local-only: it does not install dependencies, pull
packages, or use `npx` to resolve remote CLIs. Its output is now
`nexusim.client-build-prereqs.v1` with an execution policy: it may run local
toolchain/version probes, read environment variables and inspect repo-local
Node bins, but it does not build artifacts, start services / Docker, install or
contact devices, download toolchains, or print raw command output / local
absolute paths.

The readiness report is non-failing and is useful before deciding whether to
install native toolchains or run the Docker builder. It also reports whether
current prepared shell assets verify against `nexusim-shell-assets-manifest.json`:

```powershell
npm --prefix clients run report:artifact-readiness
npm --prefix clients run report:android-platform-readiness
npm --prefix clients run report:android-device-readiness
npm --prefix clients run plan:shell-smoke
npm --prefix clients run plan:artifact-install
npm --prefix clients run bundle:desktop:dry-run
npm --prefix clients run bundle:desktop
npm --prefix clients run build:desktop-installer
npm --prefix clients run plan:desktop-installer
npm --prefix clients run plan:desktop-signing
npm --prefix clients run report:desktop-signing-readiness
npm --prefix clients run sign:desktop-artifact
npm --prefix clients run verify:desktop-signature
npm --prefix clients run smoke:desktop-artifact-launch
npm --prefix clients run smoke:desktop-composed -- --clientweb-summary <client-web-summary.json>
npm --prefix clients run smoke:desktop-webview-metadata
npm --prefix clients run smoke:android-webview-metadata -- --dry-run
```

The report includes `nextActions`. When the Android Docker builder image is
missing, the first Android next action is the explicit bootstrap command. After
the image exists, the next action becomes the safe builder run command that
writes the APK and manifest. The Android Docker builder dry-run also emits an
execution policy: dry-run only reads Docker builder state; it does not start
Docker, build images / APKs, write manifests, install artifacts or contact
devices. Bootstrap dry-run may report `plannedDownloadsToolchain=true`, but
actual downloads only happen through the explicit bootstrap command.

Artifact wrappers:

```powershell
node clients/tools/build-desktop-artifact.mjs --dry-run
node clients/tools/build-android-apk.mjs --dry-run
node clients/tools/collect-client-artifacts.mjs --target all --dry-run
npm --prefix clients run build:desktop-artifact
npm --prefix clients run build:android-apk
npm --prefix clients run build:desktop-artifact:collect
npm --prefix clients run build:android-apk:collect
npm --prefix clients run build:android-apk:docker
npm --prefix clients run build:android-apk:docker:bootstrap
npm --prefix clients run build:android-apk:docker:image
npm --prefix clients run collect:client-artifacts
```

After preparing both shell targets, `node clients/tools/verify-shell-assets.mjs --target all`
checks both prepared asset directories. Per-target native build wrappers run the
matching verifier automatically after asset prep.
