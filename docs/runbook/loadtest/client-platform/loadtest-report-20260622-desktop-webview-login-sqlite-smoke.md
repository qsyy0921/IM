# NexusIM Desktop WebView Login SQLite Smoke - 2026-06-22

## Scope

This smoke reruns the login-level Tauri WebView path after the desktop SQLite
local-store bridge landed in commit `2b67b0e1`.

It verifies:

- the local client-web BFF / push stack can be started for a fresh tenant;
- the rendered Tauri WebView can log in through the public BFF path;
- the WebView can connect to push-gateway and receive an externally triggered
  `delivery.notify`;
- the WebView can PullInbox, observe the message and AckDelivery;
- the WebView displays native local-store readiness with
  `nativeStoreBridge=tauri-sqlite` and `nativeStoreReady=true`.

It does not verify:

- MSI / NSIS installer packaging;
- Android APK packaging;
- capacity, production SLO or HA behavior.

## Command

```powershell
.\loadtest\clientweb\run-local-smoke.ps1 `
  -RunName client-web-desktop-webview-login-sqlite-20260622-01 `
  -SkipBuild `
  -RunDesktopWebViewLoginSmoke `
  -DesktopWebViewSkipWebBuild
```

Raw summaries:

```text
H:\NexusIM\loadtest-results\client-web-desktop-webview-login-sqlite-20260622-01\client-web-summary.json
H:\NexusIM\loadtest-results\client-web-desktop-webview-login-sqlite-20260622-01\desktop-webview-login-summary.json
```

## Result

The command completed successfully on clean commit `2b67b0e1`.

Client-web setup summary:

```json
{
  "commit": "2b67b0e1",
  "git_dirty": false,
  "success": true,
  "member_boundary_seq": 1,
  "send_message_conversation_seq": 2,
  "pull_inbox_item_count": 1,
  "ack_delivery_last_received_seq": 2,
  "device_delivery_cursor_seq": 2
}
```

Desktop WebView verdict:

```json
{
  "loginLevelDesktopUISmoke": true,
  "deliveryNotifyInWebView": true,
  "pullInboxInWebView": true,
  "ackDeliveryInWebView": true
}
```

Desktop WebView native-store evidence:

```json
{
  "nativeStoreReadinessDisplayed": true,
  "nativeStoreReadiness": {
    "ok": true,
    "currentDefault": "local-storage",
    "productionTarget": "sqlite",
    "nativeStoreReady": true,
    "nativeStoreReason": "",
    "nativeStoreBridge": "tauri-sqlite"
  },
  "sentConversationSeq": 3,
  "observedMessage": true,
  "ackSeq": 3
}
```

## Notes

- The smoke uses WebView2/CDP automation against the rendered Tauri WebView.
- Local smoke-only auth input is kept in a temporary fixture and is not written
  into shell config or output reports.
- This proves desktop runtime metadata and login-level WebView flow on the
  current desktop source. It is still not a production packaging, signing,
  updater or capacity result.
