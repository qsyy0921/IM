# NexusIM Current Brief

## 每轮只读这个

1. 先运行 `git status --short --branch`。
2. 本文件是默认入口，保持短。
3. 需要找文档先看 `docs/runbook/README.md`，再只读相关短索引或服务文件。
4. 需要服务状态先看 `docs/runbook/service-briefs/README.md` 索引，再只读相关服务文件。
5. 需要历史证据再按关键词查 `docs/runbook/archive/` 或 `docs/runbook/loadtest/<service>/`。
6. 不要为了“了解项目”全文读取长文档。

## 当前状态

NexusIM 已有本地/双机可运行的最小分布式 IM 后端：

- message-service、conversation-service、delivery-service、push-gateway、receipt-service、contacts-service、identity-service、policy-service、api-gateway 均已有真实链路或最小闭环。
- Win/Mac Docker 分布式 smoke 已证明跨实例 route / resume / PullInbox fallback 等关键路径。
- 当前不是生产级 HA：Kafka HA、PostgreSQL failover、Redis quorum / 网络分区、服务发现、统一观测和部署编排仍是后续。
- 当前 9 个服务足够支撑 IM 主链路；后续服务和中间件不写死，新增或替换必须满足拆分 / 演进准则并通过 ADR。

## 当前优先级

1. 当前已完成：identity-service 已补只读 `session-mfa-proof-audit` 运维模式，用于发现历史 session MFA proof 脏数据。
2. 当前已完成：服务状态已拆成 `docs/runbook/service-briefs/<service>.md`，并由 `.\tools\check-local.ps1` 防止入口和单服务 brief 重新变长。
3. 当前已完成：`docs/README.md` 和 `docs/sdd/README.md` 也纳入短入口检查；旧长 SDD 索引已归档。
4. 当前先治理已有 9 个微服务；identity repository、repository test、`loadtest/pushgateway/main.go` 和长目标架构文档已完成拆分，delivery-service 已补 `/debug/metrics`、`outbox-repair` 和 `projection-checkpoint-repair` 基础运维入口，后续继续处理各服务 P2 hardening。

## 已知硬约束

- 压测原始数据：`H:\NexusIM\loadtest-results`
- 仓库文档：`E:\development\IM\docs`
- Win/Mac 有线通信优先 `172.31.50.*`
- 不做外网流量诊断，除非用户明确要求。
- 不回滚用户已有修改。
- 代码规模治理：生产手写文件接近 2500 行、测试/runner 接近 3000 行时优先同 package 拆分，避免继续堆大文件。

## 每轮结束

1. 更新本文件的“当前优先级”。
2. 若服务状态变化，更新 `docs/runbook/service-briefs/<service>.md`。
3. 需要历史归档时只追加或拆分，不把长历史重新塞回入口文档。
4. 运行 `.\tools\check-local.ps1`，保证下一轮入口仍然短，并捕获基础 whitespace / PowerShell 语法问题。
5. 按切片风险追加必要测试，提交并推送有意义的切片。
