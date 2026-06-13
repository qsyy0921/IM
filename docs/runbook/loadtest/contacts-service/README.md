# contacts-service Loadtest Reports

本目录保存 `contacts-service` 的小规模验证报告和阶段总入口。

## 当前阶段结论

`contacts-service` 已完成联系人 / 好友关系第一条真实小闭环，并补齐好友申请列表、接受 / 拒绝 / 取消 / 删除 / 拉黑 / 解除拉黑 / 备注名路径：

```text
SendContactRequest
-> contact_requests(PENDING)
-> ListContactRequests(INCOMING, PENDING)
-> contacts_outbox(contact.request.created.v1)
-> RespondContactRequest(ACCEPT)
-> ListContactRequests(INCOMING, ACCEPTED)
-> contact_edges 双向 ACTIVE
-> contacts_outbox(contact.request.accepted.v1)
-> CancelContactRequest(PENDING -> CANCELED)
-> contacts_outbox(contact.request.canceled.v1)
-> contacts-service outbox-relay
-> Kafka im.contact.events
-> ListContacts / GetContactState
```

本阶段重点不是容量，而是证明联系人关系作为独立事实源成立，并且保持低耦合：

- 不写 `conversation_members`。
- 不自动创建会话。
- 不让 `message-service` 同步依赖 `contacts-service`。
- Kafka 事件仍通过 outbox relay 发布，业务事务不直接 publish Kafka。
- gRPC server 和 `loadtest/contacts` smoke runner 都支持第一阶段可选 TLS / mTLS 静态配置；默认仍是 plaintext，不代表证书签发、轮换、分发或全服务 mTLS rollout 已完成。

可选 TLS / mTLS smoke 参数示例：

```powershell
.\loadtest\contacts\run-local-smoke.ps1 `
  -ContactsGrpcTlsCertFile .\certs\contacts-server.crt `
  -ContactsGrpcTlsKeyFile .\certs\contacts-server.key `
  -ContactsGrpcTlsClientCaFile .\certs\ca.pem `
  -ContactsGrpcTlsRequireClientCert true `
  -ContactsGrpcTlsClientAllowedDnsNames api-gateway.nexusim.local `
  -ContactsTlsCaFile .\certs\ca.pem `
  -ContactsTlsServerName contacts-service.nexusim.local `
  -ContactsTlsClientCertFile .\certs\loadtest-client.crt `
  -ContactsTlsClientKeyFile .\certs\loadtest-client.key
