# rag-service SDD v0.1

`rag-service` 是 NexusIM AI 应用链路中的只读问答边界。它位于
`retrieval-gateway` 之后，消费已经完成 visibility / policy / temporal
version 过滤的 `EvidencePack`，并向客户端返回带引用的回答。

## 职责

- 对外提供 `AnswerQuestion`。
- 调用 `retrieval-gateway.RetrieveEvidence` 获取 EvidencePack。
- 可选 `at_conversation_seq` 显式传给 retrieval-gateway，用于固定 memory
  current-only 查询时点；未传时 retrieval-gateway 必须使用显式查询策略，不能从 search hit 隐式推断查询时点。
- 第一版通过 `AnswerProvider` port 生成 deterministic extractive answer；
  默认实现不调用外部 LLM provider。
- 可选 `external-http` provider mode 只作为第一阶段外部 LLM boundary：它仍走
  `AnswerProvider` port，prompt 只能由 EvidencePack 构造，HTTP 明文 endpoint
  只允许 loopback / private，provider failure 返回稳定 unavailable，unsafe /
  malformed output fail closed。
- 可选 `python-worker` provider mode 只作为第一阶段服务级 Python candidate
  guard：Go 先生成 grounded answer，Python worker 只返回 candidate hash /
  citations / confidence metadata；Go 校验 task/candidate id、hash 和 citations
  后才接受，失败时 fail closed。
- response 必须保留 `citations`、原始 `EvidencePack`、`rag_version` 和
  `generated_by_llm`。
- 无可见证据时返回 `INSUFFICIENT_EVIDENCE`，不能编造答案。
- provider 输出必须经过 citation verifier；引用无法匹配 EvidencePack 时
  fail closed，不返回 ungrounded answer。
- 生成用的 text evidence 必须携带至少一个可验证 `source_ref`。`citation`
  verifier 只接受命中 `source_ref` 的引用，不再用 evidence item 顶层
  `conversation_id / seq` 做兜底匹配。
- `SEARCH_MESSAGE` 和 `MEMORY_EVENT` evidence 必须携带正数
  `visibility_version`，否则视为 retrieval boundary 无法证明成员可见性，
  fail closed。
- `MEMORY_EVENT` evidence 必须是 `ACTIVE` 且已 review/approve；`SUPERSEDED`、
  `EXPIRED`、`PENDING` 或未审批 memory 不能进入回答生成。`PROFILE_AGGREGATE`
  若携带 temporal/review metadata，也必须满足 active/approved。

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
- 显式 `at_conversation_seq` 不能为负；传入后必须贯穿到 retrieval-gateway，
  防止 RAG 读取过期或 superseded memory。
- citations 必须可追踪到 EvidencePack `source_ref`；provider 输出后统一
  由 citation verifier 检查，不能靠 item 顶层字段补齐缺失引用。
- 高风险写动作属于后续 Agent / action-executor，不属于本服务。

## 后续

- 后续接 provider-specific LLM / Python worker 时继续复用 prompt boundary、
  token budget、PII / secret filter、provider failure fail-closed、hash / citation
  metadata 校验和 citation verifier。
- `summary-service` 复用 EvidencePack 和 citation verifier 语义。
