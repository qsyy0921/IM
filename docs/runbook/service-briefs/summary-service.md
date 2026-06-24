# summary-service Brief

状态：foundation-active / EvidencePack-grounded summary first path。

## 已落

- 只读摘要 first path，输入只接受 retrieval-gateway EvidencePack。
- Citation verifier、grounded-summary anchor gate、guarded external HTTP LLM boundary。
- Memory graph edges、profile aggregate evidence 和 ref-only evidence audit 语义。
- 与 RAG safety gate 共享低敏 provider / output safety 策略。
- EvidencePack safety gate：生成用 text evidence 必须带 `source_ref`，
  search/memory evidence 必须带 `visibility_version`，memory/profile evidence
  必须满足 active/approved；citation verifier 不使用顶层字段兜底。

## 边界

- 不直接读 message / conversation / memory / search 私表。
- 无 evidence、citation 不匹配或 unsafe output 必须 fail-closed。
- missing source_ref、missing visibility version、superseded / unapproved memory 必须
  在 provider 调用前 fail-closed。
- 不保存 raw provider body、raw summary 或敏感 prompt。

## 下一步

- 扩展多会话摘要、未读摘要、时间版本冲突、citation regression 和 unsafe output cases。
