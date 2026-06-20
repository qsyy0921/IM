# presence-service gRPC smoke report 2026-06-20

## Scope

Local first-stage `presence-service` smoke for the product-active slice.
The run verifies the minimal online-state path:

```text
UpdatePresence -> idempotent replay -> GetPresence visibility filtering
-> UpdateTyping -> presence_outbox low-sensitive payload check
```

Raw summary and logs are stored outside the repository:

```text
H:\NexusIM\loadtest-results\presence-grpc-smoke-20260620
```

## Environment

- commit at run: `ab14866146d1722511bb2a28bcf034ff5e2bb312`
- git_dirty: `true` because this implementation slice was still uncommitted
- target: `127.0.0.1:50276`
- tenant: `tenant-presence-grpc-smoke-20260620`

## Result

Status: passed.

Key facts from `presence-grpc-summary.json`:

| Check | Result |
| --- | --- |
| `UpdatePresence(ONLINE)` | `online_visible_state=ONLINE` |
| same request replay | `replay_same_state=true` |
| replay outbox behavior | `replay_did_not_write_outbox_row=true` |
| self device count | `1` |
| unauthorized visible state | `UNKNOWN` |
| `UpdatePresence(INVISIBLE)` visible / actual | `OFFLINE` / `INVISIBLE` |
| `UpdateTyping(STARTED)` | `STARTED` |
| `presence_outbox.total` | `3` |
| `presence_outbox.pending` | `3` |
| `presence_outbox.dlq` | `0` |
| outbox payload low-sensitive scan | `true` |

## Boundaries

This smoke proves only the local gRPC + PostgreSQL first path.

It does not prove:

- push-gateway session event consumer.
- `SubscribePresence`.
- stale session scanner.
- presence outbox relay / Kafka publication.
- Redis hot-state integration.
- production HA, sizing, long-run SLO, or provider-grade presence platform.

`presence-service` remains a product presence projection. It does not replace
delivery-service durable inbox / ACK, conversation membership facts, or
policy-service permission decisions.
