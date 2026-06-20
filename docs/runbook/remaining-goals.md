# NexusIM Remaining Goals

这份文档只记录当前还没有完成的工作。当前进度总览见
`development-progress.md`，单服务事实见 `service-briefs/<service>.md`。

维护规则：

- 新发现的待完成工作追加到本文件。
- 已完成工作从本文件移除，并同步到对应 service brief / progress / smoke report。
- 不记录已完成证据，不写长历史，不替代 SDD / ADR。

## 当前默认主线

AI 大模型应用底座仍是默认开发主线：

```text
search-service -> memory-service -> retrieval-gateway
-> rag-service / summary-service -> agent-service
-> skill-registry -> mcp-gateway -> action-executor -> ai-eval-service
```

9 个既有 IM 服务只做阻塞 AI 主线的必要收口；生产级 HA、长压、sizing、
provider-grade 运维和完整系统测试暂不作为当前转进阻塞。

## 当前未完成重点

1. AI eval 回归扩展：
   `ai-eval-service` 已能记录低敏 summary，并已具备 profile / Agent safety、
   action-executor external adapter 和 Python worker 的 policy-driven
   multi-adapter gate smoke，也已跑通 RAG / Agent service-stack live gate 和
   CI-safe gate skeleton。RAG / Agent / Summary / Python / action / memory
   group safety fixture扩展已落；仍不得保存 raw prompt、
   EvidencePack、model output、用户正文、secret 或 tool input。
   后续 eval case 必须低敏，可复核，能区分 retrieval failure、reasoning failure 和 action boundary failure。

2. Memory / retrieval 深化：
   `memory-service` 继续按 2025/2026 group memory / collaborative memory 论文
   方向深化 source refs、speaker / audience scope、valid_from / valid_to、
   supersedes / contradicts、confidence、PENDING / ACTIVE / SUPERSEDED /
   REJECTED 状态。`QueryMemoryEvents.at_conversation_seq`、runtime checks 和
   retrieval-gateway EvidencePack current-only memory query，以及 RAG /
   Summary / Agent API 显式透传 `at_conversation_seq`，以及 RAG / Summary /
   Agent current-memory consumption CI-safe regression 和 memory extraction
   confidence / review eval 已落；后续补 current-memory service-stack live
   smoke，并继续保留 source-ref / visibility / review 边界。

3. Agent 真实业务动作扩展：
   `agent-service`、`skill-registry`、`mcp-gateway`、`action-executor` 已具备
   first path。后续接真实 MCP / provider tool 或业务写动作时，仍必须走：
   policy precheck -> skill contract -> prepare audit -> proposal -> approval
   -> executor -> low-sensitive result projection -> audit。高风险动作第一阶段
   禁止自动执行。

4. Python AI Worker 扩展：
   `rag-service`、`summary-service`、`agent-service` 已接 candidate guard。
   后续可扩 embedding / rerank / memory extraction / planner / eval 候选，但
   Python 只返回候选和 hash / citation metadata；Go 继续拥有权限、审计、
   状态和持久化。

## 9 个现有服务必要收口

这些不是默认下一步，只有阻塞 AI 主线或用户点名时才进入当前切片：

| 服务 | 未完成工作 |
| --- | --- |
| `api-gateway` | legacy observation window 目标环境证据、provider-grade 配置中心 quota 控制面、灰度治理、生产观测。 |
| `identity-service` | WebAuthn/passkeys、OIDC federation、多 issuer、KMS/HSM、完整风控、生产级 email/SMS provider。 |
| `message-service` | 会话级删除策略深化、provider-grade 外部 proof 工作流、发送链路生产观测；媒体二进制后续由 media 能力承担。 |
| `conversation-service` | 更完整群管理、owner transfer 策略深化、完整历史窗口 / targeted replay repair。 |
| `delivery-service` | 更多 delivery event 消费方、projection repair 深化、容量曲线。 |
| `push-gateway` | 生产级 Redis HA 设计、跨实例 resume 深化、长时间在线容量曲线。 |
| `receipt-service` | 会话列表更多产品能力、更多摘要策略和容量曲线。 |
| `contacts-service` | 组织级策略、租户默认值、来源策略、隐私例外接入 admin/config service。 |
| `policy-service` | provider-grade ReBAC graph / DSL、moderation / risk scoring、tenant DSL / quota、外部 audit pipeline。 |

## 后置平台 / 产品化服务

这些服务可以后续新增，不是当前阻塞项：

- `media-service`
- `notification-service`
- `audit-service`
- `admin-service`
- Web / App / 桌面端展示层

新增服务必须满足独立数据模型、独立伸缩需求、独立故障边界、独立安全边界，
或能显著降低现有服务复杂度，并通过 ADR。

## 后置 Hardening

- 生产级统一观测：collector、Alertmanager 路由、日志汇聚、SLO、retention。
- 分布式 HA / 故障演练：Redis / Kafka / PostgreSQL 更长时长和多故障组合。
- Repair / DLQ / audit 产品化：审批系统、运维 UI、批量 repair、外部审计。
- 容量和复杂度治理：9 服务长压 campaign、资源曲线、生产 sizing、文件拆分。

## Sub-Agent 规则

可使用多个 sub-agent 并行推进，但必须按服务、文档集、测试面或只读审查问题
拆分互不重叠职责。禁止多个 sub-agent 同时修改同一 proto、migration、service
brief 或架构章节。主 agent 负责集成、检查和关闭 stale sub-agent。
