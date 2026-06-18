# rag-service Loadtest Runbook

本目录只保存低敏 smoke / loadtest 报告。原始运行结果放在
`H:\NexusIM\loadtest-results`，不要把原始结果复制进仓库。

## 当前报告

- `loadtest-report-20260619-rag-adapter-smoke.md`：真实本地
  `retrieval-gateway -> rag-service` RAG adapter smoke。

## 运行入口

```powershell
.\tools\run-ai-eval-rag-adapter.ps1 `
  -PGDSN "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable" `
  -RAGTarget "127.0.0.1:10610"
```

该入口要求 search-service、memory-service、retrieval-gateway 和
rag-service 的 `grpc` runtime 已启动。
