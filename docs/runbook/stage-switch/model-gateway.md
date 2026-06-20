# model-gateway Stage-Switch Review

Date: 2026-06-20

## Result

`model-gateway` SDD v0.1 is ready to enter the implementation stage. No P0/P1
blocker was found in the stage-switch review.

This review does not create `services/model-gateway` yet. The implementation
slice must switch `model-gateway` out of `future` in `service-registry.json` and
create the service directory in the same coherent change.

## Why Promotion Is Justified

- Independent data model: model invocations, route snapshots, budget windows,
  provider failures and model outbox are not owned by RAG, summary, agent,
  retrieval, ai-eval, policy or control-plane services.
- Independent scale profile: text generation, embedding, rerank, provider
  timeout, fallback and budget accounting scale differently from IM facts,
  search projection, memory projection and Agent approval.
- Independent failure boundary: model provider outage, budget exhaustion or
  circuit breaker state must not break durable IM delivery, search / memory
  projection, retrieval EvidencePack construction or Agent approval facts.
- Security boundary: provider URL / key handling, prompt safety envelopes,
  raw prompt / output retention and data-class routing need a dedicated owner
  with fail-closed defaults.
- Complexity reduction: keeping OpenAI / Claude / local-model adapters inside
  RAG, summary, agent, ingestion and eval would duplicate allowlist, timeout,
  budget, fallback and redaction logic across services.

## Boundary Checks

- retrieval-gateway still owns EvidencePack assembly and permission-filtered
  evidence retrieval.
- RAG / summary / agent still own prompt builders, citation / schema verifiers
  and user-visible response / proposal semantics.
- agent-service still owns proposal state; action-executor still owns approved
  tool execution; model-gateway must not approve or execute actions.
- control-plane-service can publish provider route / budget snapshots, but
  model-gateway owns runtime enforcement and invocation metadata.
- policy-service remains the model invocation precheck owner; model-gateway must
  call public policy APIs or explicit ports instead of reading policy tables.
- Metrics, events and DB rows must not contain raw prompt, raw model output,
  EvidencePack text, provider body, provider secret, user content, API key,
  private key, token, DSN or raw identifiers.

## Gate Impact For Next Slice

The implementation slice is broader than a docs-only change. It must update:

- `docs/runbook/service-registry.json`: switch `model-gateway` from `future` to
  `product-active` with local process / debug / observability metadata.
- `api/proto/nexusim/model/v1/model_gateway_service.proto`.
- `migrations/postgres/model-gateway/000001_model_gateway_core.sql`.
- `services/model-gateway` six-layer skeleton and `cmd/model-gateway`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full `check-local`
before push.

## First Implementation Scope

Keep the first code slice narrow:

```text
InvokeTextGeneration
GetModelInvocation
```

Use a deterministic mock provider or explicitly allowlisted local HTTP provider
for the first smoke. `CreateEmbedding`, `RerankEvidence`, outbox relay,
route-refresh worker, budget-reset worker and cleanup worker can follow after
the first gRPC smoke unless needed to prove the first path.

## Focused Acceptance For First Smoke

- `InvokeTextGeneration` requires service identity, caller service, use case,
  data class, prompt hash, prompt schema version, idempotency key and timeout.
- provider route / model id must come from an allowlisted route; public or
  secret-bearing provider URLs fail closed by default.
- request timeout, max output tokens, budget key and failure classification are
  enforced before recording invocation completion.
- raw prompt, raw output, EvidencePack text and provider body are never written
  to PostgreSQL, Kafka, metrics, logs or debug snapshots.
- `GetModelInvocation` returns only low-sensitive metadata: invocation id,
  caller, request type, data class, provider/model ids, token/cost estimate,
  hashes, status, failure class and trace refs.
- provider failure returns stable public errors and records low-sensitive
  failure class without poisoning caller citation / schema verification.
