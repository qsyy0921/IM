# NexusIM Desktop WebView Login Smoke - 2026-06-22 prereqs baseline

## Scope

This is a clean rerun of the login-level Windows desktop Tauri WebView smoke
after the client build-prerequisites report hardening commit.

It verifies:

- the local client-web BFF + push smoke still succeeds;
- the rendered Tauri WebView can log in through the public Web UI;
- the WebView can connect to push, receive `delivery.notify`, open the
  conversation, PullInbox the message and AckDelivery;
- the WebView displays the desktop native-store readiness contract with
  `tauri-sqlite`;
- the output summaries remain low-sensitive.

It does not verify:

- MSI / NSIS installer packaging or signing;
- Android APK packaging;
- long-running desktop stability, capacity or production SLO.

## Command

```powershell
.\loadtest\clientweb\run-local-smoke.ps1 `
  -RunName client-web-desktop-webview-login-smoke-20260622-05 `
  -SkipBuild `
  -RunDesktopWebViewLoginSmoke
```

The command ran after commit `bad96bf7` with `git_dirty=false`.

## Result

The command completed successfully.

Observed desktop WebView verdict:

```json
{
  "loginLevelDesktopUISmoke": true,
  "deliveryNotifyInWebView": true,
  "pullInboxInWebView": true,
  "ackDeliveryInWebView": true
}
```

Observed WebView flow:

```json
{
  "loginOK": true,
  "pushConnected": true,
  "openedConversation": true,
  "nativeStoreReadinessDisplayed": true,
  "nativeStoreBridge": "tauri-sqlite",
  "sentConversationSeq": 3,
  "observedMessage": true,
  "ackSeq": 3
}
```

Observed base client-web smoke facts:

```json
{
  "commit": "bad96bf7",
  "git_dirty": false,
  "success": true,
  "delivery_notify_seq": 2,
  "pull_inbox_item_count": 1,
  "pull_inbox_max_seq": 2,
  "ack_last_received_seq": 2,
  "device_delivery_cursor_seq": 2
}
```

Raw low-sensitive summaries:

- `H:\NexusIM\loadtest-results\client-web-desktop-webview-login-smoke-20260622-05\client-web-summary.json`
- `H:\NexusIM\loadtest-results\client-web-desktop-webview-login-smoke-20260622-05\desktop-webview-login-summary.json`

## Notes

- This smoke uses a temporary local fixture for auth input while the local BFF
  and push stack are alive. The fixture is removed after the run and is not
  persisted in the repository.
- The rerun confirms the PC shell path still works after the build-prereqs
  report was changed to hide local absolute paths and raw command output.
- Android remains blocked on JDK 17+ / Gradle / Android SDK or the explicit
  Docker builder bootstrap path.
