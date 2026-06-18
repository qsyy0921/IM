# search-service Projection Smoke - 2026-06-19

## Scope

This smoke validates the first `search-service` foundation slice:

```text
conversation.timeline.events
-> search-service timeline-consumer
-> PostgreSQL search projection
-> SearchMessages
```

It does not validate LLM, RAG, summary, Agent, a production search backend, or
capacity.

## Evidence

| Field | Value |
| --- | --- |
| commit | `f2a57516b0793ac8d0e727d5f8a4a4c374e23cb3` |
| git_dirty | `false` |
| raw summary | `H:\NexusIM\loadtest-results\search-service-projection-smoke-20260619-004523\search-projection-summary.json` |
| topic | `conversation.timeline.search.20260619004523` |
| consumer_group | `nexusim-search-smoke-20260619004523` |
| search target | `127.0.0.1:10570` |
| checkpoint_offset_value | `9` |
| document_count | `4` |
| membership_status | `LEFT` |
| membership_join_seq | `1` |
| membership_leave_seq | `7` |
| projection_version | `12` |

## Verified Checks

| Check | Result |
| --- | --- |
| persisted message visible | passed |
| edited text visible | passed |
| original text hidden after edit | passed |
| revoked message hidden | passed |
| deleted message hidden | passed |
| message after member leave hidden | passed |
| stranger without membership hidden | passed |

## Event Chain

```text
conversation.member.joined.v1 seq=1
message.persisted.v1 seq=2
message.edited.v1 seq=2
message.persisted.v1 seq=3
message.revoked.v1 seq=4
message.persisted.v1 seq=5
message.deleted.v1 seq=6
conversation.member.left.v1 seq=7
message.persisted.v1 seq=8
```

## Notes

- The local smoke script creates the Kafka topic before starting
  `search-service timeline-consumer`; this avoids a local `kafka-go` reader
  startup race when the topic is created after the consumer group starts.
- The runner only cleans and reads `search-service` owned tables:
  `search_message_documents`, `search_membership_projection`, and
  `search_projection_checkpoints`.
- The next backend foundation step is `memory-service v0.1` design/contracts:
  group memory projection, `StructuredMemoryEvent`, source refs, validity
  windows, supersession, confidence, and review state.
