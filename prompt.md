# NexusIM Codex Prompt

## Codex 目标框短 Prompt

把下面这段放进 Codex 目标框即可：

```text
持续推进 E:\development\IM 的 NexusIM 项目，当前主线是必要收口 + 转向 AI 大模型应用底座。每轮先运行 git status --short --branch --untracked-files=all，然后读取仓库根目录 prompt.md 和 agent.md，再按需读取本轮必要文档；短期生产级测试后置，不把具体长目标写进目标框，不全文扫长历史文档，不回滚用户已有修改。
```

## 本文件的作用

- 本文件只维护 Codex 目标框短 Prompt 和每轮文档路由。
- 具体执行目标维护在 `docs/runbook/current-goal.md`；目标框不要复制长目标。
- Agent 进度管理规则见 `agent.md`；需要管理项目进度、分配子 agent 或选择下一切片时先读它。
- `prompt.md` 只负责把 Codex 带到正确入口；`agent.md` 决定本轮需要按需读取和维护哪些项目文档。
- 当前主线只在入口保持短句：必要收口 + 转向 AI 大模型应用底座；具体长目标仍由 runbook 维护。
- 具体当前阶段不在这里维护，见 `docs/runbook/current-brief.md`。
- 当前未完成工作不在这里维护，见 `docs/runbook/remaining-goals.md`。
- 单服务状态不在这里维护，见 `docs/runbook/service-briefs/<service>.md`。

## 每轮开始

1. 运行 `git status --short --branch --untracked-files=all`。
2. 读取根目录 `prompt.md` 和 `agent.md`，确认本轮入口、文档路由和进度维护规则。
3. 按 `agent.md` 的任务类型读取对应文档；每轮只读本轮必要文档，不默认全文读取所有 runbook、SDD、archive、history、loadtest 报告。
4. 只维护本轮事实变化涉及的文档；不为了“了解项目”全文读取或批量改写长历史文档。

## 工作原则

1. 当前主线是必要收口 + 转向 AI 大模型应用底座；阶段细节以 `docs/runbook/current-brief.md` 和 `docs/runbook/remaining-goals.md` 为准，不在本文件重复维护。
2. 小切片闭环：设计、代码、必要测试、文档一起收；短期生产级压测、长周期演练和生产就绪测试后置到明确阶段或用户指定任务。
3. 降低耦合：不跨服务读内部表，不引入网状同步 RPC，不为了短期功能抽公共包。
4. 控制复杂度：生产手写文件接近 2500 行、测试或 runner 接近 3000 行时，优先同 package 拆分。
5. 新服务和中间件不写死；只有独立数据模型、独立伸缩需求、独立故障边界或显著降低复杂度时才新增，并通过 ADR。
6. 压测原始数据放 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
7. 新发现的待完成工作写入 `docs/runbook/remaining-goals.md`。
8. 不回滚用户已有修改。

## 每轮结束

1. 若当前阶段变化，更新 `docs/runbook/current-brief.md`。
2. 若剩余工作变化，更新 `docs/runbook/remaining-goals.md`。
3. 若服务状态变化，更新对应 `docs/runbook/service-briefs/<service>.md`。
4. 若面试讲述线变化，更新 `docs/interview/project-progress.md`。
5. 需要历史归档时追加到 archive / loadtest 报告，不把长历史塞回入口文档。
6. 按本轮风险运行必要检查；短期不把生产级压测、长周期演练或完整生产就绪测试作为默认收口条件，除非本轮任务明确涉及。
