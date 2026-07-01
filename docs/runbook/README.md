# NexusIM Runbook Index

短路由页。不要把历史结论、压测详情或长设计写到这里。

## 当前主线

- 当前 active slice 见 `current-goal.md`；本轮默认只读必要文档。
- 当前 workspace 是 Agent Lab，主线是 Agent / RAG / memory / Python AI Worker /
  EvidencePack / eval gate 的探索、SDD 和后续 fixture/eval。
- 第一阶段不使用真实 NexusIM IM 数据；先用公开数据集和 synthetic IM-like fixture。
- 当前不写 proto、OpenAPI、Kafka schema、migration、production service directory 或
  production startup path。
- 后端 hotgroup 压测、Docker runtime profile、性能实验和完整生产就绪测试不属于本工作区。

## 默认入口

- Codex 文档路由入口：`../../prompt.md`
- Agent 进度管理规则：`../../agent.md`
- 具体执行目标：`current-goal.md`
- 每轮短入口：`current-brief.md`
- 剩余工作：`remaining-goals.md`
- 协作线程边界：`codex-sessions.md`
- Agent Lab 开发过程：`development-process.md`
- Agent Lab 进度总览：`development-progress.md`

## 按需读取

- Agent 平台级 SDD：`../sdd/agent-platform.md`
- Agent 初步设计：`../architecture/agent-plane-initial-design.md`
- Agent 研究输入：`../research/agent-plane-redesign-20260701.md`、
  `../research/agent-ecosystem-research-20260701.md`、
  `../research/agent-system-complete-scope-20260701.md`
- 完整目标架构：`../architecture/target-architecture-complete.md`
- 服务级设计：`../sdd/<service>.md`
- AI eval / evidence manifests：`ai-eval/README.md`、`*-evidence.json`
- 历史客户端 / Docker / loadtest / repair 资料：只在用户点名或主集成需要时读取。
- 历史长文档：`archive/`、`history/`

## 维护规则

- 入口和索引保持短；每轮只读取本轮必要文档。
- 历史事实写入 archive / history / loadtest 报告，不回填到入口文档。
