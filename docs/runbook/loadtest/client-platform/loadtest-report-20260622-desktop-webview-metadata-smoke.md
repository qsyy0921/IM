# NexusIM Desktop WebView Metadata Smoke - 2026-06-22

## Scope

This is the first real Tauri WebView metadata callback smoke for the client
platform MVP foundation.

It verifies:

- the Windows desktop Tauri shell can load the prepared shared Web shell;
- the rendered WebView can invoke the read-only `runtime_metadata` Tauri IPC;
- the WebView can POST a low-sensitive metadata report to a loopback callback;
- the smoke output does not contain local absolute paths or sensitive fields.

It does not verify:

- login inside the Tauri WebView;
- PullInbox, `delivery.notify`, SendMessage or AckDelivery inside the WebView;
- MSI / NSIS installer packaging;
- Android APK packaging.

## Command

```powershell
npm --prefix clients run smoke:desktop-webview-metadata
```

## Result

The command completed successfully.

Observed low-sensitive verdict:

```json
{
  "metadataWebViewSmoke": true,
  "loginLevelDesktopUISmoke": false
}
```

Observed callback summary:

```json
{
  "received": true,
  "schemaVersion": "nexusim.shell-webview-metadata-smoke.v1",
  "mode": "metadata",
  "shellTarget": "windows-desktop",
  "nativeMetadataReady": true,
  "native": {
    "target": "windows-desktop",
    "nativeBridgeVersion": "0.1.0",
    "runtimeLabel": "NexusIM desktop shell"
  },
  "runtimeConfig": {
    "apiConfigured": true,
    "pushConfigured": true
  }
}
```

## Notes

- The desktop artifact wrapper now forces a fresh app-level Tauri build output
  when a custom shell config is supplied, preventing stale embedded shell config
  from posting to an old callback port.
- The callback timeout starts after the desktop process launches, so the smoke
  does not count release build time as WebView callback time.
- The next PC client step is a login-level Tauri shell smoke that drives the
  rendered WebView through login, PullInbox, online notify and AckDelivery.
