# push-gateway Redis Resume Negative Smoke - 2026-06-16

## Result

Passed.

This smoke validates Redis-backed resume recovery behavior for negative paths. It is not a capacity test and does not prove production Redis HA.

## Environment

| Item | Value |
| --- | --- |
| commit | `6e500c1` |
| git_dirty | `false` |
| scenario | `redis-resume-negative` |
| route_backend | `redis` |
| redis_mode | `single` |
| raw summary | `H:\NexusIM\loadtest-results\push-gateway-redis-resume-negative-smoke-20260616-clean\pushgateway-summary.json` |

Command:

```powershell
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario redis-resume-negative `
  -RouteBackend redis `
  -RunName push-gateway-redis-resume-negative-smoke-20260616-clean
```

## What Was Verified

1. Unknown client-supplied `resume_token` was not accepted.
   - requested token: `client-picked-token`
   - server returned a different token: `resume_17d0990028348e4646fe05c70ddcecc5`
   - server sent `server.resume_hint(reason=buffer_miss)`

2. A known resume token used by a different device was rejected.
   - error code: `PERMISSION_DENIED`
   - retryable: `false`

3. Redis resume buffer gap fell back to durable inbox.
   - runner sent 260 messages to force Redis resume buffer trimming
   - message seq range: `2..261`
   - reconnect with stale cursor received `server.resume_hint(reason=buffer_miss)`
   - `PullInbox` returned 260 items, max seq `261`
   - `delivery.ack.ok` advanced to seq `261`
   - device cursor advanced to seq `261`

## Key Counters

| Metric | Value |
| --- | ---: |
| `redis_resume_append_count` on consumer gateway | 260 |
| `redis_resume_miss_count` on WebSocket gateway | 2 |
| `redis_resume_permission_denied_count` on WebSocket gateway | 1 |
| `delivery_outbox_published` | 261 |
| `delivery_outbox_pending` | 0 |
| `delivery_outbox_dlq` | 0 |
| skipped frames while waiting for ACK | 1 |

## Interpretation

Redis-backed resume remains a best-effort online recovery optimization. When the token is unknown, belongs to a different device, or cannot cover the requested cursor window, the gateway returns `server.resume_hint` and the client must use local cursor state plus delivery-service `PullInbox`.

The reliable recovery path is still:

```text
server.resume_hint
-> client PullInbox
-> delivery.ack
-> delivery-service AckDelivery
-> delivery.ack.ok
```

This smoke proves that the negative Redis resume paths fall back to durable delivery without losing inbox data or ACK cursor correctness.
