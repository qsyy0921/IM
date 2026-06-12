# NexusIM Current Brief

本文是每轮 Codex 工作的低 token 入口。每轮默认只读本文；需要哪类信息，就按关键词读取对应文档的相关片段，不要把 `current-goal.md`、SDD、压测报告或历史文档整篇读一遍。旧版长目标档案已归档到 `docs/runbook/history/current-goal-archive-20260611.md`，只在追溯历史证据时按关键词查询。

## 当前定位

NexusIM 已完成四个真实微服务的最小链路：

```text
conversation-service
-> message-service
-> delivery-service
-> push-gateway
```

当前系统可以表述为“本地多进程 + Win/Mac 双机 Docker 最小分布式 IM 后端”。已跑通发消息、会话上下文、PostgreSQL outbox、Kafka timeline、durable inbox、PullInbox、AckDelivery、WebSocket notify、Redis route、cross-instance resume、Win/Mac 双机 Docker smoke、Redis Sentinel discovery、手动 failover 和停止当前 master 后的自动切主 recovery smoke。Mac 侧当前 NexusIM 服务镜像和 PostgreSQL / Redis / Kafka / Schema Registry / Kafka UI 均已确认 `arm64/linux`，双机 smoke 使用有线 `172.31.50.*`。

## 当前优先级

