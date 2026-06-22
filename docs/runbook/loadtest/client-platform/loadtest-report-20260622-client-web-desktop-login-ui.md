# NexusIM Client Web / Windows Desktop Login UI Smoke

Date: 2026-06-22

## Scope

This report records the first account-password oriented client shell pass for
the browser / Windows desktop priority path.

The visible UI now behaves more like a normal IM client:

- account and password are the primary login inputs;
- tenant, endpoint, device and runtime diagnostics are hidden from normal users;
- conversation list, chat pane, message bubbles and composer are visible as the
  main surface;
- automation selectors remain available for smoke runners without exposing
  internal configuration in the product UI.

This is not a production UI release, capacity test, signed installer, or mobile
baseline.

## Evidence

Focused checks run during this slice:

```powershell
npm --prefix clients run typecheck
npm --prefix clients run build:web
npm --prefix clients run typecheck:desktop
npm --prefix clients run validate:desktop-tauri
npm --prefix clients run test:shell-web-assets
npm --prefix clients run test:shell-config
npm --prefix clients run test:desktop-webview-login-smoke
npm --prefix clients run test:android-webview-login-smoke
npm --prefix clients run test:clientweb-smoke-hooks
npm --prefix clients run test:android-webview-login-smoke-plan
npm --prefix clients run validate:android-native
```

Real Windows desktop WebView login smoke:

```powershell
.\loadtest\clientweb\run-local-smoke.ps1 `
  -RunDesktopWebViewLoginSmoke `
  -SkipBuild `
  -RunName client-web-desktop-login-ui-20260622-01
```

Result directory:

```text
H:\NexusIM\loadtest-results\client-web-desktop-login-ui-20260622-01
```

Key result:

```text
desktop verdict:
  loginLevelDesktopUISmoke = true
  deliveryNotifyInWebView = true
  pullInboxInWebView = true
  ackDeliveryInWebView = true

clientweb summary:
  success = true
  delivery.notify seq = 2
  PullInbox item_count = 1
  AckDelivery last_received_seq = 2
```

Preview screenshots captured outside the repository:

```text
H:\NexusIM\loadtest-results\client-ui-preview-20260622\web-desktop-v2.png
H:\NexusIM\loadtest-results\client-ui-preview-20260622\web-mobile-v2.png
```

## Android Note

Android was not the priority of this slice, but one blocking white-screen issue
was fixed while debugging:

- Android WebView assets now rewrite root-absolute Vite asset URLs to relative
  paths for `WebViewAssetLoader`.
- Android metadata WebView smoke passed on the installed APK over network ADB.

Android login-level WebView smoke is not claimed in this report. It remains a
separate follow-up when the user switches back to Android.

## Limitations

- The Windows desktop output is still a standalone development artifact, not a
  signed installer.
- Web auth still uses the current first-stage BFF/session model; production Web
  token hardening remains future work.
- The UI is a pragmatic IM shell for smoke and demonstration, not final product
  design.
