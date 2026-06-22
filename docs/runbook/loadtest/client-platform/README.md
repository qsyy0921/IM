# Client Platform Loadtest Reports

This directory stores focused client-platform smoke reports.

Current scope:

- Browser/Web client path through `api-gateway` HTTP BFF and `push-gateway`
  WebSocket.
- Durable facts are still verified through BFF `PullInbox` / `AckDelivery`.
- Reports in this directory are not capacity or production SLO evidence.
- Desktop composed smoke can combine an existing `client-web-summary.json` with
  desktop artifact launch evidence, but it is an intermediate evidence bundle,
  not Tauri WebView GUI automation.
- WebView metadata smoke uses shell config `smokeCallbackURL` with loopback
  HTTP only. It is intended to prove native metadata is read from inside the
  shell; it does not include login form data or message flow.
- Desktop WebView login smoke is driven externally through WebView2/CDP and a
  local fixture file. It does not put auth input into shell config or output
  reports. It is intended to run while `loadtest/clientweb/run-local-smoke.ps1`
  keeps the BFF and push stack alive.

Reports:

- `loadtest-report-20260621-client-web-bff-push-smoke.md`: first WIP smoke,
  recorded with `git_dirty=true`.
- `loadtest-report-20260621-client-web-bff-push-clean-baseline.md`: first clean
  committed baseline for the browser MVP path.
- `loadtest-report-20260621-client-web-bff-push-wired-172-smoke.md`: first WIP
  private `172.31.50.1` wired-address smoke, recorded with `git_dirty=true`;
  clean committed rerun is still required.
- `loadtest-report-20260621-client-web-bff-push-wired-172-clean-baseline.md`:
  first clean committed baseline for the browser MVP path on the Windows wired
  `172.31.50.1` address.
- `loadtest-report-20260622-desktop-webview-metadata-smoke.md`: first real
  Tauri WebView metadata callback smoke; proves the rendered PC shell can read
  native runtime metadata and post a low-sensitive loopback report.
- `loadtest-report-20260622-desktop-webview-login-smoke.md`: first clean
  login-level Tauri WebView smoke; proves the rendered PC shell can log in,
  receive `delivery.notify`, PullInbox and AckDelivery through public client
  paths.
- `loadtest-report-20260622-desktop-webview-metadata-sqlite-smoke.md`: rerun
  after the desktop SQLite bridge landed; proves rendered Tauri WebView metadata
  reports `nativeBridgeVersion=0.2.0` and `tauri-sqlite` ready.
- `loadtest-report-20260622-desktop-webview-login-sqlite-smoke.md`: rerun
  login-level Tauri WebView smoke after the SQLite bridge landed; proves login,
  notify, PullInbox and AckDelivery while the WebView displays `tauri-sqlite`
  native-store readiness.
- `loadtest-report-20260622-desktop-webview-login-prereqs-baseline.md`: clean
  rerun after the build-prerequisites report hardening; proves the PC WebView
  login path still works with `git_dirty=false` on commit `bad96bf7`.
- `loadtest-report-20260622-android-local-apk-build.md`: first Windows local
  Android debug APK build baseline using the F-drive JDK / Gradle / Android SDK
  toolchain; proves packaging and artifact collection only, not device install
  or WebView login smoke.
- `loadtest-report-20260622-client-web-desktop-login-ui.md`: browser /
  Windows-priority UI pass; proves the account-password oriented IM shell still
  passes real Windows Tauri WebView login, notify, PullInbox and AckDelivery
  through public client paths.

Useful command:

```powershell
npm --prefix clients run smoke:desktop-composed -- --clientweb-summary <client-web-summary.json>
npm --prefix clients run smoke:desktop-webview-metadata
.\loadtest\clientweb\run-local-smoke.ps1 -RunDesktopWebViewLoginSmoke
```
