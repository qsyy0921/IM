# model-gateway SDD v0.1 Draft

## 1. 服务定位

`model-gateway` 是 NexusIM AI 应用底座中的模型 provider 统一入口。它负责文本生成、
embedding、rerank、provider route、超时、重试、fallback、成本预算、限流和低敏调用
审计。

职责：

- 提供统一的 `InvokeTextGeneration`、`CreateEmbedding`、`RerankEvidence` 调用面。
- 适配 OpenAI-compatible、Claude-compatible、本地模型 HTTP、embedding 和 rerank provider。
- 执行 provider allowlist、模型 allowlist、tenant budget、timeout、retry、circuit breaker。
- 记录低敏 invocation metadata、token / cost 估算、latency、failure class 和 trace refs。
- 为 RAG、summary、agent、ai-eval、knowledge-ingestion 提供可控模型调用能力。

不负责：

- 不拥有 IM 事实、search projection、memory projection、EvidencePack 或 citations。
- 不决定回答是否正确，不替代 RAG / summary / agent 的 citation verifier。
- 不决定 Agent proposal、approval、tool execution 或 audit。
- 不直接读取 message、conversation、search、memory、retrieval 私有表。
- 默认不持久化 raw prompt、raw model output、EvidencePack 正文、tool input/output 或 provider response body。
- 不保存 provider secret、API key、private key、token 或用户 PII 明文。

## 2. 上下游

| 方向 | 服务 / 组件 | 交互 |
| --- | --- | --- |
| 上游 | rag-service / summary-service | 提交基于 EvidencePack 构造的生成请求 |
| 上游 | agent-service | 提交 planner / proposal candidate 请求；不提交 approval 决策 |
| 上游 | ai-eval-service | 提交受控 eval model invocation |
| 上游 | knowledge-ingestion-service | 提交 chunk summary / metadata extraction / embedding 请求 |
| 同步依赖 | control-plane-service | provider route、budget、fallback、model allowlist 配置 |
| 同步依赖 | policy-service | tenant / action / data-class model invocation precheck |
| 同步依赖 | external providers | OpenAI-compatible、Claude-compatible、本地模型、embedding、rerank |
| 异步下游 | audit-service / ai-eval-service | 低敏 model invocation events |
| 事实源 | PostgreSQL | invocation metadata、budget window、provider route snapshot、outbox |

RAG / summary / Agent 仍负责业务语义和 EvidencePack 约束。`model-gateway` 只提供受控
模型调用通道。

## 3. 六层 DDD 包结构

```text
services/model-gateway/
  cmd/model-gateway/
  internal/{api,app,domain,infrastructure,types,trigger}/
```

| 层 | 本服务内容 |
| --- | --- |
| `api` | gRPC / HTTP adapter，verified service metadata，稳定错误映射 |
| `app` | InvokeTextGeneration、CreateEmbedding、RerankEvidence、GetModelInvocation |
| `domain` | provider route、budget、request class、safety envelope、failure class |
| `infrastructure` | PostgreSQL repository、provider HTTP clients、control-plane / policy clients |
| `types` | command、DTO、错误码、枚举、低敏 metadata |
| `trigger` | budget reset worker、outbox relay、route refresh worker、cleanup worker |

## 4. 领域模型

| 模型 | 说明 | 不变量 |
| --- | --- | --- |
| `ModelInvocation` | 一次模型调用元数据 | 不保存 raw prompt / raw output |
| `ProviderRoute` | tenant / use-case / model class 路由 | 只能引用 allowlisted provider |
| `BudgetWindow` | tenant / provider / model 的预算窗口 | 超额 fail closed 或降级 |
| `ProviderFailure` | provider 失败分类 | 保存低敏 failure class，不保存原始 body |
| `ModelOutboxEvent` | 低敏调用事件 | 只通过 outbox relay 发布 |

调用类型：

```text
TEXT_GENERATION
EMBEDDING
RERANK
CLASSIFICATION
EXTRACTION
EVAL_JUDGE
```

数据分类：

