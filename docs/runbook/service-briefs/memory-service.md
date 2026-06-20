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
- profile overgeneralization eval case + local fixture adapter：单条群聊事实不能升级为 ACTIVE profile，profile candidate 必须保留 GROUP scope 且 PENDING_REVIEW
- group memory fixture eval：ACTIVE 需要 source refs，validity window 必须保留，superseded memory 不能作为 current fact

下一步：补 runtime projection/query smoke 覆盖 source-ref、validity window 和 supersession。第一版仍不做 LLM extraction，不把单条群消息直接升级为 ACTIVE profile fact。
