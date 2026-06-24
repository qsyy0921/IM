# NexusIM RAG-Agent note + profile mutation service-stack gate

日期：2026-06-25

范围：本地真实服务栈 first-stage gate。该报告验证 RAG / Agent / approval /
action-executor 链路可以在显式 execute-mode 下执行两个 conversation 业务 mutation：

1. `conversation.note.create` 写入真实 conversation note fact。
2. `conversation.profile.update` 通过 conversation-service public API 更新会话资料。

这不是容量测试、HA 测试、provider-grade 运维验证或生产 SLO。

## 运行前置

- 重建本地 runtime image：
  - `nexusim/action-executor:local`
  - `nexusim/workflow-service:local`
- 通过 local compose 只刷新：
  - `nexusim-action-executor-grpc`
  - `nexusim-workflow-service-grpc`
- `action-executor` 环境包含：
  - `NEXUSIM_ACTION_EXECUTOR_CONVERSATION_GRPC_ADDR=conversation-service-grpc:10496`

## 命令

```powershell
. .\tools\go-env.ps1
.\tools\run-ai-eval-service-stack-gate-smoke.ps1 `
  -OptionalAdapter rag-agent-demo `
  -MemoryTarget 127.0.0.1:10580 `
  -RAGTarget 127.0.0.1:10610 `
  -AgentTarget 127.0.0.1:10630 `
  -ActionExecutorTarget 127.0.0.1:10660 `
  -WorkflowTarget 127.0.0.1:10750 `
  -ConversationTarget 127.0.0.1:10496 `
  -ResultRoot H:\NexusIM\loadtest-results `
  -RunName ai-eval-service-stack-live-20260625-rag-agent-note-profile-mutation `
  -RequestTimeout 10s `
  -ExpectBusinessActionExecuted
```

## 结果

主 summary：

```text
H:\NexusIM\loadtest-results\ai-eval-service-stack-live-20260625-rag-agent-note-profile-mutation\ai-eval-regression-gate-summary.json
```

RAG-Agent demo summary：

```text
H:\NexusIM\loadtest-results\ai-eval-service-stack-live-20260625-rag-agent-note-profile-mutation-rag-agent-demo\rag-agent-demo-summary.json
```

关键结果：

```text
status = passed
adapter_count = 4
case_count = 27
passed_count = 27
failed_count = 0
skipped_count = 0
selected_optional_adapters = rag-agent-demo
```

RAG-Agent demo 关键断言：

```text
business_action_executed = true
business_note_persisted = true
business_profile_updated = true
business_profile_version = 2
business_proposal_evidence_memory_count = 3
business_proposal_cross_group_source_ref_count = 3
profile_repair_negative_cases_verified = true
profile_repair_executed = true
group_memory_answer_verified = true
group_memory_proposal_verified = true
```

## 已验证不变量

- RAG answer、Agent proposal、approval 和 action-executor execution 都围绕同一
  tenant / conversation。
- EvidencePack 保留 cross-group source refs、speaker attribution、memory graph edge
  和 profile aggregate evidence。
- Agent 写动作仍必须通过 policy、skill contract、proposal、approval 和
  action-executor，不由 agent-service 直接执行业务写入。
- `conversation.note.create` 通过 conversation-service 写真实 note fact，并验证
  note fact 与 proposal / approval 绑定一致。
- `conversation.profile.update` 通过 conversation-service public
  `UpdateConversationProfile` 更新 profile，再用 public `GetConversationProfile` 读回验证。
- profile mutation 的 tool output 只保留 profile version 与 title / avatar /
  announcement hash，不回显 raw title、avatar URI 或 announcement。
- profile repair negative gate 仍 fail-closed：未审批 workflow 和 payload hash
  mismatch 都不会执行 repair。

## 本轮修正

`loadtest/ragagent` 的 business proposal seed 改为 run-specific objective 和
run-specific memory fact text。这样 repeated local runs 不会被历史 ACTIVE memory
干扰；gate 仍要求本轮三条 `DECISION` / `TASK` / `STATUS` reviewed memory 全部进入
EvidencePack。

## 限制

- 这是本地 Docker / PostgreSQL / Kafka / Redis runtime 上的单机服务栈验证。
- 不验证容量、长稳、跨机网络分区、provider-grade redrive 或外部 MCP provider。
- 不声明任何裸业务 mutation 可绕过 Agent approval；本轮只验证显式
  `-ExpectBusinessActionExecuted` execute-mode gate。
