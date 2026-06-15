# NexusIM Codex Prompt

## Codex 目标框短 Prompt

把下面这段放进 Codex 目标框即可：

```text
持续推进 E:\development\IM 的 NexusIM 项目。每轮先运行 git status --short --branch --untracked-files=all，然后读取仓库根目录 prompt.md；只按 prompt.md 的路由继续读取必要短文档并执行。不要全文读取长历史文档；不要回滚用户已有修改。
```

## 本文件的作用

- 本文件维护 Codex 长期目标 prompt 的真实内容。
- Codex 目标框只放上面的短 Prompt，不复制本文件全文，也不在目标框里直接维护长规则。
- 当前状态和下一步优先级以 `docs/runbook/current-brief.md` 为准。
- 需要更细服务状态时，只读取相关 `docs/runbook/service-briefs/<service>.md`。
- 历史证据、SDD、smoke 报告和 archive 只在需要时按关键词读取。

## 每轮开始

1. 运行 `git status --short --branch --untracked-files=all`。
2. 读取 `docs/runbook/current-brief.md`。
3. 若需要定位文档，先读 `docs/runbook/README.md`。
4. 若需要服务状态，先读 `docs/runbook/service-briefs/README.md`，再读对应服务短文档。
5. 不为了“了解项目”全文读取长历史文档。
6. 不回滚用户已有修改。

## 当前主线

面试主线暂时只覆盖后端、分布式可靠性和 AI 应用后端：

```text
先把已有 9 个后端服务收干净
-> 再进入 search-service
-> 再做 RAG / summary / agent 后端
```

Web / App / 桌面端属于后续产品化展示层，当前先不作为开发主线。

当前默认任务不是新增服务，而是先解决已有 9 个服务的收口问题：

1. 收紧安全启动门禁、trusted metadata、TLS / mTLS 暴露边界。
2. 继续补本地可验证的观测、故障恢复 smoke、repair / DLQ / audit。
3. 按服务清 P2 hardening 和容量 / 复杂度治理。
4. 每个切片都要有测试或 smoke 证据，并更新对应 service brief / progress 文档。

## 工作原则

1. 小切片闭环：设计、代码、测试、文档一起收。
2. 降低耦合：不跨服务读内部表，不引入网状同步 RPC，不为了短期功能抽公共包。
3. 控制复杂度：生产手写文件接近 2500 行、测试或 runner 接近 3000 行时，优先同 package 拆分。
4. 优先复用已有事实流、outbox、projection、read model 和端口。
5. 项目统一命名为 NexusIM，不再引入旧项目名或旧品牌残留。
6. 新服务和中间件不写死；只有独立数据模型、独立伸缩需求、独立故障边界或显著降低复杂度时才新增，并通过 ADR。
7. 压测原始数据放 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
8. 每个有意义切片结束后，按风险运行必要测试，更新对应 brief / SDD / runbook，并提交推送。

## 每轮结束

1. 若当前优先级变化，更新 `docs/runbook/current-brief.md`。
2. 若服务状态变化，更新对应 `docs/runbook/service-briefs/<service>.md`。
3. 需要历史归档时追加到 archive / loadtest 报告，不把长历史塞回入口文档。
4. 至少运行 `.\tools\check-local.ps1`；按风险追加服务级测试、集成测试或 smoke。
