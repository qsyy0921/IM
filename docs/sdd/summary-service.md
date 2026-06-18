# summary-service SDD

## 目标

`summary-service` 是 NexusIM AI 应用底座中的只读摘要边界服务。第一版只
消费 `retrieval-gateway` 返回的权限过滤 `EvidencePack`，生成可引用的会话摘
要，不直接读 message / conversation / search / memory 私有表。

## 职责

- 对外提供 `GenerateConversationSummary`。
- 调用 `retrieval-gateway.RetrieveEvidence` 获取 EvidencePack。
- 第一版通过 `SummaryProvider` port 生成 deterministic extractive summary；
  默认实现不调用外部 LLM provider。
- response 必须保留 `citations`、原始 `EvidencePack`、`summary_version` 和
  `generated_by_llm=false`。
- 无可见证据时返回 `INSUFFICIENT_EVIDENCE`，不能编造摘要。
- provider 输出必须经过 citation verifier；引用无法匹配 EvidencePack 时
  fail closed，不返回 ungrounded summary。

## 非职责

- 不直接读 message / conversation / delivery / search / memory 私有表。
- 不做 indexing、memory extraction、profile aggregate 或权限投影。
- 不执行 Agent 动作，不写业务事实，不发布 Kafka 事件。
- 不在第一版接外部 LLM adapter、prompt template registry 或缓存。

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
- citations 必须可追踪到 evidence item 或 source ref；provider 输出后统一
  由 citation verifier 检查。
- 高风险写动作属于后续 Agent / action-executor，不属于本服务。

## 后续

- 外部 LLM adapter 接入时增加 prompt boundary、token budget、PII / secret
  filter 和 provider failure fallback。
- 与 `agent-service` 对接时仍只暴露 EvidencePack summary，不授予业务写权限。
