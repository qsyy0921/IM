# NexusIM Current Goal

本文是 NexusIM 的短目标索引。每轮默认只读 `docs/runbook/current-brief.md`；只有需要长期原则、路线图、评审规则或历史索引时，才读取本文。历史流水已归档到 `docs/runbook/history/current-goal-archive-20260611.md`，不要每轮全文读取归档。

## 0. 可复制短 Goal Prompt

```text
持续推进 E:\development\IM 的 NexusIM 项目。

每轮开始：
1. 执行 git status --short --branch。
2. 读取 docs/runbook/current-brief.md。
3. 不要每轮全文读取 current-goal.md、SDD、压测报告或历史文档。
4. 需要哪类信息，就按关键词读取对应文档的相关片段：长期目标查 current-goal，服务设计查 docs/sdd/<service>.md，压测证据查 docs/runbook/loadtest/<service>/，实现细节用 rg 定位代码。
5. 按 brief 和按需读取到的当前目标、硬边界、下一步优先级继续工作。
6. 不回滚用户已有修改。

工作原则：
1. 优先把系统链路做完整，不把主要时间消耗在重型压测矩阵上。
2. 除非用户明确要求，不再把流量诊断、代理用量归因、外网消耗排查列为当前任务；日常开发只保留“少下载、用已有镜像/依赖、服务间走本地有线”的约束。
3. 每个微服务独立使用六层 DDD：api / app / domain / infrastructure / types / trigger。
4. Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
5. 优先降低微服务耦合、控制代码复杂度：不跨服务读取内部表，不引入网状依赖，不为短期功能增加不必要同步 RPC、公共包或抽象层。
6. 单个切片保持小闭环：契约 / migration / 本地事务 / consumer 或 relay / smoke 分阶段推进，不一次性横跨多个产品能力。
7. 开发过程中主动使用可用 sub-agent 做设计、实现、测试、文档或风险复核；任务完成后及时关闭 sub-agent。
8. 有意义的切片完成后运行必要检查，更新 current-brief.md；阶段状态变化时同步 current-goal.md、对应 SDD 和 runbook/loadtest 报告。
9. 批量提交和推送 GitHub，不为低风险小改动频繁推送。
```

## 1. 当前目标

持续推进 `E:\development\IM` 的 NexusIM 项目落地。

当前系统已完成本地多进程 + Win/Mac 双机 Docker 的最小分布式 IM 后端证据。主线不再停留在重型基础设施矩阵，优先推进第三层 IM 产品能力和必要可靠性补强。

当前具体下一步以 `docs/runbook/current-brief.md` 的“当前优先级”为准。

## 2. 四层路线图

### 第一层：最小可运行 IM 主链路

目标：证明 IM 后端主链路真实可跑。

范围：
- 发消息。
- 会话上下文。
- PostgreSQL 本地事务。
- outbox。
- Kafka timeline。
- durable inbox。
- PullInbox。
- AckDelivery。
- WebSocket online notify。

状态：已完成最小真实链路。

### 第二层：分布式与可靠性

目标：证明不是单机 demo，而是可解释的最小分布式后端。

范围：
- outbox relay。
- Kafka 事件流。
- delivery read model。
- Redis route。
- 多实例 push-gateway。
  补充：`loadtest/pushgateway` 和 `run-local-smoke.ps1` 调 conversation / message / delivery / identity gRPC 以及 push-gateway WebSocket 的 client 已支持可选 CA、server name 和 client cert/key；runner 也支持 `--verified-auth-metadata` / `-VerifiedAuthMetadata`，可对 conversation / message / delivery user-facing RPC 发送 gateway verified identity metadata。默认仍是 plaintext/body auth。push-gateway 进程自身的 delivery RPC client TLS 仍使用 `NEXUSIM_DELIVERY_SERVICE_TLS_*` env，且 ACK 转发会把 WebSocket auth 派生出的身份写入 delivery-service gRPC metadata；WebSocket listener 已支持第一阶段静态 `NEXUSIM_PUSH_WS_TLS_*` WSS / mTLS 配置；证书签发 / 轮换 / 分发、动态服务身份治理和全服务 mTLS rollout 仍是后续项。
  补充：clean commit `72d8a1b` 已跑通 `full + -VerifiedAuthMetadata` 真实进程 smoke：`CreateMemberChange / SendMessage / PullInbox` 走 metadata auth，push-gateway 将 WebSocket auth 派生身份转发为 delivery-service `AckDelivery` metadata，最终 `delivery.notify -> PullInbox -> delivery.ack.ok` 成功；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260613-push-gateway-verified-metadata-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\push-gateway-verified-metadata-smoke-20260613-183530`。
  补充：clean code commit `c53ae41` 已跑通 push-gateway WebSocket WSS / mTLS 真实进程 smoke：server 端启用 `NEXUSIM_PUSH_WS_TLS_*`、require client cert、client DNS SAN allowlist=`desktop-client.nexusim.local`、client URI SAN allowlist=`spiffe://nexusim/desktop-client`，runner 端使用 CA/server name/client cert/key，并通过 `-VerifiedAuthMetadata` 完成 `delivery.notify -> PullInbox -> delivery.ack.ok`；summary 显示 `push_tls_enabled=true`、`verified_auth_metadata=true`、`push_url=wss://127.0.0.1:11598`、`delivery_outbox PUBLISHED=2/PENDING=0/DLQ=0`。conversation/message/delivery/identity gRPC 在该 run 中仍是 plaintext，route backend 为 memory；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260613-push-gateway-wss-mtls-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\push-gateway-wss-mtls-smoke-20260613-203146`。
