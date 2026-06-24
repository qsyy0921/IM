# ai-eval-service

状态：foundation-active / collaborative-memory live service-stack gate passed.

定位：AI eval harness 的低敏持久化目录。它保存 eval run 的 suite、stage、
adapter、状态、计数、summary/report 引用和低敏 metadata。

当前已落：

- proto、migration、六层 skeleton、`RecordEvalRun` / `GetEvalRun` / `ListEvalRuns`、PostgreSQL repository、`grpc` runtime 和 observability wiring。
- `ai-eval-record-smoke` 与 `run-ai-eval-record-run-smoke.ps1`：profile /
  Agent safety summary -> gRPC `RecordEvalRun` -> `GetEvalRun` / `ListEvalRuns`。
- `run-ai-eval-regression-gate-smoke.ps1`、`gate-policy.local.json` 和
  `validate-ai-eval-gate-policy.ps1`：声明必跑 adapter、阈值、Get / List
  读回、禁止持久化字段和可选 service-stack adapter。
- gate runner 支持 optional adapters；Python path、memory-service、retrieval-gateway
  和 RAG / Summary / Agent service-stack live gate 已登记到 catalog。
- `check-ai-eval-regression-gate.ps1` 已接入 `check-local`，只跑 CI-safe
  required adapters，不启动 Docker / PostgreSQL / live RAG-Agent stack。
- Case catalog 76；current-memory service-stack live gate 38/38 passed；cross-group / temporal memory fixture eval、retrieval smoke、RAG / Summary / Agent stack smokes 和 40/40 optional stack gate 已落。
- 2026-06-23 低敏 collaborative-memory eval 扩到 20 个 profile / Agent safety fixture cases，
  新增 multi-hop actor-chain completeness、workstream / decision dependency edge、
  reviewed multi-source profile activation、supporting-memory delete 后 profile recompute
  检查；该 gate 不调用模型、数据库或业务服务。
- 2026-06-23 optional live adapter first pass：新增
  `run-ai-eval-memory-adapter.ps1` 和 `run-ai-eval-retrieval-adapter.ps1`；
  gate policy 和 service-stack wrapper 可选择 memory-service、retrieval-gateway、
  RAG、Summary、Agent adapter。RAG / Summary / Agent live adapters 同步断言
  multi-hop actor/source-chain completeness。
- 2026-06-24 memory-service live adapter 已增加
  `must_recompute_profile_via_public_api` 断言：`loadtest/memory` 必须通过
  memory-service 公开 `RecomputeProfileAggregate` RPC 生成 reviewed multi-source
  profile，而不是手工写 active profile row。
- 2026-06-24 `ai-eval-service-stack-live-20260624-collab-memory-v4` 已通过
  真实 service-stack gate：8 adapters、51 cases、47 passed、0 failed、4 skipped。
  通过范围包括 profile / action required adapters、memory-service、retrieval-gateway、
  rag-service、summary-service 和 agent-action-executor。4 个 skipped 是
  当时尚未覆盖的 retrieval-gateway negative / miss cases，不属于 positive
  EvidencePack live smoke 覆盖范围。
- 2026-06-24 `ai-eval-service-stack-live-20260624-retrieval-negative` 已补齐
  retrieval-gateway negative / miss adapter，并通过真实 service-stack gate：
  9 adapters、51 cases、51 passed、0 failed、0 skipped。
- 2026-06-24 `rag-agent-demo` 已接入 optional service-stack adapter：
  `run-ai-eval-ragagent-adapter.ps1` 会运行 `loadtest/ragagent`，断言同一
  tenant / conversation 上的 RAG grounded answer、Agent proposal / approval、
  action-executor audit、EvidencePack graph edge 和 profile aggregate evidence，
  并只持久化 hash、计数、状态和 summary ref。`run-ai-eval-service-stack-gate-smoke.ps1`
  已能对该 adapter 做 endpoint preflight。2026-06-24
  `ai-eval-rag-agent-demo-live-20260624-current-image-fixed` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped；
  RAG grounded answer、Agent approval、action-executor audit、cross-group source refs、
  memory graph edge 和 profile aggregate evidence 均被保留。
- 2026-06-24 `ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3`、
  `ai-eval-rag-agent-demo-live-20260624-temporal-update-v2` 和
  `ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3` 已把 public
  candidate review、temporal replacement 和 profile repair workflow approval 纳入
  RAG-Agent demo 真实 service-stack gate；每轮均为 4 adapters、27 cases、27 passed、
  0 failed、0 skipped。
