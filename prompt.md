# NexusIM Project Routing

本文件只维护 NexusIM 的项目入口、文档路由和通用协作原则。不在这里
保存 Codex 目标框正文、临时粘贴内容、历史副本或具体切片正文。
Codex 目标框内容只存在于 Codex UI，不进入仓库文档。
仓库文档只维护可持续更新的项目事实。

具体 active slice、推进策略、架构边界、优先级和阶段结论由对应进度文档维护。
本文件只告诉后续 agent 应该去哪里读取项目事实。
本文件不保存一次性提示词、目标框正文、临时切片正文或可被进度文档替代的重复内容。

## 文档路由

- 本文件只维护每轮文档路由和工作原则；具体目标、推进策略和架构边界见
  `docs/runbook/current-goal.md`、`docs/runbook/current-brief.md`、`agent.md` 和
  `docs/architecture/target-architecture-complete.md`。
- 根目录 `README.md` 是 GitHub 首页总览，阶段、架构、客户端能力、新服务、
  新中间件或下一步状态变化时必须同步维护。
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
