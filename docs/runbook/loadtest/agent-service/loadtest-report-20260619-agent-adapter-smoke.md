# agent-service Adapter Smoke Report

日期：2026-06-19

范围：真实本地 `retrieval-gateway -> policy-service -> agent-service`
adapter smoke。原始结果保存在
`H:\NexusIM\loadtest-results\agent-adapter-smoke-20260619-021621`。

## 运行方式

本轮启动了临时本地进程：

- `search-service grpc`：`127.0.0.1:10570`
- `memory-service grpc`：`127.0.0.1:10580`
- `policy-service grpc`：`127.0.0.1:18080`
  - `NEXUSIM_POLICY_TOOL_ALLOWED=true`
  - `NEXUSIM_POLICY_TOOL_REQUIRES_APPROVAL=true`
  - `NEXUSIM_POLICY_TOOL_PERMISSION_VERSION=1`
  - `NEXUSIM_POLICY_TOOL_CLASSIFICATION=LOW_RISK_APPROVAL_REQUIRED`
- `retrieval-gateway grpc`：`127.0.0.1:10590`
- `agent-service grpc`：`127.0.0.1:10630`

执行入口：

```powershell
.\tools\run-agent-adapter-smoke.ps1 `
  -PGDSN "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -AgentTarget "127.0.0.1:10630" `
  -ResultRoot "H:\NexusIM\loadtest-results" `
  -RunName "agent-adapter-smoke-20260619-021621" `
  -RequestTimeout "15s"
```

## 关键结果

```text
proposal_status=PROPOSED
requires_approval=true
generated_by_llm=false
policy_allowed=true
policy_requires_approval=true
policy_decision_source=STATIC_DEFAULT
policy_classification=LOW_RISK_APPROVAL_REQUIRED
policy_permission_version=1
evidence_item_count=2
search_item_count=1
memory_item_count=1
citation_count=2
search_projection_version=41
memory_projection_version=43
retrieval_version=retrieval-gateway.v1
agent_version=agent-service.v1
```

## 已验证不变量

- `agent-service` 先通过 `policy-service.CheckToolAction` 观察到 allow decision。
- `agent-service` 随后通过 `retrieval-gateway` 获取 `EvidencePack`。
- `EvidencePack` 同时包含 search message evidence 和 memory event evidence。
- proposal citation 可追踪到 seeded message / memory source ref。
- 第一版 response 保持 proposal-only，明确未执行业务 mutation。
- 第一版未接外部 LLM，`generated_by_llm=false`。

## 边界

- 本轮是本地小规模 adapter smoke，不是容量测试。
- policy 使用静态 tool allow 配置，不代表 provider-grade tool policy operator 已完成。
- agent-service 仍未持久化 proposal，也未接 approval / executor / audit outbox。
- 后续进入 `skill-registry`、`mcp-gateway` 和 `action-executor`。
