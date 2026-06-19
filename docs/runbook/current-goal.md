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

- `search-service`、`memory-service`、`retrieval-gateway` first paths。
- `rag-service`、`summary-service` read-only EvidencePack paths。
- `agent-service` proposal-only path、mcp-gateway prepare、proposal store、
  approval workflow、approval outbox relay、`VerifyApprovedAgentProposal`。
- `agent-service` proposal approval operator first path。
- `skill-registry` first catalog path。
- `mcp-gateway` first prepare path。
- `action-executor` first execution audit、approved proposal preflight、
  low-sensitive result projection、本地安全 `nexusim.local.echo` adapter。
- `action-executor` 外部 MCP fallback 错误分类和 tool output safety first path。
- Agent execution eval adapter 覆盖 approval / execution / result projection /
  safe local tool output。
- Python AI Worker foundation 目录、`IM` conda toolchain、candidate contract
  helpers、低敏安全检查和 `tools/check-python-ai-worker-boundary.ps1`。
- RAG/Summary guarded external HTTP LLM boundary：prompt 只能来自 EvidencePack，
  provider failure 回退 extractive，unsafe / malformed output fail closed。
- Python worker malformed / unsafe output eval coverage 和第一条 candidate-only
  worker smoke。
- Go-side Python candidate adapter smoke：Go 控制面调用 Python worker CLI，
  验证只消费低敏 candidate metadata / output hash。
- `rag-service` 服务级 Python worker candidate guard：`python-worker` provider
  mode 调用 Python worker CLI，只接受 hash / citation metadata，最终答案、
  citation verifier 和 fail-closed 仍由 Go 控制。
- `summary-service` 服务级 Python worker candidate guard：`python-worker`
  provider mode 调用 Python worker CLI，只接受 hash / citation metadata，
  最终摘要、citation verifier 和 fail-closed 仍由 Go 控制。
- `agent-service` 服务级 planner Python worker candidate guard：`python-worker`
  proposal provider mode 调用 Python worker CLI，只接受 proposal hash /
  citation metadata，最终 proposal、citation verifier、approval 和 audit
  仍由 Go 控制。
- `action-executor` 外部 HTTP provider guarded adapter first path：默认关闭；
  显式 `http` mode + allowlist + `LOW` risk 才执行，只发送 tool metadata /
  `input_sha256`，provider output 继续走 safety gate 和 output hash projection。
- action-executor external adapter eval / failure smoke：本地 provider fixture
  覆盖 allowlisted success、provider failure 分类、unsafe output 抑制和 raw input
  不外发。
- profile overgeneralization / Agent output safety eval cases：新增低敏
  profile / Agent safety cases 和本地 fixture adapter，覆盖单条群聊事实不能升级为
  ACTIVE profile、profile candidate 必须 PENDING_REVIEW、Agent output 不能泄露
  raw EvidencePack / secret-like 内容且不能发出未审批业务动作。

下一步默认推进：

```text
ai-eval-service first skeleton / persistent eval run catalog
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
