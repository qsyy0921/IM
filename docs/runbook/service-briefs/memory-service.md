# memory-service

状态：foundation-active / first projection smoke passed。`memory_service.proto`、PostgreSQL projection migration、六层 runtime 和 focused projection smoke 已落。

定位：group memory / StructuredMemoryEvent / Memory Graph / ProfileAggregate 投影服务。它消费 message / member / domain events，维护带 source refs、speaker / audience scope、validity window、supersession、confidence 和 review state 的协作记忆，不做 RAG 回答，不执行 Agent 动作，不替代 search / policy / message facts。

当前已落：

- `docs/sdd/memory-service.md`
- `api/proto/nexusim/memory/v1/memory_service.proto`
- `migrations/postgres/memory/000001_memory_core.sql`
- `services/memory-service` 六层 skeleton、`grpc` runtime mode、`timeline-consumer` runtime mode、debug `/metrics`
- domain / types / app validation、PostgreSQL repository first pass、timeline projection usecase、PG integration tests、timeline worker tests
- registry / Docker runtime / local compose / Prometheus / Grafana foundation-active wiring
- `loadtest/memory` 和 clean projection smoke：member join -> message persisted -> PENDING StructuredMemoryEvent + source ref -> Query/Get -> revoke hidden

下一步：补 memory extraction / review / profile aggregate hardening，或进入 `retrieval-gateway` / EvidencePack。第一版仍不做 LLM extraction，不把单条群消息直接升级为 ACTIVE profile fact。
