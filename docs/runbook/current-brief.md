# NexusIM Current Brief

## 每轮只读这个

1. 先运行 `git status --short --branch`。
2. 本文件是默认入口，保持短。
3. 需要服务状态再读 `docs/runbook/service-briefs/README.md` 的相关段落。
4. 需要历史证据再按关键词查 `docs/runbook/archive/` 或 `docs/runbook/loadtest/<service>/`。
5. 不要为了“了解项目”全文读取长文档。

## 当前状态

NexusIM 已有本地/双机可运行的最小分布式 IM 后端：

- message-service、conversation-service、delivery-service、push-gateway、receipt-service、contacts-service、identity-service、policy-service、api-gateway 均已有真实链路或最小闭环。
- Win/Mac Docker 分布式 smoke 已证明跨实例 route / resume / PullInbox fallback 等关键路径。
- 当前不是生产级 HA：Kafka HA、PostgreSQL failover、Redis quorum / 网络分区、服务发现、统一观测和部署编排仍是后续。

## 当前优先级

1. 当前切片：identity-service TOTP proof 在最终 Login / Refresh PostgreSQL 事务中重新锁定 ACTIVE factor 并检查 `login_locked_until`，避免 app 层验证后 factor 被锁仍写 session / refresh token。
2. 继续做小而完整的生产级 hardening，不一次性横跨多个服务。
3. 文档维护目标：入口短、状态拆分、按需查询，减少每轮 token 浪费。

## 已知硬约束

- 压测原始数据：`H:\NexusIM\loadtest-results`
- 仓库文档：`E:\development\IM\docs`
- Win/Mac 有线通信优先 `172.31.50.*`
- 不做外网流量诊断，除非用户明确要求。
- 不回滚用户已有修改。

## 每轮结束

1. 更新本文件的“当前优先级”。
2. 若服务状态变化，更新 `docs/runbook/service-briefs/README.md`。
3. 需要历史归档时只追加或拆分，不把长历史重新塞回入口文档。
4. 运行必要检查，提交并推送有意义的切片。
