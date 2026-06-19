# NexusIM Codex Prompt

## Codex 目标框短 Prompt

把下面这段放进 Codex 目标框即可：

```text
持续推进 E:\development\IM 的 NexusIM 项目。当前主线就是 AI 大模型应用底座，不是继续泛化清九个 IM 服务 P2 backlog。

【当前主线 / 不可偏移】
默认推进这条链：group memory -> EvidencePack -> RAG -> summary -> multi-agent -> skill-registry -> MCP/tool gateway -> action-executor -> proposal / approval / audit -> ai-eval。

九个既有 IM 服务是可运行基础，不再作为默认主线继续堆长期 P2 hardening。只处理阻塞 AI 主线的 P0/P1、用户明确点名任务、或本轮切片必须补的边界。不要把默认任务切回生产级 HA、长压、sizing、provider-grade 运维或泛泛“把九服务继续做干净”。

每轮开始：
1. 执行 git status --short --branch --untracked-files=all。
2. 读取 prompt.md 和 agent.md。
3. 读取 docs/runbook/current-goal.md 的当前 active slice；再按需读取 current-brief、remaining-goals、相关 service brief 或 SDD；不要全文扫长历史文档。

当前 active slice：skill-registry first catalog path、mcp-gateway first prepare path、action-executor first execution audit path、Agent -> mcp-gateway adapter smoke、proposal store、approval workflow、action-executor approved proposal handoff、Agent execution eval adapter 和 low-sensitive tool result projection 已落；agent-service proposal 前会调用 mcp-gateway prepare，并返回 skill_id / prepared_audit_id；action-executor 会通过 agent-service 公开 RPC 校验 approved proposal，但仍不执行外部工具。下一步默认推进 real tool adapter、approval operator / audit outbox、真实 tool output safety cases；如果该串接已完成，则按 current-goal.md 选择下一项 AI 主线任务。

硬边界：RAG / summary / Agent 只能消费 EvidencePack，不直接读 message / conversation / private tables；真实写动作必须继续走 policy、proposal / approval / executor / audit。

可用多个 sub-agent 做互不重叠任务；主 agent 负责集成、检查和文档同步。不回滚用户已有修改。新发现的待办写入 docs/runbook/remaining-goals.md。
```

## 本文件的作用

- 本文件只维护 Codex 目标框短 Prompt 和每轮文档路由；具体目标见 `docs/runbook/current-goal.md`。
- `agent.md` 决定按需读取和维护哪些项目文档；阶段细节见 `docs/runbook/current-brief.md`。
- 未完成工作见 `docs/runbook/remaining-goals.md`；单服务状态见 `docs/runbook/service-briefs/<service>.md`。

## 每轮开始

1. 运行 `git status --short --branch --untracked-files=all`。
2. 读取根目录 `prompt.md` 和 `agent.md`，确认本轮入口、文档路由和进度维护规则。
3. 按 `agent.md` 的任务类型读取对应文档；每轮只读本轮必要文档，不默认全文读取所有 runbook、SDD、archive、history、loadtest 报告。
4. 只维护本轮事实变化涉及的文档；不为了“了解项目”全文读取或批量改写长历史文档。

## 工作原则

1. 主线阶段以 `current-brief.md` / `remaining-goals.md` 为准；不要在本文件重复维护长状态。
2. 小切片闭环：设计、代码、必要测试、文档一起收；生产级长压 / HA / sizing 默认后置。
3. 降低耦合并控制复杂度：不跨服务读内部表，不引入网状同步 RPC，接近行数阈值就拆同 package 文件。
4. 新服务和中间件不写死；满足独立模型 / 伸缩 / 故障 / 安全边界或明显降复杂度时通过 ADR 新增。
5. 可用多个 sub-agent，但必须拆分互不重叠职责；主 agent 负责集成和最终检查。
6. 压测原始数据放 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
7. 新发现待办写入 `docs/runbook/remaining-goals.md`；不回滚用户已有修改。

## 每轮结束
1. 若当前阶段变化，更新 `docs/runbook/current-brief.md`。
2. 若剩余工作变化，更新 `docs/runbook/remaining-goals.md`。
3. 若服务状态变化，更新对应 `docs/runbook/service-briefs/<service>.md`。
4. 若面试讲述线变化，更新 `docs/interview/project-progress.md`。
5. 需要历史归档时追加到 archive / loadtest 报告，不把长历史塞回入口文档。
6. 按本轮风险运行必要检查；短期不把生产级压测、长周期演练或完整生产就绪测试作为默认收口条件，除非本轮任务明确涉及。
