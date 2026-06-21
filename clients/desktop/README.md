# NexusIM Desktop Client

Desktop target for Windows first, with macOS later.

The desktop client uses the same Web UI, `@nexusim/protocol`, and
`@nexusim/client-core` as the browser client. The shell is planned as Tauri so
the native bridge can stay narrow and auditable.

## Current Status

- Architecture and package boundary exist.
- First TypeScript runtime shell exists through `createDesktopPlatformAdapter`.
- The shell includes development-only session storage, an in-memory message
  cache, static lifecycle/network ports, and unsupported local wakeup
  notifications.
- First Tauri v2 Rust runner skeleton exists under `src-tauri`; it exposes only
  the read-only `runtime_metadata` command. The shared Web shell can read this
  metadata for diagnostics and must fail closed on malformed bridge output.
- `shell-config.example.json` records the low-permission WebView config bridge
  for local LAN endpoints and desktop runtime identity. It can be rendered to
  `web/public/nexusim-shell-config.js` before a shell build.
- `npm --prefix clients run build:desktop-artifact` is the first-stage artifact
  wrapper. It prepares Web assets and then runs the available Tauri CLI. Use
  `node clients/tools/build-desktop-artifact.mjs --dry-run` to inspect the
  command and missing toolchain without building.
- No Windows installer is produced yet.
- `src-tauri/tauri.conf.json` records the intended shell boundary.

## Security Rules

- Desktop IPC must expose only explicit commands.
- Current IPC is metadata-only. It must not expose tokens, storage, file-system
  access or message APIs until a separate native capability ADR defines the
  audit and permission boundary.
- Shell config is endpoint and identity metadata only. It must not contain
  gateway tokens, refresh tokens, passwords, private keys, or arbitrary native
  capability flags.
- No broad file-system bridge.
- `DesktopDevelopmentSessionStore` is local-development only; token storage must
  use OS secure storage before production release.
- Auto-update and code signing are required before public distribution.

## Focused Checks

```powershell
npm --prefix clients run typecheck:desktop
npm --prefix clients run validate:desktop-tauri
npm --prefix clients run test:shell-config
npm --prefix clients run test:artifact-builders
node clients/tools/render-shell-config.mjs --input clients/desktop/shell-config.example.json
node clients/tools/build-desktop-artifact.mjs --dry-run
```
