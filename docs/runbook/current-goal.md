# NexusIM Current Goal

本文件只写当前可执行目标。完整架构见
`docs/architecture/target-architecture-complete.md`，未完成工作见
`docs/runbook/remaining-goals.md`，单服务事实见 `docs/runbook/service-briefs/`。

## Active Module

`action-executor` provider failure redrive first path 收口。

## 目标

- 提供 `RedriveProviderFailure` gRPC first path。
- 只允许 redrive 已进入 `DLQ` 的 provider failure。
- 要求 fresh proposal / approval / prepared audit，禁止复用 source failure 的旧
  proposal / approval / prepared audit。
- 要求 redrive command 的 skill / tool / resource 与 source failure 完全匹配。
- 要求调用方提交新的 `input_json` 和 64 位小写 hex `reason_sha256`。
- Redrive 必须复用 `ExecuteApprovedAction` 正常校验和执行链路。
- execution audit 必须记录 source provider failure id 和 reason hash。
- 不保存或恢复旧 raw input，不重放旧 provider output，不引入隐藏 fallback。

## 本轮完成条件

- Proto / gRPC / app / postgres / cmd wiring 完成。
- App、gRPC、真实 PostgreSQL integration 覆盖 redrive happy path 和 fail-closed cases。
- SDD、service brief、remaining goals、README 和架构文档同步公开能力变化。
- 相关 focused checks 通过；若涉及 proto / migration / 安全边界，提交前跑完整门禁或明确记录阻塞原因。
- 提交并推送到 GitHub。

## 非目标

- 不做 operator UI。
- 不做 provider-grade batch redrive。
- 不自动 provider replay。
- 不把 redrive 做成普通 tool action。
- 不扩大客户端、Docker、双机或 HA 测试范围。

## 后续优先级

1. AI / Agent demo path：EvidencePack、RAG / Summary safety、Agent proposal /
   approval / action execution、eval gate。
2. action-executor 后续 hardening：batch redrive、provider replay、operator UI、
   provider failure metrics。
3. Product-active 服务按需推进，不抢占 AI / Agent 演示主线。
