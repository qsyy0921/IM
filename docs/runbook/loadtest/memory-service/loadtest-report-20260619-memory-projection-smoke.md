# memory-service Projection Smoke - 2026-06-19

## Scope

This smoke validates the first `memory-service` foundation implementation:

```text
conversation.timeline.events
-> memory-service timeline-consumer
-> PostgreSQL memory projection
-> QueryMemoryEvents / GetMemoryEvent
```

It does not validate LLM extraction, RAG, Agent execution, long-term profile
approval, production graph quality, or capacity.

## Evidence

| Field | Value |
| --- | --- |
| commit | `e399201cd0dad080c9d43035975e0ca474b88683` |
| git_dirty | `false` |
| raw summary | `H:\NexusIM\loadtest-results\memory-service-projection-smoke-20260619-015627\memory-projection-summary.json` |
| topic | `conversation.timeline.memory.20260619015627` |
| consumer_group | `nexusim-memory-smoke-20260619015627` |
| memory target | `127.0.0.1:10580` |
| checkpoint_offset_value | `3` |
| memory_event_count | `1` |
| membership_status | `ACTIVE` |
| membership_join_seq | `1` |
| projection_version | `2` |

## Verified Checks

| Check | Result |
| --- | --- |
| memory projected | passed |
| source ref projected | passed |
| GetMemoryEvent works | passed |
| stranger without membership hidden | passed |
| revoked memory hidden | passed |
| profile aggregates not created by first-stage rules | passed |

## Event Chain

```text
conversation.member.joined.v1 seq=1
message.persisted.v1 seq=2
message.revoked.v1 seq=3
```

## Notes

- The local smoke script creates the Kafka topic before starting
  `memory-service timeline-consumer`.
- The runner applies `migrations/postgres/memory/000001_memory_core.sql` before
  publishing events, then only cleans and reads `memory-service` owned tables.
- This is a source-backed rules projection smoke. It intentionally keeps memory
  events in `PENDING` and does not promote single group messages into ACTIVE
  profile facts.
