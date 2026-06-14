# NexusIM Codex Prompt

持续推进 `E:\development\IM` 的 NexusIM 项目。

## 每轮开始

1. 运行 `git status --short --branch`。
2. 读取 `docs/runbook/current-brief.md`。
3. 需要更多状态时，先读 `docs/runbook/README.md` 和相关 `service-briefs/<service>.md`。
4. 不全文读取长历史文档；只按关键词读取相关 SDD、runbook、loadtest 报告或 archive 片段。
5. 不回滚用户已有修改。

## 当前主线

只聚焦后端、分布式可靠性和 AI 应用后端。Web / App / 桌面端暂不纳入当前开发主线。

当前顺序：

```text
先把已有 9 个后端服务收干净
-> 再进入 search-service
-> 再做 RAG / summary / agent 后端
```

## 下一步优先

继续做入口治理和后端服务观测 rollout：

- 运行时动态 tenant quota 文件热更新已进入第一阶段，后续只做配置中心 / DB-backed quota hardening；
- api-gateway 已有入口 server span、下游 gRPC client span、第一阶段 Prometheus text `/metrics`、本地 Prometheus scrape / alert rules 原型和本地 Grafana dashboard 原型；
- contacts-service、identity-service、message-service、conversation-service、delivery-service、receipt-service、policy-service 已开始后端服务 gRPC server span rollout，push-gateway 已补 first-stage WebSocket connection span；本地 OTel collector debug 入口、policy OTLP smoke 脚本和 first-stage trace sampling policy / check 已补；后续继续做采样治理 hardening；
- legacy descriptor opt-in 使用面已有 first-stage metrics 计数，后续继续迁移观察和移除计划；
- 必要单测 / 集成测试；
- 同步相关 service brief、SDD 或进度文档。

## 工作原则

1. 小切片闭环：设计、代码、测试、文档一起收。
2. 控制耦合和复杂度：不跨服务读内部表，不引入网状同步 RPC，不为了短期功能抽公共包。
3. 优先复用已有事实流、outbox、projection、read model 和端口。
4. 生产手写文件接近 2500 行、测试或 runner 接近 3000 行时及时同 package 拆分。
5. 压测原始数据放 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
6. 每轮结束运行 `.\tools\check-local.ps1`，按风险追加必要测试，提交并推送有意义的切片。
