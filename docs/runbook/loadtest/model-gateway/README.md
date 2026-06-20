# model-gateway smoke

本目录记录 `model-gateway` 本地小规模 smoke。原始 summary / log 写入
`H:\NexusIM\loadtest-results`，仓库只保存说明和报告。

## 最小 gRPC smoke

脚本：

```powershell
.\loadtest\modelgateway\run-local-smoke.ps1
```

默认行为：

1. 构建 `services/model-gateway/cmd/model-gateway` 和 runner。
2. 启动 `NEXUSIM_MODEL_GATEWAY_MODE=grpc` 真实进程。
3. runner 通过真实 gRPC 调用：
   `InvokeTextGeneration -> replay -> GetModelInvocation`。
4. runner 查询 PostgreSQL，确认：
   - `InvokeTextGeneration` 使用 allowlisted deterministic mock provider；
   - `idempotency_key` replay 返回同一 invocation，不再次返回 raw output；
   - `GetModelInvocation` 只返回低敏 metadata；
   - `model_invocations` / `model_outbox` 只保存 hash refs、token/cost/latency
     和 trace refs，不保存 raw prompt、raw model output、provider body、secret。

边界：

- 这是 PostgreSQL + gRPC 的最小本地 smoke，不验证 OpenAI / Claude / 本地模型
  HTTP provider、embedding、rerank、outbox relay、route-refresh worker 或预算重置 worker。
- RAG / summary / Agent 仍负责 prompt builder、EvidencePack、citation verifier 和
  action approval；`model-gateway` 只提供受控模型调用通道。

## 已归档报告

- `loadtest-report-20260620-model-gateway-grpc-smoke.md`
