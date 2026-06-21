# NexusIM

NexusIM 是面向企业协同场景的分布式 IM + AI 协作平台。当前仓库已经从普通 IM demo 推进到：

```text
本地 / 双机可运行的分布式 IM 后端
-> group memory / EvidencePack / RAG / summary / Agent 应用底座
-> skill registry / MCP gateway / action executor / proposal approval / audit
```

GitHub 首页只放当前总览。每轮 Codex 继续开发时，目标框只复制 [prompt.md](prompt.md) 中的短 Prompt；具体进度入口看 [agent.md](agent.md)、[current-brief.md](docs/runbook/current-brief.md) 和 [remaining-goals.md](docs/runbook/remaining-goals.md)。

## 当前状态

已进入真实链路的 9 个 IM 后端服务：

| 服务 | 作用 |
| --- | --- |
| `api-gateway` | 外部入口、认证透传、租户 quota、legacy descriptor 迁移门禁。 |
| `identity-service` | 登录、Refresh、MFA、recovery code、JWKS、challenge delivery。 |
| `message-service` | 发消息、编辑 / 撤回 / 删除、timeline / outbox。 |
| `conversation-service` | 会话、成员边界、owner transfer、发送上下文。 |
| `delivery-service` | durable inbox、`PullInbox`、`AckDelivery`、delivery outbox。 |
| `push-gateway` | WebSocket 在线通知、Redis route、resume / PullInbox fallback。 |
| `receipt-service` | 已读 / 送达回执、会话列表、未读 / 置顶 / 静音等读模型。 |
| `contacts-service` | 联系人请求、隐私策略、分组 / 搜索、来源风险。 |
| `policy-service` | 策略决策、ReBAC first path、moderation、tenant quota、tool policy precheck。 |

已启动并持续推进的 AI / Agent 应用底座：

| 服务 / 模块 | 当前状态 |
| --- | --- |
| `search-service` | 搜索 projection、visibility / tombstone、`SearchMessages`、timeline consumer、projection smoke。 |
| `memory-service` | group memory projection、StructuredMemoryEvent、source refs、visibility window、revoke hidden。 |
| `retrieval-gateway` | EvidencePack 统一边界，聚合 search / memory / policy precheck，不直接调用 LLM。 |
| `rag-service` | 只读问答 first path、EvidencePack citation verifier、guarded external HTTP LLM boundary。 |
| `summary-service` | 只读摘要 first path、EvidencePack citation verifier、guarded external HTTP LLM boundary。 |
| `agent-service` | proposal-only path、mcp-gateway prepare、approval workflow、approval outbox relay、planner Python candidate guard。 |
| `skill-registry` | 技能目录、输入输出合约、风险等级、审批要求和审计元数据。 |
| `mcp-gateway` | tool prepare 边界、skill catalog check、policy precheck、低敏 audit，不直接执行外部工具。 |
| `action-executor` | approved execution audit、proposal / approval / prepare audit 校验、本地安全 adapter、guarded external HTTP provider adapter、eval smoke。 |
| `ai/python` | Python AI Worker 候选层：contract guard、低敏 safety guard、candidate-only worker CLI、`IM` conda toolchain。 |

已启动的客户端平台：

| 模块 | 当前状态 |
| --- | --- |
| `clients/` | Browser / PC / Android client platform first slice：`protocol`、`client-core`、Web shell、PC desktop shell contract 和 Android runtime contract 已建立并通过 focused validation；`api-gateway` client BFF first-stage HTTP/JSON surface 已落；Web BFF fetch / push WebSocket / IndexedDB local store adapters 已接；客户端只连 `api-gateway` / `push-gateway`，PullInbox 是消息事实源，WebSocket 只做在线唤醒。 |

当前默认主线不是继续泛化清理 9 服务 P2 backlog，而是先推进客户端平台 MVP：
Web client -> `api-gateway` client BFF -> `push-gateway` WebSocket 的局域网 smoke，
然后复用同一 core 接 Windows PC 和 Android。AI 大模型应用底座作为后续主线保留：

```text
group memory
-> EvidencePack
-> RAG / summary
-> multi-agent
-> skill-registry
-> MCP/tool gateway
-> action-executor
-> proposal / approval / audit
-> ai-eval
```

下一步默认看 [current-goal.md](docs/runbook/current-goal.md)。截至当前主线，下一步是跑 Web client -> client BFF -> push-gateway 的局域网 Web MVP smoke；随后补 BFF HTTP 层 metrics / rate-limit，并复用同一 core 接 Windows PC 和 Android。

## 不变量

- PostgreSQL 是交易事实源；Kafka 是事件传播面；业务事务不能直接 publish Kafka，必须走 outbox relay。
- RAG / summary / Agent 只能消费 EvidencePack，不能直接读 message / conversation / private tables。
- Agent 写动作必须走 policy、tool policy、proposal、approval、action-executor 和 audit。
- Python 只做 LLM / embedding / rerank / memory extraction / planner / eval 候选层；Go 负责控制面、权限、状态、审计和持久化。
- push-gateway 不拥有 durable inbox；断线和慢连接恢复以 delivery-service `PullInbox` 为准。
- 新服务和中间件不写死；只有当独立数据模型、伸缩、故障、安全边界或复杂度收益成立时才通过 ADR 新增。
- 压测 / smoke 原始输出放 `H:\NexusIM\loadtest-results`，仓库只保存低敏报告、summary 和索引。

