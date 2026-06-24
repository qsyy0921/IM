# RAG-Agent Business Mutation Execute Smoke Blocked - 2026-06-25

## 结论

Superseded by
`loadtest-report-20260625-rag-agent-business-mutation-execute-gate.md`:
Docker / PostgreSQL runtime 恢复后，v7 已通过完整 service-stack opt-in mutation smoke。

本轮没有归档为通过的 full smoke。`conversation.note.create` 显式业务 adapter、
RAG-Agent execute-mode gate 和 service-stack wrapper 已具备执行入口，但本机 live
运行在进入完整 RAG-Agent optional adapter 前被本地 Docker / PostgreSQL runtime
状态阻塞。

该报告只记录阻塞证据，不声明 `conversation.note.create` opt-in mutation full
service-stack smoke 已通过。

## 已确认成立

- `run-ai-eval-service-stack-gate-smoke.ps1` 支持 `-ExpectBusinessActionExecuted`。
- execute-mode preflight 会把 conversation-service gRPC endpoint 纳入必需检查。
- `run-ai-eval-regression-gate-smoke.ps1` 会把 execute-mode flag 传递给
  `rag-agent-demo` optional adapter。
- `loadtest/rag` 已补 `--setup-timeout`，避免 PostgreSQL migration / seed setup
  无限等待。
- `loadtest/ragagent` 已把 child process compact output 纳入错误文本，避免只返回
  “rerun child directly”。

## 本轮证据

### Preflight

命令：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -PreflightOnly `
  -AllowMissing `
  -ExpectBusinessActionExecuted `
  -RunName codex-ragagent-business-execute-preflight-afterpatch-20260625-033508 `
  -RequestTimeout 5s
```

结果：

```text
OK ai-eval service-stack preflight summary written:
H:\NexusIM\loadtest-results\codex-ragagent-business-execute-preflight-afterpatch-20260625-033508\ai-eval-service-stack-preflight-summary.json
```

该 preflight 覆盖了 `rag-service`、`agent-service`、`action-executor`、
`retrieval-gateway`、`search-service`、`memory-service`、`mcp-gateway`、
`skill-registry`、`policy-service`、`workflow-service`、`conversation-service`
和 PostgreSQL endpoint。

### Full Gate Attempts

`ai-eval-rag-agent-demo-live-20260625-business-mutation-execute-v1`：

```text
apply migration: context deadline exceeded
```

`ai-eval-rag-agent-demo-live-20260625-business-mutation-execute-v2` / `v3`
使用 `-NoApplyMigration` 后仍在 recorder path 失败：

```text
record eval run: rpc error: code = DeadlineExceeded desc = context deadline exceeded
```

### Direct RAG-Agent / RAG Child

direct RAG-Agent execute-mode run 现在会带出 child error：

```text
rag-agent demo failed: rag child run failed: exit status 1; output:
rag smoke failed: apply migrations\postgres\search\000001_search_core.sql:
context deadline exceeded
```

RAG child direct run 使用 `--setup-timeout 5s` 后也能快速暴露同一类 setup
阻塞，而不是长时间挂起。

### Runtime Observation

本轮 Docker CLI 查询容器时返回：

```text
request returned 500 Internal Server Error for API route ...
```

同时多个服务端口仍由 Docker backend / 本地服务进程监听，因此本机处于“端口可见但
Docker API / PostgreSQL migration path 不稳定”的状态。下一轮应先恢复本地 runtime
状态，再重跑 full smoke。

## 下一步

1. 恢复 Docker Desktop / PostgreSQL runtime，确认 Docker API 可用。
2. 清理或确认 search / ai-eval 相关 migration lock / long transaction 状态。
3. 重跑：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -ExpectBusinessActionExecuted `
  -NoApplyMigration `
  -RunName ai-eval-rag-agent-demo-live-<date>-business-mutation-execute `
  -RequestTimeout 120s
```

4. 通过后再新增正式 `loadtest-report-...-gate.md`，并更新 current-goal /
current-brief / remaining-goals。
