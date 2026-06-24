# summary-service

状态：foundation-active / provider boundary + cross-group temporal stack smoke passed.

定位：会话摘要边界服务。它只消费 retrieval-gateway 返回的 `EvidencePack`，
不直接读 message / conversation / search / memory 私有表，不执行 Agent 动
作，不绕过 policy / visibility。

当前已落：

- `summary_service.proto`、SDD、六层 skeleton、`grpc` runtime、metrics、Docker / observability wiring
- app usecase 调用 retrieval port 和 `SummaryProvider` port；默认本地
  extractive provider 基于 EvidencePack 生成 deterministic summary
- 已补 guarded external HTTP LLM boundary：只由 EvidencePack 构造 prompt；
  provider failure 返回稳定 unavailable，unsafe / malformed output fail closed
- 已补可选 `python-worker` provider mode：Go 先生成 grounded summary，Python
  worker 只返回 candidate hash / citations；Go 校验 id、hash 和 citation
- response 保留 citations、EvidencePack、`generated_by_llm=false`；provider 输出后统一运行 citation verifier
- retrieval-gateway 公开 proto RPC client、app / gRPC / cmd focused tests
- `loadtest/summary`、`tools/run-summary-adapter-smoke.ps1` 和真实本地
  `retrieval-gateway -> summary-service` adapter smoke 已通过
- `at_conversation_seq` 已透传到 EvidencePack current-memory query；CI-safe regression 和 2026-06-20 live smoke 均验证 stale memory 不作为 current citation。
- 2026-06-20 cross-group / temporal stack smoke 已验证跨群 source refs /
  speaker attribution 被保留，expired / superseded / future memory 不进入
  current EvidencePack。
- 2026-06-23 Summary live adapter 已增加 multi-hop actor/source-chain completeness
  断言；仍只基于 retrieval-gateway 返回的 EvidencePack 与 citation verifier
  判断，不直接读 memory / search 私表。
- 2026-06-24 Summary EvidencePack graph edge 透传已落：retrieval client 会保留
  `EvidenceMemoryGraphEdge`，gRPC response 会继续向调用方返回该字段；service
  仍只基于 EvidencePack 与 citation verifier 工作。
- 2026-06-24 Summary EvidencePack profile evidence 透传已落：retrieval client 会保留
  `PROFILE_AGGREGATE` evidence 的 profile subject、aggregate type/key、
  supporting memory ids 和时间字段；gRPC response 会继续向调用方返回这些字段。
  service 仍只基于 EvidencePack 与 citation verifier 工作。

下一步：

- 真实服务栈启动后与 memory-service / retrieval-gateway adapter 一起跑完整
  optional gate；之后扩展 temporal update / profile recompute 和更完整
  group-memory 摘要场景，provider 仍走 port、guard、hash / citation 校验和 verifier。
