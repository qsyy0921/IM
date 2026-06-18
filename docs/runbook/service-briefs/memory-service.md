# memory-service

状态：foundation-active / implementation slice。`memory_service.proto`、PostgreSQL projection migration 和 `services/memory-service` 六层 runtime skeleton 已开始落地。

定位：group memory / StructuredMemoryEvent / Memory Graph / ProfileAggregate 投影服务。它消费 message / member / domain events，维护带 source refs、speaker / audience scope、validity window、supersession、confidence 和 review state 的协作记忆，不做 RAG 回答，不执行 Agent 动作，不替代 search / policy / message facts。

当前已落：

- `docs/sdd/memory-service.md`
- `api/proto/nexusim/memory/v1/memory_service.proto`
- `migrations/postgres/memory/000001_memory_core.sql`
- `services/memory-service` 六层 skeleton、`grpc` runtime mode、`timeline-consumer` runtime mode、debug `/metrics`
- domain / types / app validation、PostgreSQL repository first pass、timeline projection usecase skeleton
- registry / Docker runtime / local compose / Prometheus / Grafana foundation-active wiring

下一步：补真实 PostgreSQL repository integration tests、timeline worker 单测、focused projection smoke。第一版仍不做 LLM extraction，不把单条群消息直接升级为 ACTIVE profile fact。