```text
LOW_SENSITIVE
BUSINESS_INTERNAL
USER_CONTENT
SECURITY_SENSITIVE
```

第一版只允许调用方显式声明数据分类；后续可接入 classifier / DLP。

## 5. 同步 API 契约

```text
rpc InvokeTextGeneration(InvokeTextGenerationRequest) returns (InvokeTextGenerationResponse)
rpc CreateEmbedding(CreateEmbeddingRequest) returns (CreateEmbeddingResponse)
rpc RerankEvidence(RerankEvidenceRequest) returns (RerankEvidenceResponse)
rpc GetModelInvocation(GetModelInvocationRequest) returns (GetModelInvocationResponse)
```

`InvokeTextGeneration` 请求字段：

```text
tenant_id, caller_service, caller_use_case
request_id, idempotency_key
model_class, preferred_model, route_policy
data_class, safety_policy
prompt_parts[], prompt_hash, prompt_schema_version
evidence_pack_ref, citation_required
max_output_tokens, temperature, timeout_ms
correlation_id, causation_id, trace_id
```

`prompt_parts[]` 是内存内 provider 请求材料；默认不持久化。调用方必须同时提供
`prompt_hash` 和低敏 `evidence_pack_ref`，用于审计和排障。

`InvokeTextGeneration` 响应字段：

```text
invocation_id, provider_id, model_id
output_text
output_hash, output_schema_version
token_usage, cost_estimate
failure_class, fallback_used
provider_latency_ms
```

响应中的 `output_text` 只返回给同步调用方，不进入 `model_invocations` 或 Kafka 事件。

`CreateEmbedding` 请求字段：

```text
tenant_id, caller_service, caller_use_case
input_chunks[], input_hashes[]
embedding_model_class, vector_dimension
data_class, timeout_ms
```

响应字段：

```text
invocation_id, embedding_vectors[], model_id, token_usage, cost_estimate
```

`RerankEvidence` 请求字段：

```text
tenant_id, caller_service, query_hash
evidence_item_refs[], evidence_item_texts[]
rerank_policy, top_k, timeout_ms
```

响应字段：

```text
invocation_id, ranked_item_refs[], scores[], model_id, fallback_used
```

错误码：

| 错误码 | 语义 | 是否可重试 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | model class、prompt schema、token budget、route policy 非法 | 否 |
| `PERMISSION_DENIED` | policy precheck 或数据分类不允许 | 否 |
| `RESOURCE_EXHAUSTED` | tenant budget、rate limit 或 token quota 超限 | 是，带退避 |
| `FAILED_PRECONDITION` | provider / model 未 allowlist，citation-required 不满足 | 否 |
| `UNAVAILABLE` | provider、control-plane、policy 或存储暂不可用 | 是 |
| `DEADLINE_EXCEEDED` | provider 超时 | 是 |

## 6. Provider Route 和预算

Route 选择顺序：

```text
request route_policy
-> control-plane route snapshot
-> tenant / data_class / use_case allowlist
-> provider health / circuit breaker
-> budget window
-> fallback chain
```

预算维度：

```text
tenant_id
caller_service
caller_use_case
provider_id
model_id
data_class
window_start
```

预算指标：

```text
request_count
input_tokens
output_tokens
embedding_tokens
estimated_cost_microunits
failure_count
fallback_count
```

超额策略：

| 场景 | 策略 |
| --- | --- |
| optional generation | fallback extractive / local provider |
| required generation | `RESOURCE_EXHAUSTED` |
| embedding pipeline | pause ingestion / retry later |
| eval judge | mark eval as blocked / skipped |

## 7. 异步事件契约

| 事件 | Topic | 分区键 | 说明 |
| --- | --- | --- | --- |
| `model.invocation.completed.v1` | `im.model.events` | `tenant_id:invocation_id` | 模型调用成功 |
| `model.invocation.failed.v1` | `im.model.events` | `tenant_id:invocation_id` | 模型调用失败 |
| `model.budget.exhausted.v1` | `im.model.events` | `tenant_id:budget_key` | 预算耗尽 |
| `model.provider.circuit_opened.v1` | `im.model.events` | `provider_id` | provider 熔断 |

