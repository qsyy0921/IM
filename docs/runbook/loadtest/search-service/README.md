# search-service Projection Smoke

This smoke validates the first search foundation slice:

```text
conversation.timeline.events
-> search-service timeline-consumer
-> PostgreSQL search projection
-> SearchMessages
```

It does not validate LLM, RAG, summary, Agent, or production search backend
behavior.

## Runner

Build:

```powershell
. .\tools\go-env.ps1
go build -o .\bin\search-smoke.exe ./loadtest/search
```

Run the local smoke. The script creates a per-run Kafka topic before starting
the search timeline consumer, then starts `search-service grpc` and
`search-service timeline-consumer` as temporary local processes:

```powershell
.\loadtest\search\run-local-smoke.ps1
```

To run against already started search-service roles, call the runner directly:

```powershell
.\bin\search-smoke.exe `
  --pg-dsn "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  --kafka-brokers "localhost:9092" `
  --topic "conversation.timeline.events" `
  --consumer-group "nexusim-search-service" `
  --search-target "127.0.0.1:10570" `
  --result-root "H:\NexusIM\loadtest-results"
```

Raw summary is written outside the repository as `search-projection-summary.json`.

## Verified Invariants

- member join makes the viewer eligible for search results;
- persisted message is searchable;
- edit replaces old searchable text;
- stranger without membership sees no hit;
- revoke hides the message;
- delete hides the message;
- member leave makes later messages invisible to that viewer;
- checkpoint and projection rows are written by `search-service`.

The runner only cleans and reads `search-service` owned tables:
`search_message_documents`, `search_membership_projection`, and
`search_projection_checkpoints`.
