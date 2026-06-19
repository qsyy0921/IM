# ai-eval-service

状态：foundation-active / first persistent eval run catalog.

定位：AI eval harness 的低敏持久化目录。它保存 eval run 的 suite、stage、
adapter、状态、计数、summary/report 引用和低敏 metadata，方便后续把本地 eval
脚本接成可查询、可审计、可回归的服务边界。

当前已落：

- `ai_eval_service.proto`
- `migrations/postgres/ai-eval-service/000001_ai_eval_core.sql`
- 六层 skeleton：api / app / domain / infrastructure / types / cmd
- `RecordEvalRun`、`GetEvalRun`、`ListEvalRuns`
- PostgreSQL repository、真实 PG 集成测试
- `grpc` runtime mode、debug `/metrics`
- Docker runtime、local compose、Prometheus rules、Grafana dashboard、
  `service-registry.json` wiring

边界：

- 不运行 eval。
- 不调用 LLM / Python worker / tool provider。
- 不保存 raw EvidencePack、raw prompt、raw model output、用户正文、secret 或
  tool input。
- 不授权业务动作；真实动作仍走 policy、proposal / approval、executor 和 audit。

下一步：

- 把现有 AI eval scripts 的 summary 通过 `RecordEvalRun` 写入 catalog。
- 补本地 smoke：写入一个 completed run，再通过 Get/List 读回。
- 后续再做 suite aggregation / CI gate，不把第一版 catalog 夸大成生产级 eval 平台。
