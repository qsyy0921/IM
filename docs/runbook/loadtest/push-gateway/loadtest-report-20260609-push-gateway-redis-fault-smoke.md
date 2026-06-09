# push-gateway Redis Fault Smoke - 2026-06-09

## Scope

This smoke verifies the current Redis route failure semantics for `push-gateway`.

It is not a Redis HA test and not a capacity test. The goal is to prove the distributed IM chain keeps the durable delivery path correct when the online route layer is unavailable:

```text
WebSocket session is registered
-> Redis route is stopped
-> SendMessage still writes message / timeline / delivery inbox
-> online delivery.notify is not required to succeed
-> client recovers by PullInbox
-> Redis is restored
-> client reconnects and ACKs through delivery-service
```

## Environment

| Item | Value |
| --- | --- |
| Repo commit | `074902b4dad172174992bfefb4c0ba15a9365e9b` |
| Dirty state | `false` |
| Scenario | `redis-fault` |
| Route backend | `redis` |
| Redis address | `127.0.0.1:6379` |
| Result JSON | `H:\NexusIM\loadtest-results\push-gateway-redis-fault-smoke-20260609-195200\pushgateway-summary.json` |

## How It Was Run

```powershell
. .\tools\go-env.ps1
.\loadtest\pushgateway\run-local-smoke.ps1 `
  -Scenario redis-fault `
  -RouteBackend redis `
  -RunName push-gateway-redis-fault-smoke-20260609-195200
```

The runner uses the normal local smoke chain:

```text
conversation-service
-> message-service
-> message-service outbox relay
-> Kafka conversation.timeline.events
-> delivery-service timeline consumer
-> delivery-service outbox relay
-> Kafka im.delivery.events
-> push-gateway delivery consumer
-> Redis route / PubSub
-> push-gateway WebSocket
```

For this run, the runner deliberately executed:

```powershell
docker stop nexusim-redis | Out-Null
```

after the WebSocket session had been established, then sent one message. After durable recovery via `PullInbox`, it restored Redis with:

```powershell
docker start nexusim-redis | Out-Null
```

and reconnected the WebSocket client before ACK.

## Result

| Metric | Value |
| --- | --- |
| Success | `true` |
| `git_dirty` | `false` |
| `member_join.boundary_seq` | `1` |
| `send_message.conversation_seq` | `2` |
| `redis_fault.notify_received` | `false` |
| `pull_inbox.item_count` | `1` |
| `pull_inbox.max_seq` | `2` |
| `delivery.ack.ok.last_received_seq` | `2` |
| `cursor_last_received_seq` | `2` |
| `delivery_outbox_total` | `2` |
| `delivery_outbox_published` | `2` |
| `delivery_outbox_pending` | `0` |
| `delivery_outbox_dlq` | `0` |

## Interpretation

This validates the current design boundary:

- Redis route is an online wakeup path, not the durable delivery source.
- When Redis route is unavailable, `delivery.notify` may be missed.
- The message is still projected to `user_inbox`.
- The client can recover through `PullInbox`.
- ACK still goes through `delivery-service AckDelivery`, and the device cursor advances to the recovered seq.

The important invariant is:

```text
Redis/WebSocket can fail without losing message facts, delivery inbox rows, or ACK cursor semantics.
```

## Limitations

- This is one Redis stop/start smoke, not a full Redis HA or failover test.
- It does not prove Redis Cluster, Sentinel, network partition, or subscriber-down behavior.
- It does not prove cross-instance resume buffer; reconnect recovery still relies on durable `PullInbox`.
- It does not prove capacity under Redis failure.
- Current policy is intentionally simple:
  - connect route write failure remains fail-closed;
  - online notify lookup / publish failure is fail-open and relies on `PullInbox`.

## Interview Notes

The useful explanation is:

```text
I separated reliable delivery from online notification.
Redis route is allowed to be best-effort because it only wakes online WebSocket sessions.
If Redis or WebSocket fails, the user does not lose messages; the client falls back to PullInbox and then ACKs through delivery-service.
```

Do not claim:

```text
Redis failure is fully solved in production.
```

Claim instead:

```text
We validated the failure boundary: online notify can be dropped, but durable inbox recovery and ACK remain correct.
```
