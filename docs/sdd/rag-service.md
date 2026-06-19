# rag-service SDD v0.1

`rag-service` 是 NexusIM AI 应用链路中的只读问答边界。它位于
`retrieval-gateway` 之后，消费已经完成 visibility / policy / temporal
version 过滤的 `EvidencePack`，并向客户端返回带引用的回答。

## 职责

- 对外提供 `AnswerQuestion`。
- 调用 `retrieval-gateway.RetrieveEvidence` 获取 EvidencePack。
- 第一版通过 `AnswerProvider` port 生成 deterministic extractive answer；
  默认实现不调用外部 LLM provider。
- 可选 `external-http` provider mode 只作为第一阶段外部 LLM boundary：它仍走
  `AnswerProvider` port，prompt 只能由 EvidencePack 构造，HTTP 明文 endpoint
  只允许 loopback / private，provider failure 回退 extractive，unsafe /
  malformed output fail closed。
- response 必须保留 `citations`、原始 `EvidencePack`、`rag_version` 和
  `generated_by_llm`。
- 无可见证据时返回 `INSUFFICIENT_EVIDENCE`，不能编造答案。
- provider 输出必须经过 citation verifier；引用无法匹配 EvidencePack 时
  fail closed，不返回 ungrounded answer。

## 非职责

- 不直接读 message / conversation / delivery / search / memory 私有表。
- 不做 indexing、memory extraction、profile aggregate 或权限投影。
- 不执行 Agent 动作，不写业务事实，不发布 Kafka 事件。
- 不接 provider-specific SDK、prompt template registry、缓存或长期模型状态。

## 链路

```text
client/api-gateway
-> rag-service AnswerQuestion
-> retrieval-gateway RetrieveEvidence
-> search-service / memory-service / policy-service
-> EvidencePack
-> extractive answer + citations
```

RAG / summary / Agent 后续能力只能沿用该 evidence boundary。任何新增 LLM
生成、rerank、tool call 或 action proposal 都不得绕过 retrieval-gateway。

## 安全不变量

- `AuthContext` 优先使用 verified metadata，本地开发可用 request body。
- `retrieval-gateway` 失败时 fail closed，返回稳定 public error。
- `AnswerQuestion` 不返回没有 EvidencePack 支撑的事实。
- citations 必须可追踪到 evidence item 或 source ref；provider 输出后统一
  由 citation verifier 检查。
- 高风险写动作属于后续 Agent / action-executor，不属于本服务。

## 后续

- 后续接 provider-specific LLM / Python worker 时继续复用 prompt boundary、
  token budget、PII / secret filter、provider failure fallback 和 citation
  verifier。
- `summary-service` 复用 EvidencePack 和 citation verifier 语义。