事件 payload 只包含 invocation id、caller service、use case、provider / model id、data class、
token usage、cost estimate、latency、failure class、fallback flag、hash refs 和 trace refs。
禁止包含 raw prompt、raw output、EvidencePack 正文、provider body、secret、tenant PII 或用户内容。

## 8. 数据库设计

第一版表：

```text
model_invocations
model_budget_windows
model_provider_route_snapshots
model_provider_failures
model_outbox
```

关键字段：

```text
model_invocations:
tenant_id, invocation_id, idempotency_key, caller_service, caller_use_case,
request_type, data_class, provider_id, model_id, route_version,
prompt_hash, output_hash, evidence_pack_ref_hash,
input_tokens, output_tokens, estimated_cost_microunits,
status, failure_class, fallback_used, latency_ms,
correlation_id, causation_id, trace_id, created_at, completed_at

model_budget_windows:
tenant_id, budget_key, provider_id, model_id, caller_service, caller_use_case,
window_start, window_end, request_count, token_count, estimated_cost_microunits

model_provider_route_snapshots:
tenant_id, route_version, caller_service, caller_use_case,
provider_id, model_id, fallback_chain_json, policy_hash, status

model_provider_failures:
tenant_id, provider_id, model_id, failure_class, failure_count,
circuit_state, last_failure_at, next_probe_at
```

`model_invocations` 不包含 prompt / output 文本列。后续若确实需要 eval artifact，必须新增
低敏 artifact contract，并由 ai-eval-service 或 audit-service 明确拥有 retention。

## 9. 核心流程

Text generation：

```text
InvokeTextGeneration
-> verify service metadata
-> policy-service model invocation precheck
-> load route from control-plane snapshot
-> check tenant budget / rate limit
-> apply prompt safety envelope and max token guard
-> call provider with timeout
-> classify provider response / failure
-> record model_invocations metadata
-> write model.invocation.completed/failed outbox
-> return output_text only to caller
```

Embedding：

```text
CreateEmbedding
-> validate chunk count / dimension / data class
-> policy + budget check
-> call embedding provider
-> record hashes / token usage / cost
-> return vectors to caller
```

Rerank：

```text
RerankEvidence
-> validate evidence refs came from retrieval boundary
-> policy + budget check
-> call rerank provider or deterministic fallback
-> return ranked refs / scores
```

## 10. 与 RAG / Summary / Agent 的边界

RAG / summary / Agent 必须继续遵守：

```text
retrieval-gateway -> EvidencePack -> prompt builder -> model-gateway
-> caller verifier -> caller response / proposal
```

`model-gateway` 不做：

- EvidencePack 生成或过滤。
- citation verifier。
- stale / superseded memory 判断。
- Agent proposal status 决策。
- tool policy、approval、execution 或 audit。
- 用户可见答案的最终安全判断。

调用方必须在收到模型输出后执行：

```text
citation verifier
schema verifier
allowed output class verifier
fallback / refusal policy
low-sensitive audit linkage
```

## 11. 安全边界

- API 只接受 service identity 或 gateway verified metadata。
- provider secret 必须来自 secret manager / env ref；不得进入 DB、events、metrics 或 logs。
- provider allowlist 默认关闭公网自由 URL；本地 HTTP provider 只允许 loopback / private allowlist。
- provider request / response body 不进入 `model_invocations`、events、debug metrics 或 error message。
- 错误消息使用稳定 public message；provider body 和 raw error 只允许在本地 debug trace 中按配置采样并脱敏。
- `data_class=SECURITY_SENSITIVE` 默认禁止外部 provider，除非 route policy 显式 allow。
- `USER_CONTENT` 默认要求 caller 提供 EvidencePack / source refs / retention policy。
- metrics 禁止输出 tenant_id、user_id、conversation_id、message_id、prompt hash、output hash 或 provider body。

## 12. 幂等、重试和 fallback

