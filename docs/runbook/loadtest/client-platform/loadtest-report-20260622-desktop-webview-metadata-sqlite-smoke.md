# NexusIM Desktop WebView Metadata SQLite Smoke - 2026-06-22

## Scope

This smoke reruns the Tauri WebView metadata callback path after the desktop
SQLite local-store bridge landed in commit `2b67b0e1`.

It verifies:

- the Windows desktop Tauri shell can load the prepared shared Web shell;
- the rendered WebView can invoke the fixed `runtime_metadata` Tauri IPC;
- the WebView can POST a low-sensitive metadata report to a loopback callback;
- the native metadata now reports `nativeBridgeVersion=0.2.0`;
- the native local-store readiness reports `tauri-sqlite` with
  `nativeStoreReady=true`.

It does not verify:

- login inside the Tauri WebView;
- PullInbox, `delivery.notify`, SendMessage or AckDelivery inside the WebView;
- MSI / NSIS installer packaging;
- Android APK packaging.

## Command

```powershell
$run='client-desktop-webview-metadata-sqlite-20260622-01'
$dir=Join-Path 'H:\NexusIM\loadtest-results' $run
New-Item -ItemType Directory -Force -Path $dir | Out-Null
npm --prefix clients run smoke:desktop-webview-metadata -- --run-id $run
```

Raw stdout capture:

```text
H:\NexusIM\loadtest-results\client-desktop-webview-metadata-sqlite-20260622-01\desktop-webview-metadata-summary.json
```

## Result

The command completed successfully on clean commit `2b67b0e1`.

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
    "nativeBridgeVersion": "0.2.0",
    "runtimeLabel": "NexusIM desktop shell",
    "localStore": {
      "currentDefault": "local-storage",
      "productionTarget": "sqlite",
      "nativeStoreReady": true,
      "nativeStoreReason": "",
      "nativeStoreBridge": "tauri-sqlite"
    }
  },
  "runtimeConfig": {
    "apiConfigured": true,
    "pushConfigured": true
  }
}
```

## Notes

- This is metadata-only evidence. It proves the native bridge reports the
  `tauri-sqlite` local-store capability from inside the rendered Tauri WebView.
- It does not prove the login / PullInbox / AckDelivery path. That is covered
  by the separate login-level WebView smoke.
- The smoke output remains low-sensitive: no token, password, local absolute
  path or message payload is recorded in the report.
