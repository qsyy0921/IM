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
- No Windows installer is produced yet.
- `src-tauri/tauri.conf.json` records the intended shell boundary.

## Security Rules

- Desktop IPC must expose only explicit commands.
- No broad file-system bridge.
- `DesktopDevelopmentSessionStore` is local-development only; token storage must
  use OS secure storage before production release.
- Auto-update and code signing are required before public distribution.
