# NexusIM Development Progress

这份文档只做“当前开发进度总览”。

- 面向人看整体进度，不作为每轮默认入口。
- 每次只在阶段状态真的变化时更新。
- 细节证据仍放在 `loadtest/`、`service-briefs/`、`sdd/` 和 `archive/`。
- 阶段顺序和为什么这么做，见 `development-process.md`。

## 当前快照

截至当前仓库状态，NexusIM 已经不是单体 demo，而是可本地 / 双机运行的最小分布式 IM 后端。

当前已落地的真实服务：

- `api-gateway`
- `identity-service`
- `message-service`
- `conversation-service`
- `delivery-service`
- `push-gateway`
- `receipt-service`
- `contacts-service`
- `policy-service`

当前未进入真实实现主线、仍属于后续能力：

- `search-service`
- `media-service`
- `notification-service`
- `audit-service`
- `admin-service`
- `rag / summary / agent` 等智能化扩展

当前面试主线只覆盖：

```text
后端微服务主链路
-> 分布式可靠性
-> 安全 / 观测 / repair / 运维 hardening
-> search / RAG / Agent 应用后端
```

Web / App / 桌面端属于后续产品化展示层，暂不纳入当前开发主线。

## 总体进度

### 1. IM 主链路

当前 9 个服务已经覆盖 IM 主链路：

- 注册 / 登录 / refresh / MFA 基础能力
- 会话和成员读写
- 发送消息
- timeline / outbox / Kafka 传播
- durable inbox / `PullInbox` / `AckDelivery`
- WebSocket 在线通知
- receipt / contacts / policy 基础链路

可以把当前系统表述为：

```text
本地 / 双机可运行的最小分布式 IM 后端
```

还不能表述为：

```text
生产级完整分布式 IM 平台
```

### 2. 分布式与可靠性

已经完成的关键分布式证据：

- 本地多进程 distributed smoke
- Win / Mac Docker cross-instance smoke
- Redis route / Redis-backed resume
- Redis stop/start fault fallback
- Redis Sentinel discovery / failover / master-stop / quorum-loss fallback
- Redis Sentinel network-partition fallback smoke
- PostgreSQL `repmgr + pgpool` local failover smoke
- PostgreSQL quorum observation smoke and ADR-034 production quorum boundary
- Kafka KRaft 3 broker local failover / controller-switch / ISR observation smoke，且 ISR observation raw summary 已有可复用 JSON / Markdown summary validator
- outbox Kafka producer first-stage `acks=all` / bounded retry-backoff 配置、本地门禁、6 个 producer package 配置单测和 producer config summary

当前已经证明：

- 在线通知层可以跨实例工作
- Redis 故障时 durable `PullInbox + AckDelivery` 可以兜底
- PostgreSQL / Kafka 单点切换后最小链路仍可恢复

当前还没有证明：

- 生产级 Redis HA / Redis Cluster
- 生产级 PostgreSQL HA / split-brain fencing / quorum write guard
- 生产级 Kafka multi-failure / sustained ISR flapping / consumer rebalance治理
- 完整部署编排、服务发现、统一观测、灰度发布

### 3. 安全与运维

当前已经落地的共性 hardening：