```

服务端 TLS 可通过 `NEXUSIM_CONTACTS_GRPC_TLS_*` 环境变量配置，也可在本地 smoke 中用上面的 `ContactsGrpcTls*` 参数注入到脚本启动的 contacts-service 进程；`ContactsTls*` 参数只控制 loadtest client 如何连接 contacts-service。

gateway verified metadata auth 示例：

```powershell
.\loadtest\contacts\run-local-smoke.ps1 -VerifiedAuthMetadata
```

该模式会启动 `NEXUSIM_CONTACTS_AUTH_MODE=metadata`，并由 runner 通过 gRPC metadata 传递 `tenant_id / user_id / device_id / session_id / trace_id / request_id`；request body auth 字段仍保留用于兼容默认 body 模式。

2026-06-13 补充：`-VerifiedAuthMetadata` 真实进程 smoke 已通过，验证 `SendContactRequest / ListContactRequests / RespondContactRequest / ListContacts / GetContactState` 在 metadata auth 模式下完成 accept-flow、outbox relay 和 Kafka 读回。

api-gateway facade smoke 示例：

```powershell
.\loadtest\contacts\run-local-smoke.ps1 -GatewayFacade
```

该模式会启动 api-gateway，runner 通过 `nexusim.gateway.v1.GatewayService` 和 HMAC gateway token 调 contacts user-facing RPC；api-gateway 覆盖 request body `AuthContext`，再向 contacts-service metadata auth 注入 trusted identity。2026-06-14 clean smoke 已通过，summary `git_dirty=false/success=true/gateway_facade=true/gateway_auth_mode=hmac`，contacts outbox `PUBLISHED=2/PENDING=0/DLQ=0`。

## 报告列表

| 报告 | 内容 |
| --- | --- |
| `loadtest-report-20260610-contacts-smoke.md` | `SendContactRequest -> RespondContactRequest(ACCEPT) -> contacts_outbox -> im.contact.events -> ListContacts / GetContactState` 真实进程 smoke |
| `loadtest-report-20260610-contacts-decline-smoke.md` | `SendContactRequest -> RespondContactRequest(DECLINE) -> contacts_outbox -> im.contact.events` 真实进程 smoke，验证不创建联系人边 |
| `loadtest-report-20260611-contacts-request-list-smoke.md` | `SendContactRequest -> ListContactRequests(PENDING) -> RespondContactRequest(ACCEPT) -> ListContactRequests(ACCEPTED) -> ListContacts` 真实进程 smoke |
| `loadtest-report-20260611-contacts-cancel-smoke.md` | `SendContactRequest -> ListContactRequests(INCOMING,PENDING) -> CancelContactRequest -> ListContactRequests(OUTGOING,CANCELED) -> contacts_outbox -> im.contact.events` 真实进程 smoke，验证 sender 取消 pending 申请且不创建联系人边 |
| `loadtest-report-20260611-contacts-edge-management-smoke.md` | `DeleteContact` / `BlockContact` / `UpdateContactRemark` 三条真实进程 smoke，验证 owner 视角联系人边管理、outbox 和 Kafka 读回 |
| `loadtest-report-20260611-contacts-unblock-smoke.md` | `BlockContact -> UnblockContact` 真实进程 smoke，验证 owner 视角 `BLOCKED -> ACTIVE`、outbox 和 Kafka 读回 |
| `loadtest-report-20260611-contacts-readd-smoke.md` | `ACCEPT -> DeleteContact -> SendContactRequest -> ACCEPT` 真实进程 smoke，验证删除后重新申请恢复和 contacts outbox 版本单调 |
| `loadtest-report-20260613-contacts-verified-metadata-smoke.md` | `-VerifiedAuthMetadata` 真实进程 smoke，验证 metadata auth 下的 contacts accept-flow、outbox relay 和 Kafka 读回 |
| `loadtest-report-20260613-contacts-mtls-smoke.md` | contacts-service gRPC TLS / mTLS + client DNS SAN allowlist 下的 accept-flow、outbox relay 和 Kafka 读回 |
| `loadtest-report-20260614-contacts-api-gateway-facade-smoke.md` | api-gateway `GatewayService` facade + HMAC token 下的 contacts accept-flow、outbox relay 和 Kafka 读回 |

## 面试可讲重点

- `contacts-service` 是第三层 IM 产品能力，专门管理好友申请和联系人关系。
- 好友关系和会话成员关系解耦：接受好友不会自动把用户写入某个会话，也不会直接创建 direct conversation。
- 好友申请、接受、拒绝和取消在 PostgreSQL 本地事务内写事实表与 outbox，Kafka 发布由 relay 异步完成，保持 at-least-once。
- 所有联系人事件按 canonical user pair 做 partition key，保证同一对用户的 created / accepted / declined / deleted / blocked / unblocked / remark_updated 顺序不会被打乱。
- `ListContacts` / `GetContactState` 从 contacts-service 自己的 read model 读取，不跨服务读其它内部表。
- `ListContactRequests` 从 `contact_requests` 读取当前用户收到 / 发出的申请，cursor 绑定 tenant、user、direction、status 和 page size，避免跨条件串页。
- 当前 smoke 已验证 ACCEPT 后双向 ACTIVE edge、DECLINE / CANCEL 后不创建 edge、Delete/Block/Unblock/Remark 只修改当前 owner 视角 edge、删除后重新申请可以恢复联系人关系、outbox 清空、Kafka 读回对应 contact event。
- contacts-service 还补了只读 `outbox-repair-audit` 和 `outbox-repair-cleanup` 运维模式，方便直接审计并按 retention 清理 DLQ repair 历史；它们不直接 publish Kafka，也不会改写当前 outbox 事实状态。
- api-gateway facade smoke 已验证 contacts user-facing RPC 可以收敛到统一入口，客户端不需要直连 contacts-service；contacts-service 仍是事实源，api-gateway 只做鉴权、身份覆盖和转发。
- gRPC TLS / mTLS 可以作为“服务端和 smoke 客户端的第一阶段传输安全已接通”来讲，但必须说明还没有做证书生命周期治理、动态服务身份或服务网格。
- 后续如果要“接受好友后自动创建单聊”，应通过显式 saga / app port 编排，而不是在 contacts-service 事务里写 conversation-service 表。
