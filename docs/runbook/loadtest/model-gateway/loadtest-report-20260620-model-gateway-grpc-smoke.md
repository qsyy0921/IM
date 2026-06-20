# model-gateway gRPC smoke report

Date: 2026-06-20

## Scope

This smoke verifies the first `model-gateway` runtime path:

```text
InvokeTextGeneration -> idempotent replay -> GetModelInvocation
```

It uses the first-stage deterministic mock provider. It does not verify real
OpenAI / Claude / local-model HTTP providers, embedding, rerank, outbox relay,
route-refresh worker, budget-reset worker or cleanup worker.

## Environment

- Commit: `af345845efe537730047ec07e7bf20577e3c7090`
- Git dirty at run: `false`
- Runner: `loadtest/modelgateway`
- Result dir: `H:\NexusIM\loadtest-results\model-gateway-grpc-smoke-20260620-235328`
- Target: `127.0.0.1:59553`
- Tenant: `tenant-model-gateway-grpc-smoke-20260620-235328`

## Result

The smoke passed.

Key facts:

- `provider_id = local-mock`
- `model_id = deterministic-text-v1`
- `InvokeTextGeneration` returned `output_returned = true`
- `output_hash = sha256:d133a928724adbd1b99f3cafbfb010a88e43c20bbfa39455288d0001f2a84316`
- Replay used the same invocation and returned `output_returned = false`
- `GetModelInvocation.status = SUCCEEDED`
- Token usage: `input_tokens = 7`, `output_tokens = 5`
- `estimated_cost_microunits = 120`
- `model_outbox`: `total = 1`, `pending = 1`, `published = 0`, `dlq = 0`
- DB payload low-sensitive check: `true`

## Boundary Confirmed

- Raw prompt was sent only in the synchronous gRPC request to the local mock
  provider path.
- Raw prompt and raw model output were not stored in `model_invocations` or
  `model_outbox`.
- `GetModelInvocation` returned only low-sensitive metadata.
- The replay path did not return raw output again and did not create a second
  outbox row.

## Remaining

- Real external provider adapters.
- Embedding and rerank APIs.
- Model outbox relay to `im.model.events`.
- Route-refresh, budget-reset and cleanup workers.
- Provider-grade budget / policy / route integration with control-plane and
  policy services.