- Win/Mac 双机 smoke。
- Redis Sentinel discovery / failover smoke。
- 基础观测和 smoke 报告。

状态：面试讲解证据已够；Kafka HA、PostgreSQL failover、Redis quorum / 网络分区仍是生产化后续项，不阻塞当前产品能力主线。

### 第三层：完整 IM 产品能力

目标：补齐真正 IM 产品会被问到的核心功能。

候选能力：
- 送达 / 已读回执。
- 会话列表 / 未读数 / 置顶 / 归档 / 静音。
- 编辑 / 撤回 / 删除。
- message-service 补充：gRPC server 已补第一阶段静态 TLS / mTLS 配置，支持 client DNS / URI SAN exact-match allowlist；作为 client 调 policy-service / conversation-service 的 TLS 配置也已存在；`loadtest/sendmessage` 压测器已支持可选 CA、server name、client cert/key 和 `--verified-auth-metadata`，`loadtest/messageedit`、`loadtest/messagerevoke`、`loadtest/messagedelete` 已支持 conversation / message / delivery 三段可选 CA、server name、client cert/key 和 `--verified-auth-metadata`。默认仍是 plaintext，当前不包含证书签发 / 轮换 / 分发、动态服务身份治理或全服务 mTLS rollout。clean commit `e43a403` 已跑通 message-service gRPC mTLS 真实进程 smoke：server 端启用 TLS、require client cert、client DNS SAN allowlist=`api-gateway.nexusim.local`，client 端使用 CA/server name/client cert/key 完成 `SendMessage -> message_log / conversation_timeline_events / message_outbox -> outbox relay -> Kafka`，最终 `PENDING=0/PUBLISHED=143/DLQ=0`；报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260613-message-mtls-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\message-mtls-smoke-20260613-195916`。
- message-service auth 补充：gRPC API 已支持 `NEXUSIM_MESSAGE_AUTH_MODE=metadata` / `verified-metadata`，在该模式下 `SendMessage / EditMessage / RevokeMessage / DeleteMessage` 的 `tenant_id / user_id / device_id / session_id` 来自 gateway verified gRPC metadata，并忽略 request body 中可伪造的身份字段；clean commit `5357dfb` 已跑通 `messageedit / messagerevoke / messagedelete -VerifiedAuthMetadata` 三条真实进程 smoke，验证 metadata auth 下分别产生 `message.edited.v1 / message.revoked.v1 / message.deleted.v1` 并投影到 `PullInbox` 后完成 `AckDelivery`，报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260613-message-mutation-verified-metadata-smoke.md`；默认仍是 body 模式以兼容历史 smoke，这不是完整 API gateway 或全服务统一身份治理。
- delivery-service 补充：gRPC server 已补第一阶段静态 TLS / mTLS 配置，支持 client DNS / URI SAN exact-match allowlist；push-gateway delivery RPC client 已支持可选 CA、server name 和 client cert/key；`loadtest/delivery` 和 `loadtest/deliveryvisibility` 已支持可选 delivery client CA、server name 和 client cert/key，并均支持 `--verified-auth-metadata`；`deliveryvisibility` 同时支持 conversation client TLS。默认仍是 plaintext/body auth，当前不包含证书签发 / 轮换 / 分发、动态服务身份治理或全服务 mTLS rollout。clean code commit `e42d768` 已跑通 delivery-service gRPC mTLS 真实进程 smoke：server 端启用 TLS、require client cert、client DNS SAN allowlist=`push-gateway.nexusim.local`，client 端使用 CA/server name/client cert/key，并通过 gateway verified metadata 完成 `PullInbox -> AckDelivery`；summary 显示 `tls_enabled=true`、`verified_auth_metadata=true`、`item_count=1`、`ack_last_received_seq=1`、`cursor_last_received_seq=1`、`delivery_outbox_pending=1`。该 smoke 不启动 delivery outbox relay，所以 ACK outbox 留在 `PENDING` 是预期；报告见 `docs/runbook/loadtest/delivery-service/loadtest-report-20260613-delivery-mtls-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\delivery-mtls-smoke-20260613-201046`。
- delivery-service auth 补充：gRPC API 已支持 `NEXUSIM_DELIVERY_AUTH_MODE=metadata` / `verified-metadata`，在该模式下 `PullInbox / AckDelivery` 的 `tenant_id / user_id / device_id / session_id` 来自 gateway verified gRPC metadata，并忽略 request body 中可伪造的身份字段；默认仍是 body 模式以兼容历史 smoke，这不是完整 API gateway 或全服务统一身份治理。
- receipt-service 补充：gRPC server 已补第一阶段静态 TLS / mTLS 配置，支持 client DNS / URI SAN exact-match allowlist；`loadtest/receipt` 和 `loadtest/demo` 调 conversation / message / delivery / receipt gRPC 的 client 已支持可选 CA、server name、client cert/key 和 `--verified-auth-metadata`。默认仍是 plaintext，当前不包含证书签发 / 轮换 / 分发、动态服务身份治理或全服务 mTLS rollout。
- receipt-service auth 补充：gRPC API 已支持 `NEXUSIM_RECEIPT_AUTH_MODE=metadata` / `verified-metadata`，在该模式下 `MarkRead / GetReceiptState / ListReceiptStates / ListConversations / ArchiveConversation / PinConversation / MuteConversation` 的 `tenant_id / user_id / device_id / session_id` 来自 gateway verified gRPC metadata，并忽略 request body 中可伪造的身份字段；默认仍是 body 模式以兼容历史 smoke，这不是完整 API gateway 或全服务统一身份治理。
  补充：clean code commit `c462e57` 已跑通 receipt-service gRPC mTLS 真实进程 smoke：receipt server 端启用 TLS、require client cert、client DNS SAN allowlist=`api-gateway.nexusim.local`、client URI SAN allowlist=`spiffe://nexusim/api-gateway`，client 端使用 CA/server name/client cert/key，并通过 gateway verified metadata 完成 `GetReceiptState -> MarkRead -> ListConversations -> Archive / Pin / Mute`；summary 显示 `receipt_tls_enabled=true`、`verified_auth_metadata=true`、`success=true`、`read_user_count=1`、`conversation_list_unread_after_read.item_count=0`、`receipt_outbox PUBLISHED=3/PENDING=0/DLQ=0`。conversation/message/delivery gRPC 在该 run 中仍是 plaintext；报告见 `docs/runbook/loadtest/receipt-service/loadtest-report-20260613-receipt-mtls-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\receipt-mtls-smoke-20260613-202446`。
  补充：clean commit `4cd165e` 已跑通 receipt-service `-VerifiedAuthMetadata` 真实进程 smoke：conversation / message / delivery / receipt 四个 user-facing gRPC server 均以 metadata auth 完成投递、回执、会话列表、未读、归档、置顶和静音链路；报告见 `docs/runbook/loadtest/receipt-service/loadtest-report-20260613-receipt-verified-metadata-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\receipt-verified-metadata-smoke-20260613-184324`。
