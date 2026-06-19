# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`，
不要把长 prompt 复制到这里。

## 当前默认主线

```text
AI 大模型应用底座：
group memory -> EvidencePack -> RAG -> summary -> multi-agent
-> skill-registry -> MCP/tool gateway -> action-executor
-> proposal / approval / audit -> ai-eval
```

9 个既有 IM 服务是可运行基础；默认只处理阻塞 AI 主线的 P0/P1、用户点名任务，
或本轮切片必须补的边界。生产级 HA、长压、sizing、provider-grade 运维后置。

## 当前 Active Slice

已落：

- `search-service`、`memory-service`、`retrieval-gateway` first paths。
- `rag-service`、`summary-service` read-only EvidencePack paths。
- `agent-service` proposal-only path、mcp-gateway prepare、proposal store、
  approval workflow、approval audit outbox、`VerifyApprovedAgentProposal`。
- `skill-registry` first catalog path。
- `mcp-gateway` first prepare path。
- `action-executor` first execution audit、approved proposal preflight、
  low-sensitive result projection、本地安全 `nexusim.local.echo` adapter。
- Agent execution eval adapter 覆盖 approval / execution / result projection /
  safe local tool output。

下一步默认推进：

```text
approval operator
-> approval outbox relay
-> external MCP adapter failure fallback
-> true tool output safety cases
```

硬边界：RAG / summary / Agent 只能消费 EvidencePack；真实写动作必须走
policy、proposal / approval、executor 和 audit；不能直接读业务私表。

## 执行规则

1. 每轮先读 `prompt.md`、`agent.md`、`current-brief.md`、`remaining-goals.md`。
2. 当前任务涉及哪个服务，再读对应 `service-briefs/<service>.md` 和 SDD 章节。
3. 新发现待办写入 `remaining-goals.md`。
4. 有意义切片要闭环：代码、测试、文档、focused commit。

## 分层路线

| 层级 | 内容 | 当前状态 |
| --- | --- | --- |
| 第一层 | 发消息、会话、投递、在线通知、ACK | 最小闭环已完成 |
| 第二层 | outbox、Kafka、durable inbox、Redis route、多实例 smoke | 本地 / 双机可运行 |
| 第三层 | 回执、删除/撤回/编辑、联系人、群管理、鉴权 | 必要收口 |
| 第四层 | search、memory、retrieval、RAG、summary、Agent、tool/action | 当前主线 |
