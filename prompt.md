# NexusIM Codex Prompt

## Codex 目标框短 Prompt

把下面这段放进 Codex 目标框即可：

```text
持续推进 E:\development\IM 的 NexusIM 项目。

最高优先级主线：NexusIM 已从“只做 IM 后端服务”转向“面试导向的后端 + 分布式 + AI 大模型应用系统”。后续默认围绕 AI/RAG/Agent 主链路推进，而不是无限做生产化 hardening。

开发路线：
1. 只做现有 9 个 IM 后端服务的必要收口；非阻塞生产化事项写入 backlog。
2. 主动推进 AI 链路：search-service -> memory-service -> retrieval-gateway / EvidencePack -> rag-service -> summary-service -> agent-service -> skill-registry -> mcp-gateway/tool-gateway -> action-executor -> ai-eval。
3. AI 重点：群组 memory、跨群/跨时间 evidence、权限过滤 RAG、multi-agent 协作、MCP/skill/tool 调用、proposal/approval/executor/audit 真实业务闭环。

不要把“继续开发”理解成无限做生产级长压、完整 HA、sizing 或 provider-grade 运维；这些只作为 hardening backlog，除非用户明确点名。

已完成基线：9 个 IM 后端服务已跑通主链路；search-service projection smoke passed；memory-service group memory / StructuredMemoryEvent projection smoke passed；retrieval-gateway 第一轮 search + memory -> EvidencePack smoke passed；EvidencePack field hardening first pass（source coverage / rerank score / dedupe reason）已落；AI eval harness first pass 已有低敏 case schema 和 validator。

当前开发主线：
1. 当前 active slice：RAG 前置设计和最小只读问答边界；policy-service retrieval precheck 已有 first-stage 可选接入，EvidencePack 必须继续保持 source refs、temporal version、visibility / policy boundary，不直接读 message/conversation/private tables。
2. RAG / summary / Agent 仍只能消费 EvidencePack；新增 eval case 先进入 docs/runbook/ai-eval/retrieval-eval-cases.json。
3. 后续按顺序推进 RAG / summary / Agent / skill / MCP / action execution。

本轮只做能推进上述主线的工作；新发现的非阻塞生产化事项写入 docs/runbook/remaining-goals.md，不要抢占当前 AI/RAG/Agent 路线。

每轮先运行 git status --short --branch --untracked-files=all，读取 prompt.md 和 agent.md，再按需读取 current-brief / remaining-goals / 相关 service brief；可用多个 sub-agent 做互不重叠任务；不全文扫长历史文档，不回滚用户已有修改。新发现的待办写入 docs/runbook/remaining-goals.md。
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
