# NexusIM Repair Operators

本文件只做现有本地 operator 的统一入口。它不替代服务 SDD、smoke 报告或审批系统，也不代表已经有生产级运维 UI。

## 使用原则

- 先 audit，后 repair；没有明确 event / outbox / checkpoint / failure 范围时不要 redrive。
- 优先使用服务自带 operator，不直接手写 SQL 修改业务表。
- 所有 redrive / cleanup 必须保留服务内 audit 记录。
- `last_error` / `previous_last_error` / provider error 只能保留稳定低敏分类，不能写入 broker body、token、账号、目标地址或原始 provider body。
- operator 只处理本服务拥有的表；不能跨服务读写私有表。

## 通用 outbox 模式

这些服务已有相同形态的 outbox 排障入口：

| 服务 | 环境变量 | 只读审计 | redrive | repair 审计 | cleanup |
| --- | --- | --- | --- | --- | --- |
| `message-service` | `NEXUSIM_MESSAGE_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |
| `delivery-service` | `NEXUSIM_DELIVERY_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |
| `receipt-service` | `NEXUSIM_RECEIPT_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |
| `contacts-service` | `NEXUSIM_CONTACTS_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |
| `policy-service` | `NEXUSIM_POLICY_SERVICE_MODE` | `outbox-audit` | `outbox-repair` | `outbox-repair-audit` | `outbox-repair-cleanup` |

示例：

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE = "outbox-audit"
go run ./services/delivery-service/cmd/delivery-service
```

```powershell
$env:NEXUSIM_DELIVERY_SERVICE_MODE = "outbox-repair"
go run ./services/delivery-service/cmd/delivery-service
```

具体过滤参数以对应服务 cmd / service brief / smoke 报告为准。

以下服务的 `outbox-repair-audit` 支持写低敏 JSON 结果，便于 operator 留存证据，不写 Kafka 原始错误正文或业务 payload：

| 服务 | JSON 输出环境变量 |
| --- | --- |
| `message-service` | `NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_OUTPUT` |
| `delivery-service` | `NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTPUT` |
| `receipt-service` | `NEXUSIM_RECEIPT_OUTBOX_REPAIR_AUDIT_OUTPUT` |
| `contacts-service` | `NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_OUTPUT` |
| `policy-service` | `NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTPUT` |

## Delivery Projection

`delivery-service` 额外拥有 projection 排障入口：

| 模式 | 作用 |
| --- | --- |
| `projection-failure-audit` | 只读列出 unresolved projection failure，支持按 offset / event / failure class 缩小范围；可选 `NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `projection-checkpoint-repair` | 带审计回调 checkpoint 做 replay；只允许回调，不允许前跳跳过事件。 |
| `projection-checkpoint-repair-audit` | 只读列出 checkpoint repair audit 历史；可选 `NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `projection-checkpoint-repair-cleanup` | 清理超过保留期的 checkpoint repair audit 历史。 |
| `projection-failure-cleanup` | 只清理 resolved 且超过保留期的 failure 审计行，不触碰 unresolved blocker。 |

## Conversation Member Change

`conversation-service` 当前提供：

环境变量：`NEXUSIM_CONVERSATION_SERVICE_MODE`

| 模式 | 作用 |
| --- | --- |
| `member-change-audit` | 只读审计 `member_change_saga`，可按 tenant / conversation / status / change_id / outbox event 缩小范围；可选 `NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_OUTPUT` 写低敏 JSON 结果。 |

成员窗口历史 repair / repair action 仍是后续工作，不能用手写 SQL 直接改成员事实。

## Identity Challenge / Session

`identity-service` 当前提供：

环境变量：`NEXUSIM_IDENTITY_SERVICE_MODE`

| 模式 | 作用 |
| --- | --- |
| `session-mfa-proof-audit` | 只读发现历史 session MFA proof 脏数据；可选 `NEXUSIM_IDENTITY_SESSION_MFA_PROOF_AUDIT_OUTPUT` 写低敏聚合 JSON 结果。 |
| `challenge-delivery-repair` | 处理 challenge delivery outbox / retry / expire / DLQ 相关修复。 |
| `challenge-delivery-repair-audit` | 只读审计 challenge delivery repair 历史；可选 `NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `challenge-delivery-repair-cleanup` | 按 retention / scope 清理 challenge delivery repair audit 历史。 |
| `challenge-request-limit-cleanup` | 清理 verification / password reset request limit 历史。 |
| `gateway-token-keyring-rotate` | 轮换本地 RS256 gateway token keyring 文件；生成新当前私钥，把旧当前 key 降级为 public-only overlap，并按 old-key limit 保留旧公钥。 |

`gateway-token-keyring-rotate` 只处理本地 secret-bearing JSON 文件。它不是 KMS / HSM、不会跨主机分发密钥，也不替代正式密钥管理审批。

## Contacts Policy Operators

`contacts-service` 额外拥有租户默认值和来源策略 operator：

| 模式 | 作用 |
| --- | --- |
| `tenant-privacy-default-audit` | 只读审计租户联系人隐私默认值；可选 `NEXUSIM_CONTACTS_TENANT_PRIVACY_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `tenant-privacy-default-set` | 设置租户联系人隐私默认值。 |
| `source-policy-audit` | 只读审计联系人来源策略；可选 `NEXUSIM_CONTACTS_SOURCE_POLICY_AUDIT_OUTPUT` 写低敏 JSON 结果。 |
| `source-policy-set` | 设置联系人来源策略。 |

这些仍是本地 operator 形态；后续 admin/config service 接入后，应迁移到正式权限面和审批流。

## 仍未完成

- 跨服务统一 repair runbook 的执行编排。
- 批量 repair 审批流和运维 UI。
- 外部 audit sink。
- 更细 poison payload 分类和长期 retention 策略。
