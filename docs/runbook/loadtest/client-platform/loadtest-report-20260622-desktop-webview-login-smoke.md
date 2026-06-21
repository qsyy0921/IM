# NexusIM Desktop WebView Login Smoke - 2026-06-22

## Scope

This is the first clean login-level Tauri WebView smoke for the client platform
MVP foundation.

It verifies:

- the Windows desktop Tauri shell can load the prepared shared Web shell;
- WebView2/CDP automation can drive the rendered Web UI through login;
- the WebView receives an externally triggered `delivery.notify`;
- the WebView can open the conversation and observe the persisted message;
- the WebView can flush AckDelivery and report the acknowledged seq;
- output reports remain low-sensitive and do not include auth proof, tokens,
  local absolute paths, passwords or secrets.

It does not verify:

- MSI / NSIS installer packaging;
- Android APK packaging;
- offline storage using a native SQLite bridge;
- capacity, production SLO, or long-running desktop stability.

## Command

```powershell
.\loadtest\clientweb\run-local-smoke.ps1 -RunName client-web-desktop-webview-login-smoke-20260622-04 -SkipBuild -RunDesktopWebViewLoginSmoke
```

The command ran after commit `c72ea512` with `git_dirty=false`.

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
  "sentConversationSeq": 3,
  "observedMessage": true,
  "ackSeq": 3
}
```

Observed base client-web smoke facts:

```json
{
  "commit": "c72ea512",
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

- `H:\NexusIM\loadtest-results\client-web-desktop-webview-login-smoke-20260622-04\client-web-summary.json`
- `H:\NexusIM\loadtest-results\client-web-desktop-webview-login-smoke-20260622-04\desktop-webview-login-summary.json`

## Notes

- The desktop WebView smoke uses a temporary local fixture for auth input while
  the local BFF / push stack is alive. The fixture is not persisted in the repo
  and the output reports only record that auth input was required.
- The run proves login-level PC shell integration through the public Web UI,
  public BFF, push-gateway WebSocket and delivery Ack path.
- Android remains blocked on JDK 17+ / Gradle / Android SDK or the opt-in
  Android Docker builder profile.
