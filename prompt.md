# NexusIM Codex Prompt

## Codex 目标框内容

目标框只保留下面这段通用文档入口。具体 active slice、推进策略、架构边界、
优先级、待开发服务和治理规则都从仓库文档读取，不在目标框里写死。

```text
持续推进 E:\development\IM 的 NexusIM。

目标框只保留稳定文档入口，不写当前具体任务、服务清单、推进策略、架构边界、
优先级或阶段结论。
当前 active slice、下一步、待开发服务、推进策略和架构边界必须从仓库文档读取并维护。
如果目标框旧内容和仓库文档冲突，以仓库文档为准。

每轮开始：
1. 执行 git status --short --branch --untracked-files=all。
2. 读取 prompt.md 和 agent.md，确认文档路由和工作规则。
3. 读取 docs/runbook/current-goal.md 获取当前 active slice。
4. 按需读取 current-brief、remaining-goals、service brief、必要 SDD、目标架构或 fail-closed policy；不要全文扫长历史文档。

工作方式：按 current-goal 小切片闭环；可用多个 sub-agent 做互不重叠任务；
主 agent 负责集成、检查和文档同步。不回滚用户已有修改。
新发现待办写入 docs/runbook/remaining-goals.md。
active slice、推进策略、架构边界、服务 promotion、客户端能力、AI 边界或下一步状态变化时，
同步维护对应进度文档和根 README.md。
每个新功能先做简短架构分析，再编码：确认 owner service、数据所有权、API / 事件、
权限 / 审计、是否需要新技术或新中间件、是否影响客户端 / AI / Agent 边界，以及需要同步哪些文档。
新增内容必须归属到对应平台层：中间件归中间件平台，数据处理归数据平台，AI / Agent
能力归 AI / Agent 平台，业务能力归业务 / 产品平台，客户端能力归客户端平台。
新增或变更微服务 / 中间件 / provider / runtime 时，必须同步 README.md、相关架构文档、
middleware catalog、service brief / SDD / ADR、current-goal / current-brief /
remaining-goals 中的相关事实。

语言、架构和 fail-closed 边界是可维护文档内容，不在目标框里展开。
实现前按需读取 agent.md、docs/architecture/target-architecture-complete.md、
docs/architecture/fail-closed-policy.md 和相关 SDD。
持续清理相关切片里的隐藏兜底 / fallback 代码；新代码不得新增隐藏兜底路径。

门禁按风险分层：小改只跑相关测试 / 文档脚本；跨服务、生成代码、migration、service-registry、Docker/compose、安全边界或提交推送前才跑完整 check-local。
```

## 文档路由

- 本文件只维护 Codex 目标框内容和每轮文档路由；具体目标、推进策略和架构边界见
  `docs/runbook/current-goal.md`、`docs/runbook/current-brief.md`、`agent.md` 和
  `docs/architecture/target-architecture-complete.md`。
- 根目录 `README.md` 是 GitHub 首页总览，阶段、架构、客户端能力、新服务、
  新中间件或下一步状态变化时必须同步维护。
- Codex 目标框可能更新不及时；若目标框和仓库文档冲突，以 `current-goal.md` / `current-brief.md` / `remaining-goals.md` 为准。
- `agent.md` 决定按需读取和维护哪些项目文档；阶段细节见 `docs/runbook/current-brief.md`。
- 未完成工作见 `docs/runbook/remaining-goals.md`；单服务状态见 `docs/runbook/service-briefs/<service>.md`。

## 工作原则

1. 主线阶段以 `current-goal.md` 和 `current-brief.md` 为准；不要在本文件维护当前 active slice 细节。
2. 小切片闭环：设计、代码、必要测试、文档一起收；默认跑相关局部门禁，不频繁跑完整 `check-local`。
3. 降低耦合并控制复杂度：不跨服务读内部表，不引入网状同步 RPC，接近行数阈值就拆同 package 文件。
4. 新功能先架构分析再编码；先判断是否需要新技术、新中间件、新服务或新 provider，
   并确认 owner、平台归属、数据模型、事件 / API、权限、审计、runtime profile 和文档影响。
5. 不引入隐藏业务兜底；开发相关切片时持续删除旧隐藏兜底 / fallback 代码。
   local-test adapter、compat window、显式恢复、repair / redrive 必须按
   `docs/architecture/fail-closed-policy.md` 显式命名和隔离。
6. 新服务和中间件不写死；满足独立模型 / 伸缩 / 故障 / 安全边界或明显降复杂度时通过 ADR 新增。
7. 真实业务语言边界：Go 做后端服务 / BFF / 控制面；TypeScript 做客户端共享核心和 UI；Rust/Kotlin/Swift 只做薄 native adapter；Python 做 AI worker / eval，不接管业务状态。
8. 长期完整架构以 `docs/architecture/target-architecture-complete.md` 为准；业务平台、数据平台、AI / Agent 平台和中间件平台的新开发都必须遵守其中的边界。
9. 可用多个 sub-agent，但必须拆分互不重叠职责；主 agent 负责集成和最终检查。
10. 压测原始数据放 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
11. 新发现待办写入 `docs/runbook/remaining-goals.md`；不回滚用户已有修改。