- api-gateway 补充：第一版 gRPC 入口 skeleton 已落地，复用共享 `internal/gatewayauth` 验证 gateway token，代理 conversation / message / delivery / receipt user-facing RPC，重写 body `AuthContext` 并注入 trusted metadata；入口 gRPC 和下游 client 均已支持静态 TLS / mTLS。后端启用 verified-metadata auth 时必须配合 loopback / 内网隔离或 mTLS peer allowlist。clean commit `cff1668` 已跑通 demo 经真实 api-gateway 的 secure E2E smoke：runner 对四个 user-facing gRPC target 均指向 `127.0.0.1:11903`，使用 HMAC gateway token 和 desktop-client mTLS；api-gateway 再以 `api-gateway.nexusim.local` client cert 通过 mTLS 调下游，并注入 trusted metadata；summary `git_dirty=false/success=true/gateway_auth_mode=hmac/verified_auth_metadata=false`，报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-api-gateway-secure-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-secure-smoke-20260613-clean`。clean commit `9335bd1` 已补 first-stage `/healthz`、`/readyz`、`/debug/metrics`、低敏 gRPC JSON access log 和默认 `api-gateway` audience，并跑通 `e2e-demo-api-gateway-audience-smoke-20260613-clean`，summary `git_dirty=false/success=true/gateway_auth_mode=hmac/gateway_auth_audience=api-gateway`，报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-api-gateway-audience-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-audience-smoke-20260613-clean`；secure demo wrapper 会把 `api-gateway-debug-metrics.json` 写入结果目录；历史 `push-gateway` audience 只作为显式 env 兼容。本轮继续补第一阶段 gRPC rate limiter：默认关闭，`local` backend 为进程内 token bucket，`redis` backend 为跨实例 fixed-window counter，显式启用后按 method + token fingerprint / peer address 限流，返回 `ResourceExhausted`，并在 `/debug/metrics` 输出低敏 `rate_limit` 聚合；它不是完整 tenant quota / WAF / 风控系统。
- 群成员列表、owner transfer、群管理规则。
- 联系人 / 好友关系 / 申请列表 / 取消申请 / 拉黑 / 删除 / 备注。
- contacts-service 补充：gRPC server 已补第一阶段静态 TLS / mTLS 配置，支持 client DNS / URI SAN exact-match allowlist；`loadtest/contacts` smoke runner 已支持可选 CA、server name 和 client cert/key。默认仍是 plaintext，当前不包含证书签发 / 轮换 / 分发、动态服务身份治理或全服务 mTLS rollout。
  补充：clean commit `6ebb8ff` 已跑通 contacts-service `-VerifiedAuthMetadata` accept-flow 真实进程 smoke，验证 `SendContactRequest / ListContactRequests / RespondContactRequest / ListContacts / GetContactState` 在 metadata auth 模式下完成 outbox relay 和 Kafka 读回；报告见 `docs/runbook/loadtest/contacts-service/loadtest-report-20260613-contacts-verified-metadata-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\contacts-verified-metadata-smoke-20260613-185057`。
  补充：clean commit `0490ded` 已跑通 contacts-service gRPC mTLS 真实进程 smoke：server 端启用 TLS、require client cert、client DNS SAN allowlist=`api-gateway.nexusim.local`，client 端使用 CA/server name/client cert/key 完成 accept-flow、outbox relay 和 Kafka 读回，最终 `PENDING=0/PUBLISHED=2/DLQ=0`；报告见 `docs/runbook/loadtest/contacts-service/loadtest-report-20260613-contacts-mtls-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\contacts-mtls-smoke-20260613-193706`。