- 各核心服务已补 `/healthz`、`/readyz`、`/debug/metrics`
- 公网地址 + 弱鉴权 / 明文入口的启动门禁
- trusted metadata / mTLS 边界的第一阶段收口
- 项目命名门禁覆盖 Go / Markdown / PowerShell / Bash 等文本文件，并带 shell fixture 自测，防止旧项目名回流
- 六层 DDD 反向依赖门禁，生产代码禁止 `api/app/domain/trigger/types` 直接 import `internal/infrastructure`
- 跨服务私有表访问门禁，生产代码禁止直接 SQL 访问其他服务私有表，只保留已冻结的共享 timeline / outbox 例外
- 文件大小预算门禁，手写 Go / Markdown / PowerShell / Bash 文件继续按生产代码、测试 / runner、文档和脚本分档控复杂度；`tools/check-file-size-budget.ps1` 可按需输出 JSON / Markdown hotspot summary，且摘要格式已有 `check-local` 自测门禁；`loadtest/pushgateway` 已按 config / model / auth / scenario / util 同 package 文件拆分，避免在线通知 / Redis route / slow-client / resume smoke 继续堆进单个 `main.go`；`loadtest/receipt`、`loadtest/policyintegration`、`loadtest/sendmessage` 已按 config / model / auth / util 等同 package 文件拆分；`contacts-service` PostgreSQL privacy / source-policy 集成测试已拆到同 package 测试文件；`message-service` PostgreSQL revoke / edit / delete mutation 集成测试已拆出同 package 测试文件；`identity-service` PostgreSQL challenge command methods 已拆出，核心 repository 文件降到约 1.4k 行，app 层登录 / MFA / Refresh / Challenge 测试和 cmd 层 challenge / MFA / gateway-token / env 配置 helper 也已按主题拆分；`api-gateway` cmd 层 rate-limit / tenant-plan 配置测试已从 `main_test.go` 拆到同 package 测试文件，继续降低启动配置测试文件复杂度
- PowerShell / Bash 脚本解析门禁，`tools` 和 `loadtest` 下的 `.ps1` / `.sh` 都会进入本地检查，避免 smoke / 运维脚本语法回归
- `check-local` 覆盖门禁，新增 `tools/check-*.ps1` 默认必须接入主检查；间接或手动检查必须显式列为例外
- future service boundary 门禁，当前阶段只允许 9 个已实现服务目录存在；`search-service` / RAG / Agent 等只能保持 future / draft 文档状态，直到 current-brief 明确切换阶段
- 本地 Prometheus / Grafana / Alertmanager 覆盖门禁，已实现服务目录必须有 scrape / alert rules / dashboard 配置；`tools/run-local-observability-smoke.ps1` 可在本机已有镜像时验证 Prometheus rules、Grafana 9 服务 dashboard 和可选本地 Alertmanager null route 已由真实进程加载，也可按需把本地观测 smoke summary / report 写到 `H:\NexusIM\loadtest-results`，且摘要格式已有 `check-local` 自测门禁
- 服务 cmd 层启动配置测试门禁，已实现服务必须保留 `main_test.go` 覆盖启动 / 监听 / TLS / auth guard 配置
- 服务 cmd 构建门禁，9 个已实现服务的 `services/<service>/cmd/<service>` 必须能通过 `go build`
- 服务 Linux 构建门禁，9 个已实现服务的 cmd 包必须能以 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64/arm64` 交叉编译，保证本地 / Mac Docker runtime 的二进制基础不漂移
- 服务包级测试门禁，`go test ./services/...` 默认进入 `check-local`，覆盖 9 个已实现服务的轻量单测 / 跳过型集成测试
- 服务运行态端点门禁，已实现服务必须保留 `/healthz`、`/readyz`、`/debug/metrics` 和 `/metrics`
- Docker runtime / 本机镜像构建 / Mac 镜像同步 / 本地服务 compose 覆盖门禁，9 个已实现服务必须都有 `deploy/docker/<service>.runtime.Dockerfile` 和 `nexusim/<service>:local` 编排入口，本机构建脚本和双机镜像同步脚本默认从 `services/` 推导完整服务集合；`tools/run-local-service-health-smoke.ps1` 可启动本地服务 compose 并检查 9 个服务的 `/healthz` / `/readyz`，也可按需把 Docker resource snapshot 写到 `H:\NexusIM\loadtest-results`，再用 `tools/summarize-local-service-resource-snapshot.ps1` 生成健康态资源摘要，且摘要格式已有 `check-local` 自测门禁
- runbook consistency 门禁，防止 `development-progress.md` / service brief 已标记完成的事项继续残留在 `remaining-goals.md`
- 压测原始输出路径门禁，loadtest / smoke 默认结果不能写回仓库内 `loadtest/results`，原始数据默认落 `H:\NexusIM\loadtest-results`；9 服务 `capacity_summary` 合约、`tools/summarize-loadtest-capacity-baselines.ps1` 容量基线汇总器和 `tools/run-loadtest-capacity-baseline-suite.ps1` dry-run / 顺序执行入口已有本地自测门禁，suite 会区分 direct runner、需要后台角色的 stack runner 与 seeded-only runner；`deploy/local/docker-compose.service-workers.yml` 已提供本地 relay / consumer worker overlay，`loadtest/capacityseed` 已提供 message / conversation / delivery seeded runner fixture，且本地 seeded 短基线已覆盖 message / conversation / delivery；contacts stack 短基线已覆盖 contacts outbox relay 和 Kafka readback；identity stack 短基线已覆盖临时 webhook fixture 与 challenge-delivery-worker；receipt stack 短基线已覆盖 message / delivery / receipt relay-consumer 链路和 receipt Kafka readback；api-gateway stack 短基线已覆盖 secure mTLS + HMAC GatewayService facade、push WebSocket、delivery / receipt / policy Kafka readback；真实容量基线仍需继续补剩余服务、长时间运行和资源曲线
- outbox / projection / challenge delivery 等 repair / audit / cleanup operator，并通过 `docs/runbook/repair-operators.md` 提供统一入口；本地门禁会校验文档中的 operator mode 与对应服务 cmd 入口一致
- `check-local` 会显式检查子门禁脚本和原生命令 exit code，避免出现打印 `FAIL` 但总检查仍返回成功的假绿。
- worker / relay 非取消错误退避重试

更完整的 trace / alert / structured logging、故障演练和运维 workflow 属于后续目标，统一维护在 `remaining-goals.md`。

## 服务进度矩阵

| 服务 | 当前状态 | 最近进展 / 证据 | 详情入口 |
| --- | --- | --- | --- |
| `api-gateway` | 已落地、已接主链路 | quota source guard、future snapshot timestamp fail-closed、legacy quiet-window gate、OTel / Prometheus 本地观测、cmd rate-limit 配置测试拆分、secure stack 短基线和 `loadtest/demo --gateway-facade` `capacity_summary` | `service-briefs/api-gateway.md` |
| `identity-service` | 已落地、已接登录主链路 | login / refresh / MFA / recovery code / JWKS / challenge delivery、SMTP template、repository / cmd helper 和 app 测试拆分、loadtest `capacity_summary` 口径 | `service-briefs/identity-service.md` |
| `message-service` | 已落地、已接主链路 | `SendMessage` / 编辑 / 撤回 / 删除、`TEXT` + `IMAGE` / `FILE` / `VOICE` 附件引用消息、`LOCATION` / `CARD` 结构化 payload 消息、outbox / Kafka timeline、first-stage `/metrics` 和 OTel server span、mutation repository 测试拆分、loadtest `capacity_summary` 口径 | `service-briefs/message-service.md` |
| `conversation-service` | 已落地、已接主链路 | `GetSendContext` / member change / owner transfer / ACTIVE roster 分页与 role 过滤 / saga / audit operator、first-stage `/metrics` 和 OTel server span、loadtest `capacity_summary` 口径 | `service-briefs/conversation-service.md` |
| `delivery-service` | 已落地、已接主链路 | projection / `PullInbox` / `AckDelivery` / hide inbox / delivery outbox、loadtest `capacity_summary` 口径 | `service-briefs/delivery-service.md` |
| `push-gateway` | 已落地、已接主链路 | notify / ACK / resume / Redis route / Win-Mac / Sentinel / network-partition / TLS smoke、loadtest `capacity_summary` 口径 | `service-briefs/push-gateway.md` |
| `receipt-service` | 已落地、已接主链路 | receipt projection / outbox / audit / repair、`ListReceiptStates` repository 级批量查询、会话列表 unread / pinned / muted 过滤和 unread-first 排序、first-stage `/metrics` 和 OTel server span、loadtest `capacity_summary` 口径 | `service-briefs/receipt-service.md` |
| `contacts-service` | 已落地、已接主链路 | request source metadata / source policy gate / search-source privacy gate / contacts search / group filter / USER-TENANT-SYSTEM privacy / tenant privacy operator / outbox / audit / repair、repository 同 package 拆分、loadtest `capacity_summary` 口径 | `service-briefs/contacts-service.md` |
| `policy-service` | 已落地、已接主链路 | decision / user action restriction / projection / outbox / audit / repair、first-stage `/metrics` 和 OTel server span、loadtest `capacity_summary` 口径、本地 direct 短基线 | `service-briefs/policy-service.md` |
| `search-service` | 占位，尚未进入真实实现主线 | 无真实实现；等前 9 个服务收口后再进入 | `service-briefs/search-service.md` |

## 剩余目标入口

剩余目标、P2 hardening、收口门禁和逐服务 backlog 已拆到 `remaining-goals.md`。

本文只回答“现在开发到哪一步”；不要在这里继续堆待办长句。

## 当前阶段判断

当前最准确的阶段判断是：

```text
前 9 个微服务已经能跑通 IM 主链路，
现在处于“继续清现有服务的生产化 hardening”，
而不是“继续快速新增新服务”。
```

下一步优先级和剩余目标统一看 `remaining-goals.md`，不要在本页重复维护。

## 维护规则

- 这里只写阶段结论，不堆长历史。
- 新增真实里程碑时才更新本页。
- 具体 smoke 证据写入 `docs/runbook/loadtest/<service>/`。
- 服务细节变化写入 `docs/runbook/service-briefs/<service>.md`。
