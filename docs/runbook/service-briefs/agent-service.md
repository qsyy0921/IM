# agent-service Brief

状态：foundation-active / proposal-only Agent control plane。

## 已落

- Agent proposal 创建、查询、审批前后状态流转。
- mcp-gateway prepare、skill metadata、policy precheck 和 approval workflow 串联。
- Planner Python candidate guard：Python 只产候选，Go 拥有 proposal / approval 状态。
- EvidencePack memory graph edges、profile aggregate evidence 和 RAG-Agent demo path。
- Approval outbox relay 与 low-sensitive report / eval adapter。
- EvidencePack-backed business proposal -> approval -> action-executor demo path，
  已覆盖 conversation note / profile mutation 通过公开 conversation-service API 执行。

## 边界

- Agent 不直接写业务库，不直接绕过 retrieval-gateway 读取私表。
- 高风险动作必须 proposal -> approval -> action-executor -> audit。
- 低风险 allowlist 动作也必须经过 skill / policy / idempotency / audit。
- 不保存 raw prompt / provider body / secret；只保存 hash、引用和低敏状态。

## 下一步

- 扩展 proposal risk policy、instruction approval UI 和更多真实业务 proposal 场景。
