# NexusIM Codex Prompt

## Codex 目标框短 Prompt

把下面这段放进 Codex 目标框即可：

```text
持续推进 E:\development\IM 的 NexusIM。

【当前唯一默认主线】AI 大模型应用底座，不是泛化清理九个 IM 服务 P2 backlog。
默认链路：group memory -> EvidencePack -> RAG -> summary -> multi-agent -> skill-registry -> MCP/tool gateway -> action-executor -> proposal / approval / audit -> ai-eval。

【不要默认做】生产级 HA、长压、sizing、provider-grade 运维、九服务长期 P2 hardening、Docker/双机基础设施整理，或泛泛“把九服务继续做干净”。
九个既有 IM 服务是 AI 主线的可运行底座；只处理阻塞 AI 主线的 P0/P1、用户明确点名任务、或本轮切片必须补的边界。

每轮开始：
1. 执行 git status --short --branch --untracked-files=all。
2. 读取 prompt.md 和 agent.md，确认当前主线仍是 AI 应用底座。
3. 读取 docs/runbook/current-goal.md 的当前 active slice；再按需读取 current-brief、remaining-goals、相关 service brief 或 SDD；不要全文扫长历史文档。

当前 active slice：skill-registry catalog、mcp-gateway prepare、action-executor audit、Agent adapter smoke、proposal store、approval workflow、approval operator、approval outbox relay、approved proposal handoff、execution eval、low-sensitive result projection、本地安全 tool adapter、外部 MCP fallback、tool output safety first paths、外部 HTTP provider guarded adapter first path、external adapter eval / failure smoke、profile overgeneralization / Agent output safety eval cases、Python AI Worker foundation、RAG/Summary external HTTP LLM boundary、Python worker output-safety eval、candidate-only smoke、Go-side Python candidate adapter smoke、rag-service / summary-service / agent-service 服务级 Python worker candidate guard 已落。下一步默认推进 ai-eval-service first skeleton / persistent eval run catalog。

硬边界：RAG / summary / Agent 只能消费 EvidencePack，不直接读 message / conversation / private tables；真实写动作必须继续走 policy、proposal / approval / executor / audit；Python AI Worker 只做模型 / 算法 / eval 候选层，Go 仍负责控制面和事实边界。

可用多个 sub-agent 做互不重叠任务；主 agent 负责集成、检查和文档同步。不回滚用户已有修改。新发现的待办写入 docs/runbook/remaining-goals.md。

门禁按风险分层：小改只跑相关测试 / 文档脚本；跨服务、生成代码、migration、service-registry、Docker/compose、安全边界或提交推送前才跑完整 check-local。
```

## 文档路由

- 本文件只维护 Codex 目标框短 Prompt 和每轮文档路由；具体目标见 `docs/runbook/current-goal.md`。
- `agent.md` 决定按需读取和维护哪些项目文档；阶段细节见 `docs/runbook/current-brief.md`。
- 未完成工作见 `docs/runbook/remaining-goals.md`；单服务状态见 `docs/runbook/service-briefs/<service>.md`。

## 工作原则

1. 主线阶段以 `current-goal.md` 和 `current-brief.md` 为准；不要在本文件重复维护长状态。
2. 小切片闭环：设计、代码、必要测试、文档一起收；默认跑相关局部门禁，不频繁跑完整 `check-local`。
3. 降低耦合并控制复杂度：不跨服务读内部表，不引入网状同步 RPC，接近行数阈值就拆同 package 文件。
4. 新服务和中间件不写死；满足独立模型 / 伸缩 / 故障 / 安全边界或明显降复杂度时通过 ADR 新增。
5. 可用多个 sub-agent，但必须拆分互不重叠职责；主 agent 负责集成和最终检查。
6. 压测原始数据放 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
7. 新发现待办写入 `docs/runbook/remaining-goals.md`；不回滚用户已有修改。