- 2026-06-24 `run-ai-eval-ragagent-adapter.ps1` 已把 profile repair negative gate
  纳入必检断言：`loadtest/ragagent` summary 必须证明未审批 workflow 和 approval
  payload hash mismatch 都 fail-closed。`rag-agent-demo` optional adapter 的 runtime
  dependency 已显式包含 workflow-service。
- 同日 `ai-eval-rag-agent-demo-live-20260624-profile-repair-negative-v1` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped；该 run
  归档了 profile repair negative gate。
- 同日 `ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1`
  已通过真实 service-stack gate：4 adapters、27 cases、27 passed、0 failed、
  0 skipped；`rag-agent-demo` adapter 新增
  `group_memory_answer_and_proposal_must_preserve_multievent_evidence` 断言，覆盖
  `DECISION` / `BLOCKER` / `FILE` 三类 group memory 同时进入 RAG / Agent EvidencePack。
- 同日 `ai-eval-rag-agent-demo-live-20260624-business-proposal-source-chain-gate-v1`
  已通过真实 service-stack gate：4 adapters、27 cases、27 passed、0 failed、
  0 skipped；`rag-agent-demo` adapter 新增
  `business_proposal_must_preserve_source_chain_and_audit_boundary` 断言，覆盖
  `DECISION` / `TASK` / `STATUS` 三类 reviewed memory 驱动
  `conversation.note.create` proposal、approval 和 action-executor audit。2026-06-25
  已补显式 opt-in conversation note business adapter；未配置真实 adapter 时仍必须
  `business_action_executed=false`。同日 `rag-agent-demo` adapter 支持
  `-ExpectBusinessActionExecuted`：显式开启时断言 action-executor executed / SUCCEEDED、
  `business_note_persisted=true`、note ref / id 存在且 body 只输出 hash；2026-06-25
  该 execute-mode gate 已升级为 note + profile 双 mutation：还必须看到
  `business_profile_updated=true`、profile version 前进、profile action input hash 和
  title / avatar / announcement hash，且不保存 raw profile 字段。默认仍校验 audit-only
  边界。完整 service-stack wrapper 也支持 `-ExpectBusinessActionExecuted`，并在
  preflight 中把 conversation-service endpoint 纳入 execute-mode 必需检查。
  2026-06-25 Docker / PostgreSQL runtime 恢复后，
  `ai-eval-rag-agent-demo-live-20260625-business-mutation-execute-v7` 已通过真实完整
  service-stack opt-in mutation smoke：4 adapters、27 cases、27 passed、0 failed、
  0 skipped；`rag-agent-demo` 确认 `business_action_executed=true`、
  `business_note_persisted=true`、execution status `RECORDED`，且真实 note fact
  与 proposal / approval 绑定一致。同日重建 action-executor / workflow-service runtime 后，
  `ai-eval-service-stack-live-20260625-rag-agent-note-profile-mutation` 已通过真实
  service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped；
  `rag-agent-demo` 进一步确认 `business_profile_updated=true`、profile version 前进，
  且 profile output 只保留字段 hash。
- 2026-06-24 `action-preflight-safety` adapter 已扩到 14 个 smoke cases，并在
  catalog 中新增 approval id、prepared audit id、resource id 绑定错配的
  `PROPOSAL_MISMATCH` 断言；这些 case 区分 action boundary failure 与
  tool execution / result projection failure。
- 2026-06-24 `python-memory-extraction-candidate` optional adapter 已接入：
  catalog 增至 80 cases，新增 4 个低敏 case 覆盖 explicit cue hash-only extraction、
  ordinary chat zero candidates、profile signal review required 和 unsafe input
  fail-closed；`run-ai-eval-memory-extraction-candidate-adapter.ps1` 会运行
  Go-side batch adapter smoke，不调用数据库、服务栈或外部 provider。

边界：不保存 raw EvidencePack、prompt、model output、用户正文、secret 或 tool input；不授权业务动作。

下一步：retrieval positive adapter 的 `must_preserve_source_chain_rerank`
断言已通过 `ai-eval-service-stack-live-20260624-retrieval-source-chain-rerank-v2`
真实 service-stack gate 归档；继续深化 BM25 / vector / graph expansion /
rerank provider 覆盖，并保持 retrieval failure、memory lifecycle failure、
reasoning failure 和 action boundary failure 的独立诊断。