1. 当前分布式证据已经够用于面试讲“最小分布式 IM 后端”，不要继续长期停留在重型基础设施故障矩阵。
2. 最新 Win/Mac arm64 Docker 分布式 smoke 已通过两组：`full` 场景证明 Windows delivery consumer 通过 Redis route 唤醒 Mac WebSocket gateway，并完成 `PullInbox + AckDelivery`；`cross-instance-resume` 场景证明客户端首连 Mac gateway、断开后重连 Windows gateway 时可通过 Redis-backed resume buffer replay 同一条 `delivery.notify`。Mac 侧现在已同步 7 个 NexusIM 服务的 `linux/arm64` Docker 镜像：conversation / message / delivery / push / receipt / contacts / identity，PostgreSQL / Redis / Kafka / Schema Registry / Kafka UI 也已确认 `arm64/linux`。2026-06-12 额外跑过 all-service Docker check：Mac 7 个服务镜像均能在主 runtime mode 启动并监听端口，双机 full / resume smoke 继续通过；同日已创建 22 个 `nexusim-mac-*` 容器，覆盖 5 个基础设施容器和 17 个服务角色容器，默认不自动占用资源，重建脚本在 Mac 的 `/Users/qsyy0921/Desktop/IM/_local/distributed-smoke/docker/create-mac-nexusim-containers.sh`。随后 Mac 本机容器集合也已跑通 full smoke：Mac-local PostgreSQL / Redis / Kafka + 22 个容器完成 `CreateMemberChange -> SendMessage -> delivery.notify -> PullInbox -> AckDelivery`，测试后容器全部停止。最新 Docker-only 系统 smoke 使用 Windows Docker runner 访问 Mac Docker 服务容器，覆盖 full path、message edit/revoke/delete、contacts 8 场景、receipt/list 偏好、identity HMAC push auth、seeded member JOIN 和 ws-only cross-instance resume；报告见 `docs/runbook/loadtest/distributed/loadtest-report-20260612-all-service-docker-smoke.md`。这仍是小规模 smoke，不是生产 HA 或容量结论。
3. 当前第三层产品能力已切到送达 / 已读回执：receipt-service 已完成 proto / Kafka schema / migration / 六层骨架、PostgreSQL repository、delivery event consumer、`MarkRead` 事务、`ListReceiptStates` 薄批量查询和 receipt outbox relay。批量查询保持低耦合：app 层一次鉴权后复用既有 `GetReceiptState`，不新增批量 SQL、跨服务内部表读取或公共抽象。
4. receipt-service 真实进程 smoke 已跑通：`im.delivery.events -> receipt projection -> MarkRead -> GetReceiptState -> receipt_outbox -> im.receipt.events`；不要直接读取 delivery-service 内部表。
5. 会话列表 / 未读数最小 `ListConversations` 已在 receipt-service 内落地并跑通真实进程 smoke：`unread_count` 从投递后的 `1` 在 `MarkRead` 后变为 `0`；当前已补 `source_event_type` 口径、keyset 分页契约、`unread_only` 未读过滤，以及最小 `ArchiveConversation` / `PinConversation` / `MuteConversation` 用户列表偏好。clean commit `22ed67f` 已跑通 unread filter smoke：投递后 `ListConversations(unread_only=true)` 返回 `item_count=1/unread_count=1`，`MarkRead` 后返回 `item_count=0`，cursor 绑定 `unread_only`，并补 `unread_count > 0` partial index。clean commit `f8ab746` 已跑通 archive smoke：默认列表隐藏 archived 会话、`include_archived=true` 可查回、归档期间新消息推进 `last_visible_seq=3/unread_count=1` 但不会自动取消归档、取消归档后恢复默认可见。clean commit `bad4dda` 已跑通 pin smoke：`PinConversation(true)` 后默认列表 `pinned=true`，`PinConversation(false)` 后恢复 `pinned=false`；真实 PostgreSQL 测试覆盖 pinned-first 排序、显式 updated sort 和 cursor。clean commit `cd429d9` 已跑通 mute smoke：`MuteConversation(true)` 后列表 `muted=true`，`MuteConversation(false)` 后恢复 `muted=false`，`unread_count=1` 和 `last_read_seq=2` 不变。edit/revoke/delete 可推进会话 `last_visible_seq` 和 `last_source_event_type`，但不作为新未读消息计数；archive/pin/mute 只影响当前用户列表偏好，不影响 unread、delivery、push 或消息事实。该能力复用 `im.delivery.events`、`receipt_inbox_projection` 和 `user_read_cursors`，没有新增独立服务，也不读取其它服务内部表。
6. 当前第三层消息变更能力已在 clean commit `8d008de` 跑通 `RevokeMessage` 最小真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> RevokeMessage -> message outbox relay -> delivery tombstone projection -> PullInbox(message.revoked.v1) -> AckDelivery`；并已补 hardening：第一阶段只允许原发送者撤回，delivery tombstone 只投给已在 `user_inbox` 收到原消息的用户，revoke 早于 persisted 投影时 fail-closed 不提交 checkpoint。报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-revoke-message-smoke.md`。
7. 当前第三层消息变更能力已在 clean commit `cb2f07d` 跑通 `EditMessage` 最小真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> EditMessage -> message outbox relay -> delivery edit projection -> PullInbox(message.edited.v1) -> AckDelivery`；第一阶段限定原发送者编辑自己的 TEXT 消息，采用 last-write-wins + `message_change_history` 保留 before/after payload，不引入新服务或跨服务内部表读取。报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-edit-message-smoke.md`。
8. 当前第三层消息变更能力已在 clean commit `b001eb1` 跑通 `DeleteMessage` 最小真实进程 smoke：`SendMessage -> PullInbox(message.persisted.v1) -> DeleteMessage -> message outbox relay -> delivery delete projection -> PullInbox(message.deleted.v1) -> AckDelivery`；第一阶段语义是全局 `CONVERSATION_VIEW` tombstone，不是用户私有删除或合规物理擦除。报告见 `docs/runbook/loadtest/message-service/loadtest-report-20260610-delete-message-smoke.md`。
9. push-gateway 在线 `delivery.notify` 已保持轻量通知边界并透传 `source_event_type`，让客户端能区分新增 / 编辑 / 撤回 / 删除唤醒；展示事实仍以 `PullInbox` 为准。
10. push-gateway 标准 smoke runner 已在 clean commit `81fe92c` 跑通 `message-change-notify` 三类真实进程 smoke：`edit / revoke / delete` 均证明在线 `delivery.notify.source_event_type` 与 durable `PullInbox.event_type + message_id + conversation_seq` 一致；报告见 `docs/runbook/loadtest/push-gateway/loadtest-report-20260610-push-gateway-message-change-notify-smoke.md`。
11. push-gateway / identity-service 已完成低耦合真实鉴权主线：短期 gateway token、本地验签、legacy HMAC、标准三段 JWT HS256、第一版 RS256 私钥签发 + 公钥 JWKS 本地验签 + issuer allowlist、JWKS URL 启动拉取与定期 refresh/cache、JWKS refresh/cache debug stats、identity-service 额外旧公钥 JWKS overlap、RegisterUser / Login / RefreshGatewayToken、refresh token rotation / reuse detection、device/session revoke event、push-gateway deny-list projection 和 active close。登录失败计数和短时锁定已持久化到 `identity_users.failed_login_count / failed_login_last_at / locked_until`，默认 15 分钟窗口内 5 次失败后锁定密码 Login 15 分钟，成功 Login 会清理失败状态；有效 refresh token 仍按 rotation/reuse 规则独立保护。邮箱 / 手机验证与密码重置 challenge 核心已落地：只存 challenge token hash，验证 challenge 需要当前密码，密码重置要求已验证 destination，并在确认重置后撤销该用户 ACTIVE session / refresh token、写 `identity.session.revoked.v1` outbox；`RequestPasswordReset` 对无效目标或 active challenge 限流返回中性 accepted 形态，challenge 创建已有基础 active 上限。真实 email/SMS sender、MFA、OIDC federation、自动 key rotation、KMS/HSM、多 issuer 治理、完整 account-enumeration/timing 防护、更完整风控/限流、mTLS、统一 trace 和告警仍是后续项。已跑通的关键 smoke 包括 `IssueGatewayToken -> push auth`、`Login(jwt) -> push auth`、`RegisterUser -> Login(jwt) -> push auth`、device revoke deny-list / active close、session revoke active close；详见 `docs/runbook/loadtest/push-gateway/README.md` 和 `docs/runbook/loadtest/push-gateway/loadtest-report-20260612-push-gateway-identity-token-smoke.md`。HS256 JWK 仅用于本地 / 内部调试。
12. conversation-service 群管理最小读能力已完成：`ListConversationMembers` 的 JOIN / LEAVE / REMOVE / ROLE_CHANGED roster smoke 均通过；owner transfer 也已完成专用 RPC、事件、projection 和真实进程 smoke。它是低耦合 roster API，不让其它服务跨表读取 `conversation_members`。
13. contacts-service 已完成好友申请、申请列表、接受、拒绝、取消、删除、拉黑、解除拉黑、备注名和删除后重新申请恢复的最小闭环。clean commit `eecdccb` 跑通 `ListContactRequests` smoke：`SendContactRequest -> ListContactRequests(INCOMING,PENDING)=1 -> RespondContactRequest(ACCEPT) -> ListContactRequests(INCOMING,PENDING)=0 -> ListContactRequests(INCOMING,ACCEPTED)=1 -> ListContacts`，outbox `PUBLISHED=2/PENDING=0/DLQ=0`；clean commit `f291aa5` 跑通 `CancelContactRequest` smoke：`SendContactRequest -> ListContactRequests(INCOMING,PENDING)=1 -> CancelContactRequest(CANCELED) -> ListContactRequests(INCOMING,PENDING)=0 -> ListContactRequests(OUTGOING,CANCELED)=1`，outbox `PUBLISHED=2/PENDING=0/DLQ=0`，Kafka 读回 `contact.request.created.v1 / contact.request.canceled.v1`；报告见 `docs/runbook/loadtest/contacts-service/loadtest-report-20260611-contacts-cancel-smoke.md`。contacts-service 仍保持独立事实源，不写 `conversation_members`，不自动创建会话，message-service 不同步依赖 contacts-service。生产化 hardening 已开始补齐：`NEXUSIM_CONTACTS_AUTH_MODE=metadata` 支持从 gateway verified gRPC metadata 派生身份并忽略请求体伪造身份；`NEXUSIM_CONTACTS_DEBUG_ADDR` 暴露 `/healthz`、`/readyz` 和 `/debug/metrics`，包含 PostgreSQL ready check、pgx pool、contacts outbox 状态和 contacts gRPC method/code/latency 统计；gRPC interceptor 输出 JSON 结构化请求日志；`NEXUSIM_CONTACTS_SERVICE_MODE=outbox-repair` 支持按明确 `event_id` 把 DLQ 事件受控重置为 PENDING，再交回 outbox relay 按顺序发布。
14. 本地端到端演示入口 `loadtest/demo` 已跑通真实多进程 smoke：通过公开 gRPC / WebSocket 串起 `CreateMemberChange(JOIN) -> SendMessage -> delivery.notify -> PullInbox -> delivery.ack -> MarkRead -> ListConversations`，结果写入 `H:\NexusIM\loadtest-results\e2e-demo-smoke-20260612-013408`，报告见 `docs/runbook/loadtest/demo/loadtest-report-20260612-e2e-demo-smoke.md`。本轮修复了 runner 对异步 receipt projection 的等待：先等 `ListConversations(unread=1)`，再重试 `MarkRead` 直到 ACK projection 追上，最后验证 `ListConversations(unread=0)`。
15. RAG / Agent / 智能总结属于第四层，必须等消息事实、权限边界、撤回/编辑/删除语义更稳定后再做。
16. Kafka HA、PostgreSQL failover、Redis quorum / 网络分区可作为后续生产化项，不作为当前主线阻塞。

