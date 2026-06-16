# NexusIM Codex Prompt

## Codex 目标框短 Prompt

把下面这段放进 Codex 目标框即可：

```text
持续推进 E:\development\IM 的 NexusIM 项目。每轮先运行 git status --short --branch --untracked-files=all，然后读取仓库根目录 prompt.md 和 agent.md，并按文档指向的项目目标继续执行；不要把具体目标写在目标框里，不全文读取长历史文档，不回滚用户已有修改。
```

## 本文件的作用

- 本文件只维护 Codex 目标框短 Prompt 和每轮文档路由。
- 具体执行目标维护在 `docs/runbook/current-goal.md`；目标框不要复制长目标。
- Agent 进度管理规则见 `agent.md`；需要管理项目进度、分配子 agent 或选择下一切片时先读它。
- `prompt.md` 只负责把 Codex 带到正确入口；`agent.md` 决定本轮需要按需读取和维护哪些项目文档。
- 具体当前阶段不在这里维护，见 `docs/runbook/current-brief.md`。
- 当前未完成工作不在这里维护，见 `docs/runbook/remaining-goals.md`。
- 单服务状态不在这里维护，见 `docs/runbook/service-briefs/<service>.md`。

## 每轮开始

1. 运行 `git status --short --branch --untracked-files=all`。
2. 读取 `agent.md`，确认本轮文档路由和进度维护规则。
3. 按 `agent.md` 的任务类型读取对应文档；不要默认全文读取所有 runbook、SDD、archive、loadtest 报告。
4. 只维护本轮事实变化涉及的文档；不为了“了解项目”全文读取或批量改写长历史文档。

## 工作原则

1. 当前阶段以 `docs/runbook/current-brief.md` 和 `docs/runbook/remaining-goals.md` 为准，不在本文件重复维护。
2. 小切片闭环：设计、代码、测试、文档一起收。
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
6. 至少运行 `.\tools\check-local.ps1`；按风险追加服务级测试、集成测试或 smoke。