- conversation-service 补充：gRPC server 已补第一阶段静态 TLS / mTLS 配置，支持 client DNS / URI SAN exact-match allowlist；message-service conversation RPC client 可配置 CA、server name 和 client cert/key；`loadtest/memberchange` 和 `loadtest/deliveryvisibility` 已支持可选 conversation client CA、server name、client cert/key 和 `--verified-auth-metadata`。默认仍是 plaintext/body auth，本轮不是全服务 mTLS rollout，也不包含证书签发、轮换、分发或动态服务身份治理。clean commit `ab42119` 已跑通 conversation-service gRPC mTLS 真实进程 smoke：server 端启用 TLS、require client cert、client DNS SAN allowlist=`api-gateway.nexusim.local`，client 端使用 CA/server name/client cert/key 完成 owner-transfer、outbox relay、Kafka 发布和 saga DONE，最终 `PENDING=0/PUBLISHED=1/DLQ=0`；报告见 `docs/runbook/loadtest/conversation-service/loadtest-report-20260613-conversation-mtls-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\conversation-mtls-smoke-20260613-194641`。
- conversation-service auth 补充：gRPC API 已支持 `NEXUSIM_CONVERSATION_AUTH_MODE=metadata` / `verified-metadata`，在该模式下 `CreateMemberChange / GetMemberChange / TransferConversationOwner / ListConversationMembers` 的 `tenant_id / user_id / device_id / session_id` 来自 gateway verified gRPC metadata，并忽略 request body 中可伪造的身份字段；`GetSendContext` 仍保持 message-service 服务间 read path 的 request 契约。默认仍是 body 模式以兼容历史 smoke，这不是完整 API gateway 或全服务统一身份治理。
- policy-service 已完成第一版独立 gRPC 服务边界：`CheckMessageAction` 返回 allow/deny、`permission_version`、classification 和 reason，message-service 可通过 `NEXUSIM_POLICY_SERVICE_ADDR` 走 RPC adapter；未配置时仍保留 legacy `StaticPolicy` fallback。clean commit `815a04f` 已跑通 direct gRPC smoke：allow / deny 两组分别覆盖 `SEND / EDIT / REVOKE / DELETE`，校验响应 echo、permission_version、classification 和 reason；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-service-smoke.md`。clean commit `102cf97` 已跑通 `policy-service -> message-service SendMessage` allow / deny 集成 smoke：allow 写入 `message_log` 与 `conversation_timeline_events` 的 `permission_version=41/classification=POLICY_RPC_ALLOWED`，deny 返回 `PermissionDenied + MESSAGE_ERROR_CODE_PERMISSION_DENIED` 且 `message_log / conversation_timeline_events / message_outbox` 均未新增；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-message-integration-smoke.md`。clean commit `3666901` 已补 policy-service 自有 exact-match PostgreSQL 规则表 `policy_message_action_rules`，并跑通规则表版 `policy-service -> message-service SendMessage` allow / deny smoke：allow 规则写入 `permission_version=41/classification=POLICY_RPC_ALLOWED`，deny 规则拒绝且不写 `message_log / conversation_timeline_events / message_outbox`；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-message-rule-store-smoke.md`。clean commit `4017387` 已扩展并强化规则表版 mutation 集成 smoke，覆盖 `EditMessage / RevokeMessage / DeleteMessage` 的 allow / deny：allow 分别验证 `message.edited.v1 / message.revoked.v1 / message.deleted.v1` timeline/outbox、`message_change_history` 类型和 before/after status、mutation timestamp、idempotency 与 `conversation_seq 1 -> 2`，deny 返回 `PermissionDenied + MESSAGE_ERROR_CODE_PERMISSION_DENIED` 且 action 前后的 conversation_seq/timeline/outbox/change_history/idempotency 和原消息行不变；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-message-actions-rule-store-smoke.md`。clean commit `1f98e20` 已补 policy-service debug `/healthz`、`/readyz`、`/debug/metrics`、gRPC 结构化日志与决策聚合指标，并让 message-service policy RPC 透传 trace/request metadata；观测 smoke 验证 allow 场景 `grpc.total_requests=4/decisions.allowed=4`、deny 场景 `grpc.total_requests=4/decisions.denied=4`，报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-service-observability-smoke.md`。clean commit `b91f410` 已补 policy-service contacts event projection consumer，`im.contact.events -> policy_contact_edges_projection -> policy_kafka_checkpoints` smoke 验证 accepted 写双向 ACTIVE、blocked 写 owner 边 BLOCKED、unblocked 恢复 ACTIVE；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-contact-projection-smoke.md`。clean commit `f044069` 已打通 direct peer context：conversation-service 为 `DIRECT` 会话派生 `direct_peer_user_id`，message-service 转发给 policy-service，policy-service 在 exact rule / static fallback 前用 contacts 投影执行 direct `SEND` hard-deny；smoke 验证 blocked 时 `allowed=false/classification=CONTACT_BLOCKED/reason=contact blocked/permission_version=2`，unblocked 后恢复 allow；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-contact-block-decision-smoke.md`。clean commit `53d3758` 已补第一阶段 policy decision audit outbox：`CheckMessageAction` 最终决策返回前写 `policy_decision_audit_outbox` PENDING 行，审计写失败 fail-closed 为 `policy unavailable`，smoke 验证三次 direct SEND 决策均写入审计 outbox 且 payload 只含 stable key、context-present flags、action、allow/deny、permission_version、classification、reason_code、trace/request id；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-decision-audit-outbox-smoke.md`。clean commit `10bc901` 已补 `policy_decision_audit_outbox -> im.policy.events` 最小 Kafka relay，smoke 验证 3 条决策 audit outbox 全部 `PUBLISHED`、`PENDING=0/DLQ=0`，并从 `im.policy.events.*` 读回 3 条 `PolicyEvent`；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-decision-audit-relay-smoke.md`。clean commit `b5dec20` 已补 policy decision audit outbox 显式 DLQ event-id repair，`outbox-repair` 只把指定 DLQ 行重置为 `PENDING`、清理 retry state、写 `policy_decision_audit_outbox_repair_audit`，后续仍由 outbox relay 发布；clean smoke 验证 synthetic DLQ event `repaired=1/skipped=0/repair_audit_count=1` 且 Kafka end offset 达到 `4`；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-decision-audit-repair-smoke.md`。clean commit `f175d68` 已给 repair 增加 relay-equivalent preflight validation：valid DLQ row 才能 redrive，invalid envelope/payload 保持 `DLQ`、写 `SKIPPED/validation_failed` repair audit，且 operator 非零退出；validated smoke 仍验证 valid repair `repaired=1/skipped=0` 且 Kafka end offset 达到 `4`，真实 PG 测试覆盖 invalid row 不清 retry、不解除 blocker；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-decision-audit-repair-validated-smoke.md`。clean commit `13fcc46` 已补 tenant action default rule store `policy_tenant_message_action_rules`，决策优先级为 direct contact block -> exact user/conversation rule -> tenant action rule -> static fallback；clean smoke 覆盖 `SEND / EDIT / REVOKE / DELETE` 的 tenant-only allow/deny，mutation 场景先用 tenant `SEND / POLICY_SEND_SEED` 创建基线消息，再验证目标 action allow/deny 不误写状态；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-message-tenant-rule-smoke.md`。clean commit `e86de33` 已补 policy-service conversation role gate Kafka smoke：`conversation.timeline.events -> policy_conversation_members_projection -> CheckMessageAction`，验证 `ADMIN/ACTIVE/v7` 通过 role gate 并进入 tenant allow、`MEMBER/ACTIVE/v8` 被 `CONVERSATION_ROLE_DENIED`、旧 `conversation_permission_version=7` fail-closed 为 `Unavailable/policy unavailable`、`MEMBER/LEFT/v9` 被 deny；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-conversation-role-smoke.md`。当前 policy-service 支持 env static、exact PG rule store、tenant action default rules、conversation role gate 真实 Kafka smoke、低敏 debug observability、contacts 投影、conversation member projection、direct SEND block 决策、审计 outbox Kafka relay、显式 DLQ repair 和 repair preflight validation；conversation role gate 通过 `conversation_permission_version` fence 防止 stale projection 决策且真实 smoke 已验证 stale projection fail-closed；clean commit `773ad4c` 已补 `message-service -> policy-service` 的 `SendMessage` conversation role gate 集成 smoke：`ADMIN/ACTIVE/v41` 允许并写入 `message_log / conversation_timeline_events / message_outbox`，message/timeline 携带 `permission_version=41/classification=POLICY_ROLE_GATE_TENANT_ALLOW`，policy audit outbox 写 `allowed=true`；`MEMBER/ACTIVE/v42` 返回 `PermissionDenied + MESSAGE_ERROR_CODE_PERMISSION_DENIED`，`message_log / conversation_timeline_events / message_outbox / conversation_seq` 均不变，policy audit outbox 写 `allowed=false/classification=CONVERSATION_ROLE_DENIED/reason_code=CONVERSATION_ROLE_DENIED`；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-message-role-gate-smoke.md`。clean commit `e638913` 已补 sender-only ownership 真实进程 smoke：`EditMessage / RevokeMessage / DeleteMessage` 同 sender 允许并写对应 mutation timeline/outbox/change_history，非 sender 返回 `PermissionDenied + MESSAGE_ERROR_CODE_PERMISSION_DENIED` 且 target action 前后的 `conversation_seq / message_change_history / timeline / outbox` 不变；policy audit 最新 target action 行分别验证 `allowed=true/classification=POLICY_OWNERSHIP_FALLTHROUGH_ALLOWED` 或 `allowed=false/classification=MESSAGE_OWNERSHIP_DENIED/reason_code=MESSAGE_OWNERSHIP_DENIED`，并且 `message_id_present=true/message_key_present=true`；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-message-ownership-smoke.md`。这仍不是完整策略引擎，完整 ReBAC、独立 `MODERATOR` 角色 / product-grade moderation policy、tenant DSL / quota / risk policy、audit retention / external sink / broad repair workflow / 更完整 poison-payload 分类仍是后续项。
- policy-service 补充：本轮新增 first-stage message ownership override，`policy_message_ownership_override_rules` + `policy_conversation_members_projection` 允许非发送者 `ADMIN/ACTIVE` 在 fresh `conversation_permission_version` 下执行 `EDIT / REVOKE / DELETE`，policy response 使用 typed `ownership_override=true`，message-service repository 只认 typed flag、不解析 classification 字符串；`MEMBER/ACTIVE` 仍返回 `MESSAGE_OWNERSHIP_DENIED` 且 mutation DB facts 不变。真实进程 smoke 见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-message-ownership-override-smoke.md`，raw result 在 `H:\NexusIM\loadtest-results\policy-message-ownership-override-smoke-20260613-r2`。这不是完整 ReBAC，也没有新增独立 `MODERATOR` 角色。
- policy-service 补充：决策指标已从 evaluator wrapper 移到 `CheckMessageAction` use-case 出口，避免 normal decisions double count，并让 `MESSAGE_OWNERSHIP_DENIED` 和 `ownership_override=true` 这类 app 层 ownership gate 决策进入 `/debug/metrics.decisions` 聚合计数。
- policy-service 补充：`/debug/metrics.policy_rule_store` 已从 exact-only 规则计数扩展为低敏规则库存快照，覆盖 exact message action rules、tenant action defaults、conversation role gate rules 和 ownership override rules；只按 action / allow-deny / min_role 聚合，不暴露 tenant/user/conversation/message 维度或规则参数。
- policy-service 补充：`/debug/metrics.policy_projection` 已补 policy-owned contacts projection、conversation member projection 和 Kafka checkpoint topic 聚合；只输出 status / role / topic 级低基数字段，不列 tenant/user/conversation/partition。
- policy-service 补充：gRPC server 已补第一阶段静态 TLS / mTLS 配置，支持 client DNS / URI SAN exact-match allowlist；message-service policy RPC client 可配置 CA、server name 和 client cert/key；`loadtest/policy`、`loadtest/policycontacts` 和 `loadtest/policyroles` direct policy smoke client 也已支持可选 CA、server name 和 client cert/key；`loadtest/policyintegration` 调 message-service 的集成 smoke client 也已支持可选 CA、server name、client cert/key 和 `--verified-auth-metadata`，脚本可用 `-VerifiedAuthMetadata` 同步启动 `NEXUSIM_MESSAGE_AUTH_MODE=metadata`。clean commit `cf91cb0` 已跑通 policy-service direct gRPC mTLS smoke：server 端启用 TLS、require client cert、client DNS SAN allowlist=`message-service.nexusim.local`，client 端使用 CA/server name/client cert/key 完成 allow/deny 两组 `SEND / EDIT / REVOKE / DELETE`，并读取 `/debug/metrics` 验证 allow `decisions.allowed=4`、deny `decisions.denied=4`；报告见 `docs/runbook/loadtest/policy-service/loadtest-report-20260613-policy-service-mtls-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\policy-service-mtls-smoke-20260613-193045`。默认仍是 plaintext/body auth，本轮不是全服务 mTLS rollout，也不包含证书签发、轮换、分发或动态服务身份治理。
- 真实鉴权、设备绑定、session revoke：当前已完成 gateway token 本地验签、HS256/RS256/JWKS、issuer allowlist、RegisterUser / Login / RefreshGatewayToken、refresh token rotation / reuse detection、device/session revoke event、push-gateway deny-list projection 和 active close；已跑通 Login/RegisterUser 到 push-gateway 的 clean smoke 以及 device/session revoke active close smoke。identity-service hardening 已覆盖：密码 Login 失败计数 / 短锁定、Login 缺失/非 ACTIVE credential 的 dummy verifier timing hardening、email/phone verification、password reset challenge hash、challenge active cap / target-level request window throttle、password reset HMAC target limiter / cleanup operator / debug metrics、challenge webhook sender 第一版及 delivery failure 后过期补偿、challenge delivery debug metrics / failure class 聚合 / 持久状态审计 / encrypted outbox + worker retry-DLQ / outbox 聚合状态指标 / operator repair audit、challenge / delivery outbox / repair audit 低敏 failure class 持久化、MFA TOTP secret / challenge delivery token 本地版本化 keyring、gRPC 结构化日志 trace/request id 透传，并已跑通 `RequestVerificationChallenge(outbox) -> challenge-delivery-worker -> webhook token -> ConfirmVerificationChallenge` 真实进程 smoke；repair mode 支持 `audit`、`redrive-active-pending`、`cancel-inactive`，但不会复活 DLQ 后已过期的 challenge token，DLQ 用户必须通过正常 API 重新申请 challenge；密码重置后撤销 ACTIVE session/refresh token、TOTP MFA 生命周期、Login 强制 MFA、TOTP factor 失败计数 / 短锁定、`MFA_UNAVAILABLE`、MFA recovery codes 生成 / 再生成 / 吊销、recovery-code 登录 user-level 失败计数 / 短锁定、Refresh-token step-up、Refresh 期间直接提交 TOTP / recovery MFA proof、session MFA proof 组合约束、`/debug/metrics` 风险计数和 JWKS public-only / RS256 static key ring / one-shot rotate operator 边界：HS256 对称密钥不再通过 identity JWKS 暴露，RS256 key ring 只允许一个当前私钥签名和旧公钥 overlap，RSA 私钥至少 2048 bit，旧公钥 JWK 禁止 `k/d/p/q/dp/dq/qi/oth` 私钥材料；`gateway-token-keyring-rotate` 只更新本地 secret-bearing keyring 文件，不做 KMS/HSM 或跨主机分发。KMS/HSM 自动密钥管理、多 issuer 治理、真实 email/SMS provider 模板 / bounce、完整 account-enumeration/timing 防护、WebAuthn、OIDC federation、IP / device / tenant 自适应风控仍是后续项。
  补充：identity-service gRPC server 端可选 TLS / mTLS 配置已落地，并支持第一版 exact-match client DNS / URI SAN allowlist；`loadtest/identity` smoke runner 已支持服务端 `IdentityGrpcTls*` 参数和客户端 CA、server name、client cert/key。clean commit `2f0a4ab` 已跑通 identity-service gRPC mTLS 真实进程 smoke：server 端启用 TLS、require client cert、client DNS SAN allowlist=`push-gateway.nexusim.local`，client 端使用 CA/server name/client cert/key 完成 `RegisterUser -> RequestVerificationChallenge(outbox) -> challenge-delivery-worker -> webhook token -> ConfirmVerificationChallenge`，报告见 `docs/runbook/loadtest/identity-service/loadtest-report-20260613-identity-mtls-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\identity-mtls-smoke-20260613-192500`。全服务 mTLS rollout、证书签发 / 轮换 / 分发和动态服务身份治理仍是后续项。