## 硬边界

- 项目名统一为 `NexusIM`。
- 每个微服务独立六层 DDD：`api / app / domain / infrastructure / types / trigger`。
- Kafka 事件只能通过 outbox relay 发布，业务事务不能直接 publish Kafka。
- push-gateway 只做在线唤醒和 ACK 转发；可靠事实在 `delivery-service user_inbox`。
- 开发时优先降低微服务耦合、控制代码复杂度；不要为了“分布式”引入网状依赖、跨服务内部表读取、不必要的同步 RPC、公共包或过度抽象。
- 复杂度控制是硬约束：一个切片如果需要同时改多个服务、多条 Kafka 事件、多张核心表或多种用户语义，先拆小；一个 helper / port / 公共包如果没有两个以上真实调用方或不能明显降低复杂度，就留在单服务内。
- 新能力优先复用已有事实流、outbox、projection、read model 和端口；只有能减少重复、稳定边界或支撑真实链路时才新增服务、表、公共包或抽象。
- 单个切片保持小闭环：先补契约 / migration / 本地事务 / consumer 或 relay / smoke，再扩展 hardening；不要一次性横跨多个产品能力。实现方案明显变复杂时，先补 SDD / 契约并让 sub-agent 复核，再编码。
- 开发中可以主动创建 sub-agent 做设计、实现、测试、文档或风险复核；专项任务结束后及时关闭，不要长期占用线程池。
- 压测原始数据放到 `H:\NexusIM\loadtest-results`，E 盘仓库只放报告和文档。
- Win/Mac 服务间通信优先使用有线 `172.31.50.*`，不要把服务间流量走外网或代理。
- 除非用户明确要求，不再把流量诊断、代理用量归因、外网消耗排查列为当前任务；日常开发只保留“少下载、用已有镜像/依赖、服务间走本地有线”的约束。
- 不回滚用户已有修改。

## 每轮开始

```powershell
git status --short --branch
Get-Content docs\runbook\current-brief.md -Raw
```

按需读取规则：

```powershell
Select-String -Path docs\runbook\current-goal.md -Pattern "关键词" -Context 2,4
```

- 查长期目标 / 历史事实 / 风险时，只用 `Select-String` 查 `docs/runbook/current-goal.md` 的相关段落。
- 查某个服务设计时，只读对应 `docs/sdd/<service>.md` 的相关章节。
- 查压测或 smoke 证据时，只读 `docs/runbook/loadtest/<service>/README.md` 或指定报告的相关段落。
- 查当前实现时，优先 `rg` 定位入口和符号，再读取目标文件片段；不要为了“了解项目”全量扫描所有文档。
- 只有在明确需要重构文档结构或做完整审计时，才允许全文读取长文档。

## 每轮结束

- 更新 `docs/runbook/current-brief.md` 的当前优先级。
- 如果阶段状态、风险或历史证据变化，再同步更新 `docs/runbook/current-goal.md`。
- 有意义的切片完成后运行必要检查、提交；批量推送 GitHub，不为低风险小改动频繁推送。