| 场景 | 幂等键 | 重试策略 | fallback |
| --- | --- | --- | --- |
| Text generation | caller + idempotency_key | provider timeout / 5xx bounded retry | extractive / local model / fail closed |
| Embedding | chunk hash + model_id | retry with same chunk hashes | pause ingestion |
| Rerank | query hash + evidence refs | bounded retry | deterministic BM25 / retrieval score |
| OutboxRelay | event_id | bounded retry + DLQ | repair operator |

Kafka publish 仍通过 outbox relay；模型 provider 调用不是 Kafka exactly-once 事务的一部分。调用方需要按
`invocation_id` 和 `output_hash` 幂等处理。

## 13. Python Worker / 本地模型关系

Python Worker 可以继续做算法候选、eval 和模型侧 experimental runner，但不能绕过：

```text
Go caller service -> model-gateway -> provider / local model
```

允许模式：

- Go 服务调用 model-gateway，model-gateway 调 provider。
- Go 服务调用 Python Worker 生成 candidate，但 candidate 仍必须经过 Go verifier。
- Python Worker 调本地模型只用于离线 eval / experiment，并通过 ai-eval-service 记录低敏结果。

禁止模式：

- Python Worker 直接读取 search / memory / message / conversation 私表。
- Python Worker 直接执行 Agent tool action。
- Python Worker 把 raw prompt / raw output 写入通用 audit / event。

## 14. SLO 和指标

第一阶段不写生产 SLO，只保留本地 / 面试展示指标：

```text
model_invocation_total{caller_service,request_type,status,failure_class}
model_provider_latency_ms{provider_id,model_id,request_type}
model_token_total{provider_id,model_id,request_type}
model_cost_estimate_microunits_total{provider_id,model_id}
model_budget_exhausted_total{caller_service,request_type}
model_fallback_total{caller_service,request_type,from_provider,to_provider}
model_outbox_total{status}
```

metrics label 禁止使用 tenant_id、user_id、conversation_id、message_id、invocation_id、
prompt_hash、output_hash、provider URL、provider body 或 request_id。

## 15. 测试方案

| 测试 | 目标 |
| --- | --- |
| domain unit | route selection、budget exceed、fallback chain、failure class |
| app unit | policy deny、budget deny、provider timeout、idempotency |
| provider adapter | OpenAI-compatible / local HTTP response parsing、malformed fail closed |
| PostgreSQL integration | invocation metadata + budget window + outbox 同事务 |
| event builder | 不输出 prompt / output / provider body |
| smoke | RAG/Summary fake prompt -> model-gateway mock provider -> caller verifier |
| negative smoke | public URL disallowed、provider body redaction、budget exhausted fail closed |

## 16. Runbook

运行模式：

```text
NEXUSIM_MODEL_GATEWAY_MODE=grpc
NEXUSIM_MODEL_GATEWAY_MODE=outbox-relay
NEXUSIM_MODEL_GATEWAY_MODE=budget-reset-worker
NEXUSIM_MODEL_GATEWAY_MODE=route-refresh-worker
NEXUSIM_MODEL_GATEWAY_MODE=cleanup
```

provider config 第一版：

```text
NEXUSIM_MODEL_PROVIDER_CONFIG=file://...
NEXUSIM_MODEL_PROVIDER_SECRET_REF=...
NEXUSIM_MODEL_PROVIDER_ALLOW_PUBLIC=false
NEXUSIM_MODEL_PROVIDER_TIMEOUT=10s
```

operator：

```text
model-invocation-audit
model-budget-audit
model-provider-failure-audit
model-outbox-repair
```

## 17. 验收标准

进入编码前：

- 本 SDD v0.1 draft 被复核，无 P0/P1。
- `model-gateway` brief 指向本 SDD。
- 明确 model-gateway 不拥有 EvidencePack、prompt truth、Agent approval 或 raw prompt/output retention。

进入 first smoke 前：

- proto / migration / 六层 skeleton / cmd runtime 已落。
- provider allowlist、budget、timeout、fallback 和 raw prompt/output 不落库测试通过。
- RAG 或 Summary 通过 model-gateway mock provider 的 focused smoke 通过。
- provider failure 不污染 caller citation verifier，不泄露 provider body。
