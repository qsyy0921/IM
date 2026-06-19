# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`，
不要把长 prompt 复制到这里。

## 当前默认主线

```text
当前唯一默认主线：AI 大模型应用底座。
group memory -> EvidencePack -> RAG -> summary -> multi-agent
-> skill-registry -> MCP/tool gateway -> action-executor
-> proposal / approval / audit -> ai-eval
```

如果用户只说“继续 / 接下来做什么 / 动手去做”，默认推进这条 AI 主线。
9 个既有 IM 服务是可运行基础；默认只处理阻塞 AI 主线的 P0/P1、用户点名任务，
或本轮切片必须补的边界。不要把任务自动切回九服务长期 P2 hardening、
生产级 HA、长压、sizing、provider-grade 运维或 Docker/双机基础设施整理。

## 当前 Active Slice

已落：

- search / memory / retrieval / RAG / summary / Agent first paths。
- skill-registry、mcp-gateway、action-executor first paths。
- proposal / approval / audit / approval outbox relay / approved execution。
- local safe tool adapter、guarded external HTTP adapter、tool output safety。
- Python AI Worker foundation、Go-side candidate adapter smoke、RAG / Summary /
  Agent 服务级 Python candidate guard。
- profile overgeneralization / Agent output safety eval cases 和扩展回归。
- `ai-eval-service` persistent eval run catalog、RecordEvalRun recorder smoke、
  multi-adapter regression gate smoke、gate policy manifest、Python optional
  adapter path、service-stack preflight wrapper、RAG / Agent optional
  service-stack live gate smoke、CI-safe gate skeleton、RAG / Agent regression
  case expansion first pass、profile / Agent output safety expansion、
  service-stack version / hash-only expansion、negative RAG / Agent cases、
  Summary live negative adapter、Python / model-output negative regression cases、
  RAG / Summary citation source-ref regression、external MCP fallback eval cases、
  Agent output regression 和报告：
  `docs/runbook/loadtest/ai-eval-service/`。

下一步默认推进：

```text
继续扩 tool/action safety cases
```

硬边界：RAG / summary / Agent 只能消费 EvidencePack；真实写动作必须走
policy、proposal / approval、executor 和 audit；不能直接读业务私表。
Python AI Worker 只能做模型 / 算法 / eval 候选层，Go 仍负责控制面和事实边界。

## 执行规则

1. 每轮先读 `prompt.md`、`agent.md` 和本文件。
2. 需要阶段背景时读 `current-brief.md`；需要选择未完成任务时读 `remaining-goals.md`。
3. 当前任务涉及哪个服务，再读对应 `service-briefs/<service>.md` 和必要 SDD 章节。
4. 新发现待办写入 `remaining-goals.md`。
5. 有意义切片要闭环：代码、测试、文档、focused commit。

## 分层路线

| 层级 | 内容 | 当前状态 |
| --- | --- | --- |
| 第一层 | 发消息、会话、投递、在线通知、ACK | 最小闭环已完成 |
| 第二层 | outbox、Kafka、durable inbox、Redis route、多实例 smoke | 本地 / 双机可运行 |
| 第三层 | 回执、删除/撤回/编辑、联系人、群管理、鉴权 | 必要收口 |
| 第四层 | search、memory、retrieval、RAG、summary、Agent、tool/action | 当前主线 |
