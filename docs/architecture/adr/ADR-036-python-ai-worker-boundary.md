# ADR-036: Python AI Worker Boundary

## Status

Accepted

## Context

NexusIM 的核心后端服务已经使用 Go、gRPC、PostgreSQL、Kafka、outbox、
repair/audit 和本地门禁形成统一工程基线。AI 应用底座继续扩展到 RAG、
summary、Agent、MCP/tool 和 eval 后，会需要 embedding、rerank、LLM provider、
memory extraction、profile aggregation、planner prototype 和 benchmark runner。

这些能力在 Python 生态中迭代更快，但如果 Python 直接成为第二套业务后端，
会破坏现有权限、事务、审计、outbox 和服务边界。

## Decision

Python 只作为 AI Worker 层，不作为 IM 业务事实源或最终控制面。

默认边界：

```text
Go services
-> gRPC / HTTP / Kafka contract
-> Python AI workers
-> candidate result / model result / eval result
-> Go services validate, authorize, audit, persist or reject
```

Go 继续负责：

- public API / gRPC service boundary;
- tenant / user / device / agent identity;
- policy precheck, tool policy, approval and audit;
- durable state, PostgreSQL transaction, outbox and Kafka publish;
- DLQ / repair / operator;
- EvidencePack validation, citation verification and safe persistence.

Python worker 可以负责：

- LLM provider adapter and prompt experimentation;
- embedding and rerank;
- memory extraction candidate and profile aggregation candidate;
- planner / critic prototype;
- offline eval, benchmark and dataset processing;
- local model runtime adapter.

Python worker 禁止：

- directly writing IM business PostgreSQL tables;
- bypassing policy-service, retrieval-gateway, mcp-gateway or action-executor;
- holding final approval state or durable business state;
- executing high-risk business actions;
- persisting raw prompt, raw provider body, token, secret or full sensitive payload
  outside an approved low-sensitive audit contract.

Communication rules:

| Pattern | Use for | Boundary |
| --- | --- | --- |
| gRPC | long-lived low-latency worker service | protobuf contract, deadline, trace, structured error |
| HTTP | simple provider adapter or local prototype | explicit JSON schema, timeout, retry budget |
| Kafka | async / batch / long-running work | idempotent task/result ids, DLQ and replay path |
| CLI subprocess | local experiments only | not for production hot path |

Preferred repository organization when Python code is introduced:

```text
ai/python/
  pyproject.toml
  README.md
  nexusim_ai_common/
    config.py
    logging.py
    tracing.py
    kafka.py
    grpc_client.py
    safety.py
  workers/
    llm_worker/
    embedding_worker/
    rerank_worker/
    memory_worker/
    planner_worker/
    eval_worker/
  contracts/
  scripts/
```

Suggested Python tooling:

```text
Python 3.12, uv, pydantic, grpcio or FastAPI, pytest, ruff, mypy,
opentelemetry, aiokafka or confluent-kafka when Kafka is required.
```

## Consequences

Benefits:

- AI model work can iterate quickly without forking the Go backend architecture.
- Go remains the source of truth for security, state, approval and audit.
- Python failures degrade into candidate / provider failure handling instead of
  corrupting durable IM facts.
- RAG / summary / Agent can share the same worker boundary.

Costs:

- Cross-language contracts need protobuf / JSON schema discipline.
- Local dev and Docker runtime need an additional Python toolchain.
- Evaluation must cover worker timeout, malformed output, unsafe output and
  provider failure fallback.

## Validation

Before introducing any Python worker into a real path:

- Add an SDD or service brief section naming the worker boundary and caller.
- Define the gRPC / HTTP / Kafka request and response schema.
- Add timeout, cancellation and retry budget.
- Add output sanitizer / PII-secret filter where text leaves the worker.
- Prove Go caller rejects malformed, missing, unsafe or policy-invalid results.
- Add eval cases for provider failure fallback and unsafe output.
- Keep raw model/provider artifacts outside the repository and outside business
  fact tables.
