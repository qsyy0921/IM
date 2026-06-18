# memory-service Projection Smoke

This smoke validates the first memory foundation slice:

```text
conversation.timeline.events
-> memory-service timeline-consumer
-> PostgreSQL memory projection
-> QueryMemoryEvents / GetMemoryEvent
```

It does not validate LLM extraction, RAG, Agent behavior, profile aggregation
approval, or production memory graph quality.

## Runner

Build:

```powershell
. .\tools\go-env.ps1
go build -o .\bin\memory-smoke.exe ./loadtest/memory
```

Run the local smoke. The script creates a per-run Kafka topic before starting
`memory-service grpc` and `memory-service timeline-consumer`:

```powershell
.\loadtest\memory\run-local-smoke.ps1
```

Raw summary is written outside the repository as
`memory-projection-summary.json`.

Latest report:

- `docs/runbook/loadtest/memory-service/loadtest-report-20260619-memory-projection-smoke.md`

## Verified Invariants

- member join makes the viewer eligible for memory query results;
- persisted message becomes a PENDING `StructuredMemoryEvent`;
- source ref points back to the message and timeline event;
- `GetMemoryEvent` returns the projected memory;
- stranger without membership sees no memory;
- revoke hides the memory event;
- first-stage rules projection does not create profile aggregates.
