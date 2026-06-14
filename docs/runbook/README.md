# NexusIM Runbook Index

这是 runbook 的短路由页。不要把历史结论、压测详情或长设计内容写到这里。

## 默认入口

- Codex 目标 prompt：`../../prompt.md`
- 当前每轮入口：`current-brief.md`
- 长期目标摘要：`current-goal.md`
- 服务短状态索引：`service-briefs/README.md`

## 按需读取

- 单服务状态：`service-briefs/<service>.md`
- 开发进度总览：`development-progress.md`
- 开发过程与阶段顺序：`development-process.md`
- 服务设计：`../sdd/<service>.md`
- smoke / 压测证据：`loadtest/<service>/`
- 本地分布式和 Docker：`distributed-local.md`、`mac-arm64-docker-images.md`
- 本地压测操作：`local-loadtest.md`
- 历史长文档：`archive/`、`history/`

## 维护规则

- 当前入口和索引必须保持短。
- 历史事实写入 archive / history / loadtest 报告，不回填到入口文档。
- 每轮结束运行 `..\..\tools\check-local.ps1`。
