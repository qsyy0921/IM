# ai-eval-service SDD

`ai-eval-service` is the first persistent catalog for NexusIM AI evaluation runs.
It turns local AI eval harness outputs into low-sensitive run records that can be
queried, compared, audited and later promoted into CI / regression gates.

## Scope

First stage:

- record an eval run summary by `tenant_id + run_id`;
- query one run;
- list runs by suite / status with cursor pagination;
- store only low-sensitive metadata, counters and artifact references.
- record an existing low-sensitive local AI eval summary through a recorder
  smoke without storing raw eval payload.

Out of scope:

- running LLM / Python workers;
- storing raw EvidencePack, raw prompt, raw model output, provider response body,
  user content, secrets or tool input;
- deciding pass/fail policy for production deployment;
- publishing Kafka events.

## Data Model

`migrations/postgres/ai-eval-service/000001_ai_eval_core.sql` creates
`ai_eval_runs`:

- identity: `tenant_id`, `run_id`;
- grouping: `suite_id`, `stage`, `adapter`;
- result: `status`, `case_count`, `passed_count`, `failed_count`,
  `skipped_count`;
- evidence refs: `summary_ref`, `report_ref`;
- low-sensitive `metadata_json`;
- timestamps: `created_at`, `updated_at`, `completed_at`.

`metadata_json` must be a JSON object. It is for low-sensitive knobs such as
case family, fixture adapter name, local summary schema version or git commit.
It must not contain raw prompts, raw model output, evidence text or user ids.

## API

`api/proto/nexusim/aieval/v1/ai_eval_service.proto` defines:

- `RecordEvalRun`
- `GetEvalRun`
- `ListEvalRuns`

All RPCs require verified auth context or explicit first-stage auth context in
local smoke. Tenant is forced from `AuthContext`, not trusted from caller payload.

## Runtime

Modes:

- `noop`: debug endpoints only.
- `grpc`: PostgreSQL-backed catalog service.

Environment:

- `NEXUSIM_AI_EVAL_SERVICE_MODE`
- `NEXUSIM_AI_EVAL_GRPC_ADDR`
- `NEXUSIM_AI_EVAL_DEBUG_ADDR`
- `NEXUSIM_PG_DSN`

Debug endpoints expose only first-stage service info:

- `/healthz`
- `/readyz`
- `/metrics`
- `/debug/metrics`

## Boundaries

- Eval harnesses can write run summaries, but the service does not execute evals.
- RAG / summary / Agent still consume EvidencePack through their own services.
- Action execution remains gated by policy, proposal, approval, executor and
  audit; eval catalog records cannot authorize actions.
- Python AI Worker remains candidate-only; Go owns persistence and audit.

## Next

- Add richer multi-adapter regression suite aggregation without storing raw user
  content.
- Add first regression gate semantics for selected low-sensitive suites.
- Keep CI / production policy decisions separate from the first-stage catalog.