- 客户端 UI 或最小端到端演示。
  补充：clean commit `36535bd` 已跑通 demo `-VerifiedAuthMetadata` 真实进程 smoke，验证 receiver JOIN、SendMessage、`delivery.notify`、`PullInbox`、WebSocket ACK、`MarkRead` 和 `ListConversations` unread `1 -> 0` 全链路；报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-verified-metadata-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-verified-metadata-smoke-20260613-190232`。
  补充：`loadtest/demo` 已支持 push-gateway WebSocket WSS / mTLS client 参数：`--push-tls-ca-file`、`--push-tls-server-name`、`--push-tls-client-cert-file`、`--push-tls-client-key-file`，PowerShell wrapper 对应 `-PushTls*`。clean commit `5eb52d0` 已新增 `run-local-secure-demo.ps1` 并跑通 secure E2E demo smoke：脚本生成短期本地 CA/证书，启动 conversation/message/delivery/receipt/push-gateway 真实进程，覆盖 conversation / message / delivery / receipt gRPC mTLS、message-service -> conversation-service mTLS、push WSS/mTLS、push-gateway -> delivery-service mTLS 和 verified metadata；summary 显示 `conversation_tls_enabled=true/message_tls_enabled=true/delivery_tls_enabled=true/receipt_tls_enabled=true/push_tls_enabled=true/verified_auth_metadata=true/success=true`，`message_outbox / delivery_outbox / receipt_outbox` 均为 `PUBLISHED=2`。报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-secure-mtls-wss-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-secure-mtls-wss-20260613-final-r2`。这仍是本地 smoke，不是生产证书签发、轮换、分发或动态服务身份治理。
  补充：clean commit `e721e00` 已把 secure E2E demo 的 message-service policy 依赖从本地 mock 切到真实 policy-service gRPC/mTLS，并启动 policy decision audit outbox relay；正式 smoke `e2e-demo-secure-policy-mtls-wss-20260613-final` 通过，summary `git_dirty=false/success=true`，`policy_decision_audit_outbox PUBLISHED=1`、`allowed=true/permission_version=2/classification=POLICY_DEMO_ALLOWED`，报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-secure-policy-mtls-wss-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-secure-policy-mtls-wss-20260613-final`。这仍是本地 smoke，不是完整策略治理或生产证书生命周期。
  补充：clean commit `6b525e9` 已给 secure E2E demo 增加 policy audit Kafka typed read-back；正式 smoke `e2e-demo-secure-policy-readback-20260613-final` 通过，summary `git_dirty=false/success=true`，`policy_decision_audit_outbox PUBLISHED=1`，并从 per-run topic `im.policy.events.demo.secure.20260613-212756` 读回 1 条 `policy.message_action_decision.v1`，`producer=policy-service/allowed=true/permission_version=2/classification=POLICY_DEMO_ALLOWED`；原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-secure-policy-readback-20260613-final`。
  补充：clean commit `cff1668` 已跑通 demo 经真实 api-gateway 的 secure E2E smoke：`run-local-secure-demo.ps1` 启动 api-gateway gRPC `127.0.0.1:11903`，runner 对 conversation / message / delivery / receipt 四个 gRPC target 均指向 api-gateway，使用 HMAC gateway token 和 desktop-client mTLS；api-gateway 再以 `api-gateway.nexusim.local` client cert 通过 mTLS 调下游，并向下游注入 trusted metadata。summary `git_dirty=false/success=true/gateway_auth_mode=hmac/verified_auth_metadata=false`，四段 gRPC TLS 和 push WSS 均为 true，`PullInbox max_seq=2`、`delivery.ack.ok=2`、`MarkRead=2`、`ListConversations unread 1 -> 0`，`message_outbox/delivery_outbox/receipt_outbox` 均 `PUBLISHED=2`，`policy_decision_audit_outbox PUBLISHED=1` 并读回 `policy.message_action_decision.v1`；报告见 `docs/runbook/loadtest/demo/loadtest-report-20260613-e2e-demo-api-gateway-secure-smoke.md`，原始结果在 `H:\NexusIM\loadtest-results\e2e-demo-api-gateway-secure-smoke-20260613-clean`。

