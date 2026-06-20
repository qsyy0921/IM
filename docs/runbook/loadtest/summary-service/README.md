# summary-service Loadtest Runbook

本目录只保存低敏 smoke / loadtest 报告。原始运行结果放在
`H:\NexusIM\loadtest-results`，不要把原始结果复制进仓库。

## 当前报告

- `loadtest-report-20260619-summary-adapter-smoke.md`：真实本地
  `retrieval-gateway -> summary-service` adapter smoke。
- `loadtest-report-20260620-summary-cross-temporal-stack-smoke.md`：真实本地
  Summary service-stack smoke，验证跨群 source refs / speaker attribution 和
  expired / superseded / future memory exclusion。

## 运行入口

```powershell
.\tools\run-summary-adapter-smoke.ps1 `
  -PGDSN "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -SummaryTarget "127.0.0.1:10620"
```

该入口要求 search-service、memory-service、retrieval-gateway 和
summary-service 的 `grpc` runtime 已启动。
