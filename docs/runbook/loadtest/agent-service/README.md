# agent-service Loadtest Runbook

本目录只保存低敏 smoke / loadtest 报告。原始运行结果放在
`H:\NexusIM\loadtest-results`，不要把原始结果复制进仓库。

## 当前报告

- `loadtest-report-20260619-agent-adapter-smoke.md`：真实本地
  `retrieval-gateway -> policy-service -> agent-service` adapter smoke。

## 运行入口

```powershell
.\tools\run-agent-adapter-smoke.ps1 `
  -PGDSN "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -AgentTarget "127.0.0.1:10630"
```

该入口要求以下 runtime 已启动：

- `retrieval-gateway` `grpc` mode，默认 `127.0.0.1:10590`
- `policy-service` `grpc` mode，默认 `127.0.0.1:10800`
- `agent-service` `grpc` mode，默认 `127.0.0.1:10630`

第一版建议用静态 tool policy 验证允许路径：

```powershell
$env:NEXUSIM_POLICY_TOOL_ALLOWED='true'
$env:NEXUSIM_POLICY_TOOL_REQUIRES_APPROVAL='true'
$env:NEXUSIM_POLICY_TOOL_PERMISSION_VERSION='1'
$env:NEXUSIM_POLICY_TOOL_CLASSIFICATION='LOW_RISK_APPROVAL_REQUIRED'
```

## 验证点

- agent-service 通过 policy-service `CheckToolAction` 看到 allow decision。
- agent-service 继续通过 retrieval-gateway 获取 `EvidencePack`。
- 返回 `PROPOSED`、`requires_approval=true`、`generated_by_llm=false`。
- proposal 带 citation，citation 可追踪到 seeded message / memory source ref。
- 第一版不执行任何业务 mutation。
