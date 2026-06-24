# rag-service Brief

状态：foundation-active / EvidencePack-grounded answer first path。

## 已落

- 只读问答 first path，输入只接受 retrieval-gateway EvidencePack。
- Citation verifier、grounded-answer anchor gate、guarded external HTTP LLM boundary。
- Memory graph edges、profile aggregate evidence 和 ref-only evidence audit 语义。
- RAG-Agent demo path：与 Agent proposal / approval / action audit 形成低敏报告。

## 边界

- 不直接读 message / conversation / memory / search 私表。
- 无 evidence 或 citation 不匹配必须 fail-closed / refuse。
- 不保存 raw provider body、raw answer 或敏感 prompt。
- Provider 调用只在显式配置下发生。

## 下一步

- 扩展 refusal、citation regression、unsafe output、multi-hop / temporal / profile eval cases。
