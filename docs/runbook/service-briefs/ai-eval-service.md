# ai-eval-service

状态：foundation-active / first persistent eval run catalog.

定位：AI eval harness 的低敏持久化目录。它保存 eval run 的 suite、stage、
adapter、状态、计数、summary/report 引用和低敏 metadata。

当前已落：

- proto、migration、六层 skeleton、`RecordEvalRun` / `GetEvalRun` /
  `ListEvalRuns`、PostgreSQL repository、`grpc` runtime 和 Docker / observability
  wiring。
- `ai-eval-record-smoke` 与 `run-ai-eval-record-run-smoke.ps1`：profile /
  Agent safety summary -> gRPC `RecordEvalRun` -> `GetEvalRun` / `ListEvalRuns`。
- `run-ai-eval-regression-gate-smoke.ps1`、`gate-policy.local.json` 和
  `validate-ai-eval-gate-policy.ps1`：声明必跑 adapter、阈值、Get / List
  读回、禁止持久化字段和可选 service-stack adapter。
- gate runner 已支持显式 optional adapters；Python optional path 和 RAG /
  Agent service-stack live gate 已通过并登记到 catalog。
- `check-ai-eval-regression-gate.ps1` 已接入 `check-local`，只跑 CI-safe
  required adapters，不启动 Docker / PostgreSQL / live RAG-Agent stack。
- RAG / Agent live gate 已扩到 17 cases：version preservation、prepare audit、
  tool policy metadata、hash-only action audit；profile fixture 已扩到 6 cases。

边界：不运行 eval，不调用 LLM / Python worker / tool provider。
- 不保存 raw EvidencePack、raw prompt、raw model output、用户正文、secret 或
  tool input。
- 不授权业务动作；真实动作仍走 policy、proposal / approval、executor 和 audit。

下一步：补 service-stack negative RAG / Agent cases。
