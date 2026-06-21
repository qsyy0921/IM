# NexusIM Codex Prompt

## Codex 目标框短 Prompt

把下面这段放进 Codex 目标框即可：

```text
持续推进 E:\development\IM 的 NexusIM。

目标框只保留稳定工作规则，不写当前具体任务、服务清单、优先级或阶段结论。
当前 active slice、边界、下一步和待开发服务必须从仓库文档读取并维护。
如果目标框旧内容和仓库文档冲突，以仓库文档为准。

每轮开始：
1. 执行 git status --short --branch --untracked-files=all。
2. 读取 prompt.md 和 agent.md，确认文档路由和工作规则。
3. 读取 docs/runbook/current-goal.md 获取当前 active slice。
4. 按需读取 current-brief、remaining-goals、service brief 和必要 SDD；不要全文扫长历史文档。

工作方式：按 current-goal 小切片闭环；可用多个 sub-agent 做互不重叠任务；主 agent 负责集成、检查和文档同步。不回滚用户已有修改。新发现待办写入 docs/runbook/remaining-goals.md。

语言边界：Go 负责后端服务、BFF、控制面、事实源、审计和安全边界；TypeScript 负责 Web/PC/Android 共享客户端协议、同步核心和 UI；Rust/Kotlin/Swift 只做薄平台桥；Python 只做 AI worker、模型算法、eval 和离线工具，不能拥有业务事实源。

门禁按风险分层：小改只跑相关测试 / 文档脚本；跨服务、生成代码、migration、service-registry、Docker/compose、安全边界或提交推送前才跑完整 check-local。
```

## 文档路由

- 本文件只维护 Codex 目标框短 Prompt 和每轮文档路由；具体目标见 `docs/runbook/current-goal.md`。
- Codex 目标框可能更新不及时；若目标框和仓库文档冲突，以 `current-goal.md` / `current-brief.md` / `remaining-goals.md` 为准。
- `agent.md` 决定按需读取和维护哪些项目文档；阶段细节见 `docs/runbook/current-brief.md`。
- 未完成工作见 `docs/runbook/remaining-goals.md`；单服务状态见 `docs/runbook/service-briefs/<service>.md`。

## 工作原则

1. 主线阶段以 `current-goal.md` 和 `current-brief.md` 为准；不要在本文件维护当前 active slice 细节。
2. 小切片闭环：设计、代码、必要测试、文档一起收；默认跑相关局部门禁，不频繁跑完整 `check-local`。
3. 降低耦合并控制复杂度：不跨服务读内部表，不引入网状同步 RPC，接近行数阈值就拆同 package 文件。
4. 新服务和中间件不写死；满足独立模型 / 伸缩 / 故障 / 安全边界或明显降复杂度时通过 ADR 新增。
5. 真实业务语言边界：Go 做后端服务 / BFF / 控制面；TypeScript 做客户端共享核心和 UI；Rust/Kotlin/Swift 只做薄 native adapter；Python 做 AI worker / eval，不接管业务状态。
6. 长期完整架构以 `docs/architecture/target-architecture-complete.md` 为准；业务平台、数据平台、AI / Agent 平台和中间件平台的新开发都必须遵守其中的边界。
7. 可用多个 sub-agent，但必须拆分互不重叠职责；主 agent 负责集成和最终检查。
8. 压测原始数据放 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
9. 新发现待办写入 `docs/runbook/remaining-goals.md`；不回滚用户已有修改。