推进原则：优先选择能复用已有事实流、read model 和 outbox 的小闭环；不要为了产品功能让服务间耦合变高。

### 第四层：智能化扩展

目标：在核心 IM 事实、权限和消息变更语义稳定后，做智能化能力。

候选能力：
- 聊天记录搜索。
- RAG 问答。
- 智能总结。
- 群聊问答 Agent。
- 客服机器人。
- 推荐 / 风控辅助。

推进原则：第四层必须遵守成员可见窗口、撤回 / 编辑 / 删除语义、ACL 过滤、审计和失败降级。不得绕过 IM 事实源。

## 3. 硬边界

- 项目统一命名为 `NexusIM`。
- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- 根目录 `api/` 只放全局接口契约；`services/<service>/internal/api/` 才是服务内部接口适配实现。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
- push-gateway 只做在线唤醒和 ACK 转发；可靠事实在 `delivery-service user_inbox`。
- 不跨服务读取内部表，不引入网状依赖，不为短期功能增加不必要同步 RPC、公共包或抽象层。
- 单个切片如果需要同时改多个服务、多条 Kafka 事件、多张核心表或多种用户语义，必须拆小。
- 公共包、共享 helper、跨服务接口和统一框架必须有两个以上真实调用方或明确降低复杂度，否则保持在单服务内。
- 服务间同步调用只用于查询当前请求必须依赖的权限 / 上下文；状态传播优先走 Kafka 事实事件和本服务 projection。
- 压测原始数据放到 `H:\NexusIM\loadtest-results`；E 盘仓库只放报告和文档。
- Win/Mac 服务间通信优先使用有线 `172.31.50.*`，不要把服务间流量走外网或代理。
- 除非用户明确要求，不再把流量诊断、代理用量归因、外网消耗排查列为当前任务。
- 不回滚用户已有修改。

