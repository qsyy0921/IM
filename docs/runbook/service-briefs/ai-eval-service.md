# ai-eval-service

状态：foundation-active / eval catalog + cross-group temporal stack gate passed.

定位：AI eval harness 的低敏持久化目录。它保存 eval run 的 suite、stage、
adapter、状态、计数、summary/report 引用和低敏 metadata。

当前已落：

- proto、migration、六层 skeleton、`RecordEvalRun` / `GetEvalRun` / `ListEvalRuns`、PostgreSQL repository、`grpc` runtime 和 observability wiring。
- `ai-eval-record-smoke` 与 `run-ai-eval-record-run-smoke.ps1`：profile /
  Agent safety summary -> gRPC `RecordEvalRun` -> `GetEvalRun` / `ListEvalRuns`。
- `run-ai-eval-regression-gate-smoke.ps1`、`gate-policy.local.json` 和
  `validate-ai-eval-gate-policy.ps1`：声明必跑 adapter、阈值、Get / List
  读回、禁止持久化字段和可选 service-stack adapter。
- gate runner 支持 optional adapters；Python path 和 RAG / Agent service-stack live gate 已登记到 catalog。
- `check-ai-eval-regression-gate.ps1` 已接入 `check-local`，只跑 CI-safe
  required adapters，不启动 Docker / PostgreSQL / live RAG-Agent stack。
- Case catalog 70；current-memory service-stack live gate 38/38 passed；cross-group / temporal memory fixture eval、retrieval smoke、RAG / Summary / Agent stack smokes 和 40/40 optional stack gate 已落。
- 2026-06-23 低敏 collaborative-memory eval 扩到 20 个 profile / Agent safety fixture cases，
  新增 multi-hop actor-chain completeness、workstream / decision dependency edge、
  reviewed multi-source profile activation、supporting-memory delete 后 profile recompute
  检查；该 gate 不调用模型、数据库或业务服务。

边界：不保存 raw EvidencePack、prompt、model output、用户正文、secret 或 tool input；不授权业务动作。

下一步：把新增 collaborative-memory fixture 覆盖逐步提升到 memory-service / retrieval /
RAG / Agent live stack adapter，继续区分 retrieval failure、memory lifecycle failure
和 reasoning failure。
