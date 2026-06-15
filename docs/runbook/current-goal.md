# NexusIM Current Goal

本文件只维护长期目标摘要。可复制到 Codex 目标框的真实 prompt 维护在仓库根目录 `prompt.md`。

## 短 Goal Prompt

```text
持续推进 E:\development\IM 的 NexusIM 项目。每轮先运行 git status --short --branch --untracked-files=all，然后读取仓库根目录 prompt.md；只按 prompt.md 的路由继续读取必要短文档并执行。不要全文读取长历史文档；不要回滚用户已有修改。
```

## 长期目标

把 NexusIM 做成可讲清楚工程边界、分布式可靠性和 AI 应用后端扩展路线的 IM 后端项目。

当前阶段：

```text
先收干净已有 9 个后端服务
-> 再进入 search-service
-> 再做 RAG / summary / agent 后端
```

Web / App / 桌面端是后续产品化展示层，不是当前后端主线。

## 分层路线

| 层级 | 内容 | 当前状态 |
| --- | --- | --- |
| 第一层 | 最小 IM 主链路：发消息、会话、投递、在线通知、ACK | 已完成最小闭环 |
| 第二层 | 分布式与可靠性：outbox、Kafka、durable inbox、Redis route、多实例、故障 smoke | 已有本地 / 双机 smoke，不等于生产 HA |
| 第三层 | 完整 IM 后端产品能力：回执、撤回/编辑/删除、会话列表、联系人、群管理、鉴权、api-gateway | 已落地基础能力，继续 hardening |
| 第四层 | 搜索与智能化后端：search、RAG、summary、agent | 后续阶段 |

## 文档路由

具体文档路由看 `docs/runbook/README.md`。默认不要读 archive 全文，只在查历史证据时按关键词读取相关段落。
