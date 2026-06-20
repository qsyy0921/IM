# NexusIM Current Brief

本文件是每轮低 token 入口，只回答“现在处于什么阶段、下一步去哪里看”。
不要在这里维护长历史或完整待办。

## 按需读取

- 具体执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务细节：先读 `docs/runbook/service-briefs/README.md`，再读对应服务 brief。
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/`
  或 `docs/runbook/history/`。

## 当前阶段

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端。
9 个后端服务已进入真实链路：`api-gateway`、`identity-service`、`message-service`、`conversation-service`、`delivery-service`、`push-gateway`、`receipt-service`、`contacts-service`、`policy-service`。

当前 active slice 已切到 `future platform / product services promotion`：

```text
future services -> SDD v0.1 -> stage-switch plan -> service-by-service skeleton
```

AI foundation-active 服务：`search-service`、`memory-service`、`retrieval-gateway`、`rag-service`、`summary-service`、`agent-service`、`skill-registry`、`mcp-gateway`、`action-executor`、`ai-eval-service`。

Go 侧服务底座、EvidencePack、proposal / approval / audit、Python Worker 候选接入边界和低敏 eval 持久化已经足够支撑算法切片；后续 Go 工作围绕候选接入、边界校验和状态流转。

当前下一步：

```text
future platform / product services 的 10 个 SDD draft 已存在；`media-service`、
`notification-service`、`audit-service`、`control-plane-service`、`presence-service`
和 `model-gateway` 已进入 product-active 并通过各自第一版 smoke。
`knowledge-ingestion-service` 已完成第一版 metadata + chunk manifest path；
`workflow-service` 已完成 `CreateWorkflow`、`RecordWorkflowDecision`、`GetWorkflow`
最小审批等待路径，并通过 focused checks / 完整 `check-local`。
`vector-index-service` 已完成第一版 `UpsertVectorItem` / `TombstoneVectorItem` /
`SearchVectors` / `GetVectorIndexJob` path。`admin-service` 已完成第一版
`CreateAdminOperation` / `ApproveAdminOperation` / `GetAdminOperation` /
`ListAdminOperations` path、`admin_outbox -> im.admin.events` outbox relay 和
`operation-worker` risk routing 执行闭环；`REPAIR_REQUEST` 已接入
workflow-service `REPAIR_APPROVAL`，其它 `CRITICAL` operation 已接入
workflow-service `ADMIN_OPERATION`，并已写入第一版 operation-specific approval
policy / target service；`loadtest/admin` operator CLI 已支持公开 gRPC approve /
reject / get / list。
下一步默认继续 admin 真实下游公开 admin API adapter / admin operation 真实进程
smoke，或继续 vector
embedding / rebuild / outbox 后续 worker。
```

系统测试 / HA / 长压 / sizing 后置；总览、待办、单服务状态分别看
`development-progress.md`、`remaining-goals.md`、`service-briefs/<service>.md`。

- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做模型 / 算法 / eval 候选层，Go 负责控制面、状态和审计。
- future 服务 promotion 期间不得一次性创建全部服务目录。
- 媒体、通知、审计、控制面、presence、model、workflow、ingestion、vector 等边界必须继续通过公开 API、事件或明确 port 串联。
- 不回滚用户已有修改。
- 新发现的待完成工作写入 `docs/runbook/remaining-goals.md`。
