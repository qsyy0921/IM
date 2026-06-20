# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`，
不要把长 prompt 复制到这里。

## 当前 Active Slice

```text
future platform / product services promotion
```

用户已点名把 goal 指向全部待开发 future 微服务。当前目标不是只做
`media-service`，也不是一次性把全部服务目录铺开，而是把这批服务按统一边界推进到
可逐个实现的 promotion plan。

## 当前目标服务

```text
media / notification / audit / admin / control-plane / presence
model-gateway / workflow / knowledge-ingestion / vector-index
```

## 默认推进方式

1. 每轮先读 `prompt.md`、`agent.md`、本文件和相关 service brief。
2. 先冻结组合边界：哪些服务先做、哪些只保留 port / adapter、哪些需要 ADR。
   组合边界文档见 `docs/sdd/future-platform-services.md`。
3. 对每个服务按顺序推进：SDD v0.1 -> proto / migration -> 六层 skeleton
   -> cmd runtime -> Docker / Prometheus / Grafana -> focused smoke。
4. 第一组优先级建议：
   `media-service` -> `notification-service` -> `audit-service`
   -> `control-plane-service` -> `presence-service`
   -> `model-gateway` / `knowledge-ingestion-service` / `workflow-service`
   -> `vector-index-service`。
5. 只有完成对应服务 SDD v0.1 和门禁影响确认后，才把该服务从 `future`
   stage switch 到 active，并创建 `services/<service>`。

## 当前进展

- 组合 promotion 边界见 `docs/sdd/future-platform-services.md`。
- 10 个目标服务的 SDD v0.1 draft 已存在，单服务状态见 service brief。
- `media-service`、`notification-service`、`audit-service`、
  `control-plane-service`、`presence-service`、`model-gateway`、
  `knowledge-ingestion-service` 已 product-active 并通过各自第一版 focused
  checks / smoke。
- `workflow-service` 已从 stage-switch 进入 product-active 第一版 implementation
  slice，`CreateWorkflow`、`RecordWorkflowDecision`、`GetWorkflow` 最小路径已落地并
  通过 focused checks / 完整 `check-local`。

## 下一步

- `admin-service` 和 `vector-index-service` 已完成 stage-switch review；下一步可进入
  对应第一版 implementation slice。
- 默认继续 `vector-index-service` 第一版 implementation slice，用于闭合
  model-gateway / knowledge-ingestion / retrieval 的向量索引边界。
- 也可以改做 `admin-service` 第一版 implementation slice。
- 也可以继续 notification SMTP / SMS / APNs / FCM adapter 或 bounce-suppression。

## 硬边界

- 不一次性 promotion 全部 future 服务。
- 不把媒体二进制塞回 message-service。
- 不把 identity 局部 webhook / SMTP 扩成完整 notification-service 前的生产承诺。
- 不让 admin / control-plane / workflow 直接改其它服务私有表。
- 不让 model-gateway / vector-index / knowledge-ingestion 绕过 retrieval /
  policy / EvidencePack 边界。
- 不回滚用户已有修改。
- 小改跑 focused checks；涉及 service-registry / Docker / compose / proto /
  migration / 安全边界时再扩大门禁。

## 文档路由

- 当前阶段背景：`docs/runbook/current-brief.md`
- 剩余待办：`docs/runbook/remaining-goals.md`
- 服务入口：`docs/runbook/service-briefs/<service>.md`
- 新发现待办写入 `docs/runbook/remaining-goals.md`
