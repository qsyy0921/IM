# Client Platform Loadtest Reports

This directory stores focused client-platform smoke reports.

Current scope:

- Browser/Web client path through `api-gateway` HTTP BFF and `push-gateway`
  WebSocket.
- Durable facts are still verified through BFF `PullInbox` / `AckDelivery`.
- Reports in this directory are not capacity or production SLO evidence.

Reports:

- `loadtest-report-20260621-client-web-bff-push-smoke.md`: first WIP smoke,
  recorded with `git_dirty=true`.
- `loadtest-report-20260621-client-web-bff-push-clean-baseline.md`: first clean
  committed baseline for the browser MVP path.
- `loadtest-report-20260621-client-web-bff-push-wired-172-smoke.md`: first WIP
  private `172.31.50.1` wired-address smoke, recorded with `git_dirty=true`;
  clean committed rerun is still required.
