# memory-service

状态：future / SDD v0.1 draft，当前只落设计、proto 和 PostgreSQL projection migration；尚未创建 `services/memory-service` runtime 目录。

定位：group memory / StructuredMemoryEvent / Memory Graph / ProfileAggregate 投影服务。它消费 message / member / domain events，维护带 source refs、speaker / audience scope、validity window、supersession、confidence 和 review state 的协作记忆，不做 RAG 回答，不执行 Agent 动作，不替代 search / policy / message facts。

当前已落：`docs/sdd/memory-service.md`、`api/proto/nexusim/memory/v1/memory_service.proto`、`migrations/postgres/memory/000001_memory_core.sql`。

下一步：切换到 implementation slice 时再把 service-registry stage 提升为 `foundation-active`，创建六层 skeleton、cmd runtime、domain/types/app validation、PostgreSQL repository tests、timeline projection usecase 和 focused checks。