## 4. 按需读取索引

### 当前入口

- `docs/runbook/current-brief.md`：每轮默认唯一必读入口。

### 长历史

- `docs/runbook/history/current-goal-archive-20260611.md`：旧版完整历史、长事实表、逐日流水。只在需要追溯历史证据时按关键词查询。

### 架构 / 设计

- `docs/README.md`：文档入口。
- `docs/architecture/target-architecture.md`：目标架构。
- `docs/architecture/tadd.md`：技术架构决策。
- `docs/sdd/README.md`：服务设计索引。
- `docs/sdd/message-service.md`
- `docs/sdd/conversation-service.md`
- `docs/sdd/delivery-service.md`
- `docs/sdd/push-gateway.md`
- `docs/sdd/receipt-service.md`
- `docs/sdd/contacts-service.md`

### 压测 / smoke 报告

- `docs/runbook/loadtest/message-service/README.md`
- `docs/runbook/loadtest/conversation-service/README.md`
- `docs/runbook/loadtest/delivery-service/README.md`
- `docs/runbook/loadtest/push-gateway/README.md`
- `docs/runbook/loadtest/contacts-service/README.md`

### 本地 / 分布式运行

- `docs/runbook/distributed-local.md`
- `tools/local-distributed-smoke.ps1`
- `tools/sync-mac-distributed-smoke.ps1`
- `tools/check-mac-docker-desktop.ps1`