## 目录结构

| 目录 | 作用 |
| --- | --- |
| `api/` | 同步接口契约。`api/proto/` 存放 gRPC Protobuf。 |
| `schemas/` | 异步事件契约。`schemas/kafka/` 存放 Kafka topic 的 Protobuf schema。 |
| `services/` | Go 服务实现。每个服务统一使用 `api / app / domain / infrastructure / types / trigger` 六层目录。 |
| `ai/python/` | Python AI Worker 候选层代码和 `IM` conda 环境配置。 |
| `migrations/` | PostgreSQL migration，按服务归档。 |
| `deploy/` | 本地 Docker、观测、服务编排和运行配置。 |
| `loadtest/` | smoke / loadtest runner。原始结果默认写到 H 盘。 |
| `docs/` | 架构、SDD、ADR、runbook、面试叙事和证据索引。 |
| `tools/` | 生成、门禁、smoke、evidence manifest、capacity / observability 辅助脚本。 |

## 文档入口

| 文档 | 用途 |
| --- | --- |
| [prompt.md](prompt.md) | Codex 目标框短 Prompt 的唯一维护源。 |
| [agent.md](agent.md) | Codex / sub-agent 每轮读取和维护文档的路由规则。 |
| [docs/runbook/current-brief.md](docs/runbook/current-brief.md) | 低 token 当前阶段入口。 |
| [docs/runbook/current-goal.md](docs/runbook/current-goal.md) | 当前 active slice。 |
| [docs/runbook/development-progress.md](docs/runbook/development-progress.md) | 当前开发进度总览。 |
| [docs/runbook/remaining-goals.md](docs/runbook/remaining-goals.md) | 只记录还没有完成的工作。 |
| [docs/runbook/service-briefs/README.md](docs/runbook/service-briefs/README.md) | 服务 brief 索引。 |
| [docs/architecture/target-architecture.md](docs/architecture/target-architecture.md) | 总体目标架构。 |
| [docs/architecture/target-architecture-ai.md](docs/architecture/target-architecture-ai.md) | AI / RAG / Agent 目标架构。 |
| [docs/runbook/ai-eval/README.md](docs/runbook/ai-eval/README.md) | AI eval case schema、adapter 和运行入口。 |

## 六层 DDD 约束

服务目录统一为：

```text
services/<service-name>/
  cmd/
  internal/
    api/
    app/
    domain/
    infrastructure/
    types/
    trigger/
```

允许依赖方向：

```text
api -> app/types
trigger -> app/types
app -> domain/infrastructure/types
domain -> types
infrastructure -> domain/types
```

禁止方向：

```text
domain -> infrastructure/api/trigger
infrastructure -> api/trigger
types -> app/domain/infrastructure/api/trigger
```

说明：

- `api` 只做 gRPC/HTTP 适配和 request/response 转换。
- `app` 编排 use case、事务和 port。
- `domain` 表达领域规则，不依赖 SQL、Kafka、Redis 或外部 SDK。
- `infrastructure` 实现 PostgreSQL、Kafka、Redis、外部 RPC client 和 provider adapter。
- `trigger` 放 outbox relay、Kafka consumer、定时巡检和补偿任务。

## 常用命令

生成 Protobuf：

```powershell
. .\tools\go-env.ps1
.\tools\gen-proto.ps1
```

启动 / 停止本地依赖：

```powershell
make local-up
make local-down
```

运行聚焦 Go 测试：

```powershell
. .\tools\go-env.ps1
go test ./services/action-executor/... -count=1
```

使用 Python AI Worker 环境：

```powershell
conda activate IM
cd ai\python
python -m pytest
python -m ruff check .
python -m mypy nexusim_ai_common scripts tests
```

运行当前 AI eval adapter：

```powershell
. .\tools\go-env.ps1
.\tools\validate-ai-eval-cases.ps1
.\tools\run-ai-eval-action-external-adapter.ps1
.\tools\run-ai-eval-profile-agent-safety.ps1
```

完整本地门禁只在跨服务、生成代码、migration、service-registry、Docker/compose、安全边界或提交推送前需要：

```powershell
.\tools\check-local.ps1
```

## 当前不是已完成项

以下内容仍属于后续 hardening 或产品化，不要在面试或文档里说成已经生产级完成：

- 生产级 Redis / Kafka / PostgreSQL HA、长时间 fault campaign、split-brain fencing、生产 sizing。
- 统一生产观测平台、Alertmanager 路由、日志汇聚、SLO 和长期 retention。
- provider-grade OIDC / KMS / HSM / email / SMS / WebAuthn / complete risk engine。
- provider-grade ReBAC DSL、外部 audit sink、运维 UI、批量 repair 审批系统。
- 完整 Web / App / 桌面客户端；当前已启动 Web-first client platform first slice，
  且 `api-gateway` client BFF first-stage surface 和 Web adapters first path 已落，
  但还缺局域网 Web MVP smoke、Windows installer 和 Android APK。
- 完整 media / notification / admin / audit 等产品化平台服务。

当前最准确表述：

```text
NexusIM 已完成 9 个 IM 后端服务的主链路和一批本地 / 双机分布式 smoke，
并正在这些事实源、投递、策略和审计边界之上构建 AI / RAG / Agent 应用底座。
```
