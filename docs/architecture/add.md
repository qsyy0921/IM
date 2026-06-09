# NexusIM ADD v1.0

ADD（Architecture Design Document）描述系统级业务架构、服务边界和关键业务链路。本文不展开具体 SQL、代码目录和部署参数；这些内容进入 TADD、SDD、契约和 migration。

## 1. 系统定位

NexusIM 是面向企业协同的 IM + 智能协作平台。

核心目标：

- 提供 Web 端和桌面端即时通信能力；
- 保证消息写入可靠、会话内顺序稳定、客户端重试幂等；
- 支持在线投递、离线补拉、多端同步、ACK、已读和未读；
- 通过 Kafka 事件流扩展搜索、RAG、Agent、审计和补偿；
- 通过权限、审批和审计约束 RAG/Agent 的访问和写动作。

核心原则：

```text
IM 写入是事实源。
Kafka 是事件传播面。
Search/RAG/Agent 是异步投影和智能协作层。
权限与审计是所有智能能力的边界。
```

## 2. 架构范围

当前阶段只交付：

- Web 端基础聊天；
- `message-service SendMessage` 普通会话写入；
- PostgreSQL 本地事务；
- message outbox；
- Kafka publish path；
- 本地压测和幂等测试。

当前阶段不交付：

- 移动端；
- 完整 delivery/push 闭环；
- 热点 sequencer 生产实现；
- RAG 和 Agent 业务闭环；
- 多 Region 生产部署。

## 3. 参与方

| 参与方 | 说明 |
| --- | --- |
| Web Client | 浏览器端聊天入口 |
| Desktop Client | 后续桌面客户端，复用 Web/HTTP/WebSocket 协议 |
| api-gateway | HTTP/OpenAPI 入口和协议适配 |
| push-gateway | WebSocket 长连接和推送入口 |
| contacts-service | 联系人 / 好友关系事实源 |
| message-service | 消息事实源服务 |
| conversation-service | 会话、成员、权限版本事实源 |
| delivery-service | fanout、inbox、offline pull |
| receipt-service | ACK、read cursor、unread |
| search-service | OpenSearch 唯一写入口 |
| retrieval-gateway | RAG/Search 唯一检索入口 |
| agent-service | 智能任务编排 |
| approval-service | 高风险写动作审批 |
| audit-service | 不可变审计和修复留痕 |

## 4. 服务边界

| 服务 | 拥有事实 | 职责 | 禁止事项 |
| --- | --- | --- | --- |
| identity-service | user、device、session | 登录、设备、token | 不维护会话成员 |
| contacts-service | contact request、contact edge | 好友申请、联系人列表、联系人事件 | 不维护会话成员，不自动创建会话 |
| conversation-service | conversation、member、permission version | 会话和成员边界 | 不写消息 |
| message-service | message、timeline、outbox | 消息写入和变更 | 不做推送、搜索、RAG |
| delivery-service | inbox、delivery cursor | fanout、离线补拉 | 不修改 message fact |
| receipt-service | read cursor、unread projection | ACK、已读、未读 | 不作为消息事实源 |
| search-service | search index projection | 搜索索引写入 | 其他服务不直写 OpenSearch |
| retrieval-gateway | evidence pack audit | ACL 过滤、召回、rerank | Agent/前端不直连索引 |
| agent-service | agent run、proposal | 只读问答和写动作提案 | 不绕过审批写业务库 |
| audit-service | audit log、manifest | 审计、导出、修复留痕 | 不作为业务状态源 |

## 5. 总体业务流

### 5.1 发送消息

```text
Client
-> api-gateway / push-gateway
-> message-service
-> PostgreSQL transaction:
   conversation_seq
   message_log
   conversation_timeline_events
   message_outbox
-> outbox relay
-> Kafka conversation.timeline.events
-> delivery/search/rag/agent/audit consumers
```

发送成功只表示消息已可靠落库，不表示所有接收方已经收到。

### 5.2 离线补拉

```text
Client reconnect
-> auth context
-> device last_received_seq
-> delivery-service query inbox/cursor
-> message read model or message-service batch get
-> return missing messages ordered by conversation_seq
```

### 5.3 RAG 问答

```text
User query
-> retrieval-gateway
-> policy check / ACL scope
-> OpenSearch + vector recall
-> rerank
-> EvidencePack
-> LLM answer with citations
-> audit
```

RAG 不允许绕过 retrieval-gateway 直接访问消息、搜索或向量库。

### 5.4 Agent 写动作

```text
Agent plan
-> action proposal
-> policy check
-> approval
-> action-executor
-> audit
```

Agent 不能直接执行高风险写动作。

## 6. 一致性边界

强一致边界：

```text
message-service 本地事务:
conversation_seq + message_log + conversation_timeline_events + message_outbox
```

最终一致边界：

```text
delivery
receipt projection
search index
rag chunks / embeddings
agent triggers
audit export
```

跨服务不使用分布式事务，通过 outbox、Kafka、幂等消费者、retry、DLQ 和 replay 保证最终一致。

## 7. DDD 限界上下文

| 上下文 | 服务 | 聚合 / 模型 |
| --- | --- | --- |
| 身份上下文 | identity-service | User、Device、Session |
| 联系人上下文 | contacts-service | ContactRequest、ContactEdge |
| 会话上下文 | conversation-service | Conversation、Member、PermissionVersion |
| 消息上下文 | message-service | Message、TimelineEvent、OutboxEvent |
| 投递上下文 | delivery-service | Inbox、DeliveryCursor、FanoutTask |
| 回执上下文 | receipt-service | ReadCursor、UnreadProjection |
| 搜索上下文 | search-service | SearchDocument、IndexTask |
| 检索上下文 | retrieval-gateway | EvidencePack、EvidenceItem |
| Agent 上下文 | agent-service | AgentRun、Plan、Proposal |
| 审批上下文 | approval-service | ApprovalTask、Decision |
| 审计上下文 | audit-service | AuditLog、AuditManifest |

## 8. 阶段路线

| 阶段 | 目标 |
| --- | --- |
| Phase 0 | 工程基线、Docker Compose、migration、proto/schema 生成 |
| Phase 1 | `message-service SendMessage` 主写链路 |
| Phase 1.5 | WebSocket / push / delivery 最小闭环 |
| Phase 2 | 媒体、搜索、归档 |
| Phase 3 | RAG 知识库和检索网关 |
| Phase 4 | Agent、审批和动作执行 |
| Phase 5 | 多 Region、治理、成本、安全评测 |

## 9. 当前编码结论

可以开始编码，但第一条代码切片只做：

```text
message-service SendMessage
-> PostgreSQL local transaction
-> message_outbox
-> outbox relay
-> Kafka publish path
```

完整 IM 闭环需要在 `delivery-service` 和 `push-gateway` SDD 冻结后继续推进。