## 5. 读取规则

每轮默认：

```powershell
git status --short --branch
Get-Content docs\runbook\current-brief.md -Raw
```

需要细节时：

```powershell
Select-String -Path docs\runbook\current-goal.md -Pattern "关键词" -Context 2,4
Select-String -Path docs\runbook\history\current-goal-archive-20260611.md -Pattern "关键词" -Context 2,4
Select-String -Path docs\sdd\<service>.md -Pattern "关键词" -Context 2,4
Select-String -Path docs\runbook\loadtest\<service>\README.md -Pattern "关键词" -Context 2,4
```

实现细节优先：

```powershell
rg -n "SymbolOrKeyword" services api schemas migrations loadtest tools
```

不要为了“了解项目”全文读取 `current-goal.md`、历史归档、SDD 或压测报告。只有在做完整审计、文档重构或用户明确要求时，才允许全文读取长文档。

## 6. 评审与提交

- 公共契约、migration、事务、幂等、消息顺序、错误码、可运行链路完成时，邀请独立评审或 sub-agent 复核。
- sub-agent 任务结束后及时关闭。
- 有意义的切片完成后运行必要检查。
- 每轮结束更新 `docs/runbook/current-brief.md`。
- 阶段状态、风险或历史证据变化时，同步更新本文、对应 SDD 和 runbook/loadtest 报告。
- 批量提交和推送 GitHub，不为低风险小改动频繁推送。
