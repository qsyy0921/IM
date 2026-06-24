# action-executor Brief

状态：foundation-active / approved execution audit + guarded adapters + conversation business adapters.

## 已落

- `ExecuteApprovedAction` gRPC。
- 强制 `proposal_id`、`approval_id`、`prepared_audit_id`。
- 调 `agent-service.VerifyApprovedAgentProposal` 校验 approval / prepare audit / skill / tool / resource。
- 调 `skill-registry.GetSkill` 校验 active skill、tool match、`EXECUTE` action。
- 调 `policy-service.CheckToolAction(action=EXECUTE)` 做执行前 precheck。
- PostgreSQL `action_executor_execution_audits` 和 `action_executor_tool_results`。
- 本地安全 `nexusim.local.echo`：仅 `LOW` risk，deterministic 低敏输出，不回显 raw input。
- 外部 MCP failure：默认关闭；显式开启后返回稳定低敏失败分类，不落 provider 原文。
- 外部 HTTP provider adapter：默认关闭；显式 `http` mode + allowlist + `LOW` risk 才执行，只发送 tool metadata / input hash，provider output 继续走 safety gate 和 output hash projection。
- Conversation note business adapter：默认关闭；显式配置
  `NEXUSIM_ACTION_EXECUTOR_CONVERSATION_GRPC_ADDR` 后，`conversation.note.create`
  会通过 conversation-service 公开 gRPC `CreateConversationNote` 写入真实会话 note
  fact；只接受 `LOW` risk、`resource_type=conversation`、`EXECUTE` skill contract，
  output 只返回 note ref / id / hash metadata，不回显 note body。
- Conversation profile business adapter：默认关闭；复用
  `NEXUSIM_ACTION_EXECUTOR_CONVERSATION_GRPC_ADDR` 后，`conversation.profile.update`
  会通过 conversation-service 公开 gRPC `UpdateConversationProfile` 更新会话资料；
  只接受 `LOW` risk、`resource_type=conversation`、`EXECUTE` skill contract，并要求
  `expected_profile_version > 0`。output 只返回 conversation ref、profile version 和
  title / avatar / announcement hash，不回显 raw profile 字段。
- Tool output safety：malformed / oversize / secret-like / PII-like output fail closed，不入 hash。
- Docker / Prometheus / Grafana wiring、聚焦测试、PG integration、Agent execution eval adapter、external HTTP adapter eval / failure smoke、preflight safety eval。
- Action rate-limit / repair-DLQ safety：rate-limited action 在 tool execution 前 `BLOCKED`；limiter unavailable fail closed 为 `FAILED`；repair / DLQ action 需 operator workflow，不进通用 adapter。
- Provider failure lifecycle：timeout / unavailable / rate-limit -> `RETRY_PENDING`；permission denied / unsafe / generic failure -> `DLQ`；worker 只做 bounded retry bookkeeping / DLQ，不重放 tool。
- Provider failure audit / redrive-plan operator handoff：`provider-failure-audit`
  只读自有失败投影；`provider-failure-redrive-plan` 必须显式 dry-run 和 reason file，
  输出低敏审批 artifact，并挂入 `repair-operators.catalog.json`。该 plan 不修改
  provider failure row、不调用 tool executor、不重放 provider output。
- 2026-06-24 `loadtest/ragagent` 已提供 RAG-Agent demo first path：通过既有
  Agent approval 后调用 action-executor，并把 execution / result 状态纳入低敏总报告；
  该 runner 不保存 raw tool input / output。
- 2026-06-24 `action-preflight-safety` smoke / eval catalog 已从 11 个扩到
  14 个 case：approval id、prepared audit id、resource id 与 approved proposal
  绑定不一致时均返回 `PROPOSAL_MISMATCH`，并证明不会写 execution audit、不会写
  tool result projection、不会调用 tool executor。
- 2026-06-25 该 adapter 已纳入 ai-eval required local gate，随 CI-safe
  `check-ai-eval-regression-gate` 默认运行；仍只使用 in-memory ports 和 local safe
  executor，不调用真实业务工具、数据库、Docker 或 live service stack。
- 2026-06-25 `conversation.note.create` 已从 audit-only 边界推进为显式 opt-in
  business adapter：配置 conversation-service gRPC 地址后，approval 后的 action-executor
  会调用 conversation-service 公开 API 写入 note；未配置时仍明确 `executed=false`，
  不伪造成功。
- 同日 `loadtest/ragagent` / `run-ai-eval-ragagent-adapter.ps1` 已补显式 execute-mode
  verification：开启 `expect-business-action-executed` 后，必须看到 execution `SUCCEEDED`、
  低敏 output 不回显 note body，并由 loadtest verification 确认 `conversation_notes`
  中 note fact 与 proposal / approval 绑定一致；默认 audit-only gate 仍要求
  `executed=false`。
- 同日 `ai-eval-rag-agent-demo-live-20260625-business-mutation-execute-v7` 已通过真实完整
  service-stack opt-in mutation smoke：approved Agent proposal 经 action-executor 执行后
  写入真实 conversation note fact，且 execution status `RECORDED`、tool output 低敏。
- 同日 `conversation.profile.update` 已补为第二个显式 conversation business adapter：
  approved Agent action 可经 action-executor 调用 conversation-service public API 更新会话资料；
  adapter 要求 LOW risk、conversation resource、EXECUTE skill 和 expected profile version，
  并保持低敏 output，不把 title / avatar / announcement 原文写入 tool result。
- 同日 `loadtest/ragagent` / `run-ai-eval-ragagent-adapter.ps1` 已把
  `-ExpectBusinessActionExecuted` 升级为 note + profile 双 mutation gate；execute-mode
  必须同时验证 conversation note fact 和 public `GetConversationProfile` 读回的 profile
  update，默认 audit-only 模式仍要求两类 mutation 都不执行。
- 2026-06-25 重建 action-executor / workflow-service runtime 后，
  `ai-eval-service-stack-live-20260625-rag-agent-note-profile-mutation` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped。
  该报告确认 approved Agent action 能经 action-executor 同时写入真实 conversation
  note fact，并通过 conversation-service public API 更新 conversation profile；tool
  output 仍只保留 version / hash 元数据。

## 边界

- 不执行任意外部 MCP / provider tool；当前只允许显式开启的 LOW-risk HTTP adapter first path。
- 不自动执行高风险 / 真实业务写动作；执行前仍必须经过 proposal / approval / prepare / policy。
- 不保存 raw `input_json`、provider secret、provider output 或 provider 原始错误。
- 未配置 adapter 的业务 tool 默认 `executed=false`；conversation note / profile、echo /
  allowlisted HTTP provider tool 均必须显式满足各自 adapter contract 才能 `SUCCEEDED`。

## 下一步

- 后续真实 redrive API / provider replay / metrics / operator UI；继续扩展真实业务 mutation 时必须新增显式
  adapter、公开业务 API 和 operator / policy 边界，不允许用未配置工具静默执行。
