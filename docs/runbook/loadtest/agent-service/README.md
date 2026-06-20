# agent-service Loadtest Runbook

本目录只保存低敏 smoke / loadtest 报告。原始运行结果放在
`H:\NexusIM\loadtest-results`，不要把原始结果复制进仓库。

## 当前报告

- `loadtest-report-20260619-agent-mcp-adapter-smoke.md`：真实本地
  `retrieval-gateway -> agent-service -> mcp-gateway` adapter smoke。
- `loadtest-report-20260619-agent-adapter-smoke.md`：真实本地
  `retrieval-gateway -> policy-service -> agent-service` adapter smoke。
  这是历史报告。
- `loadtest-report-20260620-agent-cross-temporal-stack-smoke.md`：真实本地
  Agent proposal service-stack smoke，验证跨群 source refs / speaker
  attribution、expired / superseded / future memory exclusion 和 proposal-only
  边界。

## 运行入口

```powershell
.\tools\run-agent-adapter-smoke.ps1 `
  -PGDSN "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -AgentTarget "127.0.0.1:10630"
```

该入口要求以下 runtime 已启动：

- `retrieval-gateway` `grpc` mode，默认 `127.0.0.1:10590`
- `skill-registry` `grpc` mode，默认 `127.0.0.1:10640`
- `policy-service` `grpc` mode，默认 `127.0.0.1:10800`
- `mcp-gateway` `grpc` mode，默认 `127.0.0.1:10650`
- `agent-service` `grpc` mode，默认 `127.0.0.1:10630`

runner 会在 PostgreSQL 中 seed 低敏测试数据：

- `skill_registry_definitions`：`conversation.note.create` active skill，
  allowed action 为 `TOOL_ACTION_CALL`，requires approval。
- `policy_tool_action_rules`：允许 `CALL`，requires approval，
  permission version 固定为 `42`。
- search / memory projection rows：用于 retrieval-gateway 生成 EvidencePack。

runner 会应用所需 expand-only migration 并清理本轮 tenant 数据；原始 summary
仍写到 `H:\NexusIM\loadtest-results`。

## 验证点

- agent-service 调用 mcp-gateway `PrepareToolCall`，并返回 `skill_id` /
  `prepared_audit_id`。
- mcp-gateway 通过 skill-registry 校验 skill / tool / action contract。
- mcp-gateway 通过 policy-service `CheckToolAction` 看到 allow decision。
- `mcp_gateway_tool_call_audits` 记录 `ALLOWED` prepare audit，且只存
  `input_sha256` 等低敏字段。
- agent-service 继续通过 retrieval-gateway 获取 `EvidencePack`。
- 返回 `PROPOSED`、`requires_approval=true`、`generated_by_llm=false`。
- proposal 带 citation，citation 可追踪到 seeded message / memory source ref。
- 第一版不执行任何业务 mutation。
