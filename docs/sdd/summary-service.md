# summary-service SDD

## 目标

`summary-service` 是 NexusIM AI 应用底座中的只读摘要边界服务。第一版只
消费 `retrieval-gateway` 返回的权限过滤 `EvidencePack`，生成可引用的会话摘
要，不直接读 message / conversation / search / memory 私有表。

## 职责

- 对外提供 `GenerateConversationSummary`。
- 调用 `retrieval-gateway.RetrieveEvidence` 获取 EvidencePack。
- 可选 `at_conversation_seq` 显式传给 retrieval-gateway，用于固定 memory
  current-only 查询时点；未传时 retrieval-gateway 必须使用显式查询策略，不能从 search hit 隐式推断查询时点。
- 第一版通过 `SummaryProvider` port 生成 deterministic extractive summary；
  默认实现不调用外部 LLM provider。
- 可选 `external-http` provider mode 只作为第一阶段外部 LLM boundary：它仍走
  `SummaryProvider` port，prompt 只能由 EvidencePack 构造，HTTP 明文 endpoint
  只允许 loopback / private，provider failure 回退 extractive，unsafe /
  malformed output fail closed。
- 可选 `python-worker` provider mode 只作为第一阶段服务级 Python candidate
  guard：Go 先生成 grounded summary，Python worker 只返回 candidate hash /
  citations / confidence metadata；Go 校验 task/candidate id、hash 和 citations
  后才接受，失败时 fail closed。
- response 必须保留 `citations`、原始 `EvidencePack`、`summary_version` 和
  `generated_by_llm`。
- 无可见证据时返回 `INSUFFICIENT_EVIDENCE`，不能编造摘要。
- provider 输出必须经过 citation verifier；引用无法匹配 EvidencePack 时
  fail closed，不返回 ungrounded summary。

## 非职责

- 不直接读 message / conversation / delivery / search / memory 私有表。
- 不做 indexing、memory extraction、profile aggregate 或权限投影。
- 不执行 Agent 动作，不写业务事实，不发布 Kafka 事件。
- 不接 provider-specific SDK、prompt template registry、缓存或长期模型状态。

## 链路

```text
client / agent
-> summary-service.GenerateConversationSummary
-> retrieval-gateway.RetrieveEvidence
-> EvidencePack
-> SummaryProvider
-> citation verifier
-> grounded summary response
```

RAG / summary / Agent 后续能力只能沿用该 evidence boundary。任何新增 LLM
adapter 都必须在 SummaryProvider port 后面，不能绕过 retrieval-gateway 或
citation verifier。

## 安全边界

- `AuthContext` 优先使用 verified metadata，本地开发可用 request body。
- `retrieval-gateway` 失败时 fail closed，返回稳定 public error。
- `GenerateConversationSummary` 不返回没有 EvidencePack 支撑的事实。
- 显式 `at_conversation_seq` 不能为负；传入后必须贯穿到 retrieval-gateway，
  防止摘要读取过期或 superseded memory。
- citations 必须可追踪到 evidence item 或 source ref；provider 输出后统一
  由 citation verifier 检查。
- 高风险写动作属于后续 Agent / action-executor，不属于本服务。

## 后续

- 后续接 provider-specific LLM / Python worker 时继续复用 prompt boundary、
  token budget、PII / secret filter、provider failure fail-closed、hash / citation
  metadata 校验和 citation verifier。
- 与 `agent-service` 对接时仍只暴露 EvidencePack summary，不授予业务写权限。
