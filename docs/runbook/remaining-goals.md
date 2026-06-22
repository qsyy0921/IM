# NexusIM Remaining Goals

这份文档只记录还没有完成的工作。当前进度总览见
`development-progress.md`，单服务事实见 `service-briefs/<service>.md`。

## 维护规则

- 新发现待办追加到本文件；完成后删除或改写为下一阶段 hardening。
- 不记录已完成证据，不写长历史，不替代 SDD / ADR / loadtest report。
- 当前 active slice：`client platform MVP foundation`。
- 长期完整架构基线：`docs/architecture/target-architecture-complete.md`。
- 中间件引入和替换规则：`docs/platform/middleware-catalog.md`。
- Fail-closed 治理规则：`docs/architecture/fail-closed-policy.md`。
- 推进策略和架构边界会持续更新；目标框不维护这些细节，按 `agent.md` 的 owner
  table 找到对应文档后再修改。
- 新功能开发前必须做架构分析；如果需要新增 / promotion 微服务或引入新中间件，
  先确认 owner、平台归属、数据模型、API / 事件、权限 / 审计、runtime profile、
  观测 / 部署影响和文档影响。
- 新增微服务必须进入根 README、目标架构、`service-briefs/README.md`、对应
  service brief、相关 SDD / ADR 和进度文档。
- 新增中间件 / provider 必须进入 `docs/platform/middleware-catalog.md`、相关
  runtime profile / compose / runbook、SDD / ADR；若影响 GitHub 首页总览，同步 README。
- 平台归属规则：中间件放入中间件平台；数据摄取 / 加工 / 分析放入数据平台；
  模型、向量、检索、RAG、Agent 和 Python worker 放入 AI / Agent 平台；
  客户端交互放入客户端平台；业务产品能力放入业务 / 产品平台；运维控制能力放入
  control-plane / ops 平台。
- 生产级 HA、长压、sizing 和完整系统测试暂不作为当前阻塞。

## 当前优先顺序

1. 完成 client platform MVP foundation：Web / Windows PC 已完成真实双用户客户端
   smoke，验证好友私聊和群聊 first path；群成员列表、移除成员、角色变更和
   owner transfer 第一路径已接入 BFF / client-core / Web shell，`loadtest/clientweb`
   也已扩展并在 clean committed smoke 中跑通这些群管理动作。继续补 PC MSI / NSIS
   installer、真实 signing input / `--require-valid` signed artifact 验证、更完整群设置、成员搜索 / 分页和后续 native SQLite bridge。
   Android APK / 真机 WebView smoke 后置到用户明确切回。
2. 回到 AI / Agent 主线：group memory eval、EvidencePack、Agent 真实业务动作、
   Python AI Worker 候选算法。
3. 继续 product-active 服务：workflow / audit / admin / notification / media /
   vector / model / knowledge / presence / control-plane。
4. 按完整架构补数据平台和中间件 profile，但不抢占当前客户端切片。
5. 9 个既有 IM 服务只回补阻塞 client / AI / product platform 的 P0/P1 或用户点名项。

## Fail-Closed Governance 未完成

- 开发任一切片时，都要清理该切片触达范围内的隐藏兜底 / fallback-like 代码；
  不能清理完的项必须在本节追加 owning service、文件范围、风险和建议处理方式。
- 新代码不得新增隐藏业务兜底。允许的 local-test adapter、compat window、repair /
  redrive 必须按 `docs/architecture/fail-closed-policy.md` 显式命名、显式 profile、
  显式审计 / 文档边界。
- 历史代码中仍有 `recovery` 命名和历史文档表述；不在当前客户端脏工作区里一次性
  全量重命名，避免误改 smoke 证据和正在开发的客户端切片。
- 新增 `tools/check-fail-closed-policy.ps1` 后，后续新增 / 修改行不得继续引入隐藏
  recovery 术语；确需 local-test adapter、compat window、repair / redrive 时，
  必须按 fail-closed policy 显式命名。
- 后续按服务逐步消除旧的业务 recovery 命名：优先 policy / auth / public gateway /
  AI action path；配置默认值类 `defaultValue` 可以机械重命名，不作为业务风险。

## Client Platform MVP 未完成

- `clients/desktop`：first-stage TypeScript runtime adapter 和 Tauri runner
  skeleton 已落；`createDesktopClientRuntime` 已能组装 shared `BFFClient` /
  `WebSocketPushTransport` / auth / inbox / send / ack queue；shared runtime
  lifecycle smoke 已覆盖 login / restore / refresh / logout 本地状态；thin
  `createDesktopShellActions` 已接 shared login / refresh / restore / logout 编排；继续接真实 desktop
  shell UI / artifact。
- Windows packaging：产出本地可安装或可运行的 Windows artifact；不要求生产签名。
  desktop workspace 已通过 repo-local `@tauri-apps/cli` 产出 first-stage standalone
  `nexusim-windows-desktop.exe`，并由 artifact collector 写入 ignored
  `clients/artifacts/<run-id>/manifest.json`。collector 现在还会写
  `README-windows-desktop.txt`，standalone exe package 会写 package-local
  `launch-nexusim-windows.ps1`，install plan 会校验这些 support files 并给出人工启动命令。
  `bundle:desktop` 已能基于 collected package 产出 unsigned local portable zip
  和低敏 summary。
  `smoke:desktop-artifact-launch` 已验证 exe 启动 / 保持 / 终止的 launch sanity。
  `smoke:desktop-composed` 已能把
  clientweb BFF / push summary 与 desktop launch 证据合并成低敏 JSON，但仍不是 GUI
  自动化。真实 Tauri WebView metadata callback smoke 已通过；Web / PC shell 已接
  登录、注册、好友、好友私聊、群聊列表、建群、消息列表和发送 first path。剩余工作是
  打磨真实 UI lifecycle，以及后续启用 MSI / NSIS installer bundling、真实 signing
  input 和 signed artifact 验证。`plan:desktop-signing` 已能检查显式 `signtool`、证书来源和
  timestamp URL readiness；`plan:desktop-installer` 已改为检查仓库内
  `tauri.installer.conf.json` profile、installer target、desktop artifact baseline
  和 signing readiness；默认开发 config 仍保持不打包。它们不 build、不签名，
  也不替代真实 installer / signing 流水线。`sign:desktop-artifact` 已补显式
  `--execute` 门控的 signing wrapper，默认仍只输出低敏 execution policy；真实
  签名还需要本机 `signtool`、timestamp URL、证书来源和 signed artifact 验证。
  `verify:desktop-signature` 已补只读 Authenticode 验证入口，当前 collected baseline
  实测为 `NotSigned`；release profile 仍需签名后用 `--require-valid` fail-closed 验证。
  `build:desktop-installer` 已补显式
  `--execute` 门控的 installer build 包装器，默认仍只输出计划；desktop installer
  planner 现在会按 `windows-desktop` 目标自动选择 collected manifest，不会被更新的
  Android manifest 遮住；真实 MSI / NSIS 构建仍要先满足 signing readiness 和
  valid Authenticode signature。
- `clients/android`：first-stage TypeScript runtime adapter 和 Kotlin WebView
  asset shell skeleton 已落；`createAndroidClientRuntime` 已能组装 shared
  `BFFClient` / `WebSocketPushTransport` / auth / inbox / send / ack queue；
  shared runtime lifecycle smoke 已覆盖 login / restore / refresh / logout 本地状态；
  thin `createAndroidShellActions` 已接 shared login / refresh / restore / logout 编排；
  Windows 本机 debug APK baseline 已产出，Kotlin 只做薄 bridge，业务协议和
  sync core 复用 TypeScript。
- Android packaging：产出本地 unsigned APK，并支持局域网 `api-gateway` /
  `push-gateway` 地址配置。`F:\IM\toolchains` 下存在 JDK / Gradle /
  Android SDK 文件目录，但当前 PowerShell / no-toolchain readiness 仍检测到
  Java 8，且 Gradle / `ANDROID_HOME` / `ANDROID_SDK_ROOT` 未就绪；继续 APK /
  Android WebView login smoke 前，需要先重新加载 F 盘 toolchain env 或显式使用
  Docker builder。既有 debug APK manifest 可作为历史产物参考，但当前 shell
  不能直接宣称 Android build readiness 已通过。Android Docker builder profile
  仍保留为容器化构建后备；需要下载 Node / Gradle / Android SDK toolchain 时必须
  显式运行 `build:android-apk:docker:bootstrap`。
- 三端 smoke：Web / PC / Android 都只能连 `api-gateway` 和 `push-gateway`；
  PullInbox 是事实源，WebSocket 只做在线唤醒。
- Contacts / friends UI：api-gateway BFF、client-core 和 Web / PC shell 已接
  first-stage contacts workbench，覆盖联系人申请、接受 / 拒绝 / 取消、备注、分组、
  删除、拉黑 / 取消拉黑，并已接好友直聊 first path：BFF 校验 ACTIVE 好友关系后
  创建 / 复用 DIRECT 会话。后续需要真实双用户客户端 smoke、隐私设置 UI、来源策略 /
  review-required 管理 UI、好友会话标题 / 头像等 richer read model。
- Group chat UI：注册账号、创建群聊、添加成员、退群、成员列表、移除成员、
  角色变更和 owner transfer first paths 已接入
  api-gateway BFF / client-core / Web / PC shell；创建群聊通过 conversation-service
  `CreateConversation` 创建当前用户 OWNER，添加成员 / 退群通过 conversation-service
  `CreateMemberChange`，成员读取通过 `ListConversationMembers`，owner transfer
  通过 `TransferConversationOwner`，不绕过服务私表。`loadtest/clientweb` 已覆盖
  成员列表、角色变更、owner transfer、移除成员和最终成员列表，且已有 clean
  committed smoke；后续继续补成员搜索 / 分页、更完整群设置、群标题 / 头像 read model。
- Local store：`IndexedDBMessageStore` 已有 first-stage persistence test；
  desktop / Android 已默认接 shared `KeyValueMessageStore` + WebView
  `localStorage` first-stage durable adapter，并有 cursor replay test；后续在
  native packaging/runtime ready 后替换为 SQLite bridge。
- Auth lifecycle：BFF `/api/auth/logout`、shared runtime login / refresh /
  restore / logout 编排、desktop / Android shell action wrapper 和 Web logout local
  cleanup 已落；Web shell 已通过 shared action 执行 login / refresh / restore / logout；后续在 PC / Android 真实 shell UI 中接入 lifecycle controls 并跑平台 shell smoke。
- Web hardening：browser platform adapter 当前使用 first-stage tab-scoped
  `sessionStorage` session store；生产 Web 鉴权后续需要 httpOnly cookie /
  provider-grade session 策略，避免把 token 长期放在 Web storage。
- WebView bridge：`globalThis.__NEXUSIM_CLIENT_SHELL__` 已能选择
  `windows-desktop` / `android` target 和 LAN endpoint；desktop / Android
  `shell-config.example.json`、renderer 与 target shell Web assets prep 已落；
  后续需要真实 Tauri / Android shell UI 接入现有 shell lifecycle actions，并用真实壳层 smoke
  验证。

## AI / Agent Platform 未完成

- `memory-service`：继续 group / collaborative memory 的 source refs、speaker /
  audience scope、validity、supersession、confidence、review state 和 profile
  aggregation。
- `ai-eval-service` / eval harness：继续增加低敏 case，区分 retrieval failure、
  reasoning failure、memory lifecycle failure 和 action boundary failure。
- `retrieval-gateway`：继续结构过滤、BM25 / vector / graph expansion、rerank、
  EvidencePack 覆盖率和 source coverage 口径。
- `rag-service` / `summary-service`：继续拒答、引用校验、source-ref regression、
  provider recovery 和 unsafe output fail-closed cases。
- `agent-service`：真实业务动作继续走
  `policy -> skill contract -> prepare audit -> proposal -> approval -> executor -> audit`；
  不允许 Agent 直接写业务库。
- `skill-registry` / `mcp-gateway`：补真实 MCP/provider tool 接入前的契约版本、
  risk level、tenant allowlist、audit 和 denial reason。
- `action-executor`：继续外部 adapter、rate limit、DLQ/redrive、repair guard 和
  low-sensitive result projection。
- Python AI Worker：扩展 embedding / rerank / memory extraction / planner / eval
  候选算法；输出只能是候选、hash、citation metadata 和低敏 diagnostics，Go 继续拥有
  权限、状态和持久化。

## Product-Active 服务未完成

- `media-service`：S3-compatible adapter、scanner、thumbnail / transcode provider、
  download policy、retention / delete proof。
- `notification-service`：SMTP / SMS / APNs / FCM adapter、bounce / suppression、
  provider redrive / audit、tenant template policy。
- `audit-service`：更多 Kafka ingestion source、持久 ingestion checkpoint / rewind
  operator、export worker / manifest、SIEM forwarding、retention cleanup、segment
  sealing、provider-grade audit export。
- `admin-service`：admin UI、更多下游公开 API adapter、更多 compensation adapter、
  provider-grade instruction 审批 / UI。
- `control-plane-service`：outbox relay、drift monitor、expiry / cleanup worker、
  api-gateway quota consumer、provider-grade rollout。
- `presence-service`：push-gateway session event consumer、`SubscribePresence`、
  stale scanner、presence outbox relay、Redis hot-state、privacy / contacts policy。
- `model-gateway`：真实 OpenAI / Claude / local-model provider、真实 embedding、
  rerank、outbox relay、route-refresh、budget-reset、cleanup worker。
- `knowledge-ingestion-service`：parser worker、tombstone / delete proof、真实 connector、
  parser / crawler provider handoff、ingestion repair。
- `workflow-service`：timer worker、更多 compensation adapter、instruction approval
  UI、external approval binding、external callback wait、outbox relay、repair
  operators。
- `vector-index-service`：memory / search chunk consumer、pgvector smoke、Milvus /
  OpenSearch backend、provider repair、真 provider backfill smoke。

## 数据平台 / 中间件平台未完成

- 数据平台：CDC / ingestion、lakehouse、OLAP、data catalog、metrics、feature store
  仍是目标架构能力，尚未作为 active implementation 切片推进。
- 中间件 profile：需要按 `deploy/local/` profile 拆 core、client-demo、
  observability、search-rag、media、workflow-agent、security、data-platform、
  ai-runtime；不要默认启动所有中间件。
- 中间件登记：新增 Redis / Kafka / PostgreSQL / OpenSearch / vector store /
  MinIO / Vault / Keycloak / OpenFGA / Temporal / OTel 等能力前，先登记 capability、
  owner、source-of-truth、runtime profile、健康检查、最小 smoke、迁移和安全边界。
- Adapter 边界：中间件 adapter 只能落在对应服务的
  `internal/infrastructure/<adapter>/`；domain / app 层不能 import 具体中间件 client。
- 数据平台只消费公开事件 / CDC 构建分析和 AI 数据资产，不能成为业务事实写入口。

## 9 个现有 IM 服务必要回补

| 服务 | 未完成工作 |
| --- | --- |
| `api-gateway` | legacy observation evidence、provider-grade 配置中心 quota、灰度治理、生产观测。 |
| `identity-service` | WebAuthn/passkeys、OIDC、多 issuer、KMS/HSM、完整风控、生产级 email/SMS provider。 |
| `message-service` | 删除 / 撤回 / 编辑深化、外部 proof workflow、发送链路生产观测。 |
| `conversation-service` | 群管理、owner transfer、历史窗口 / targeted replay repair。 |
| `delivery-service` | 更多 delivery event 消费方、projection repair、容量曲线。 |
| `push-gateway` | Redis HA、跨实例 resume、长时间在线容量曲线。 |
| `receipt-service` | 会话列表产品能力、更多摘要策略和容量曲线。 |
| `contacts-service` | 组织级策略、租户默认值、来源策略、隐私例外接入 admin/config。 |
| `policy-service` | provider-grade ReBAC / DSL、moderation / risk scoring、tenant quota、外部 audit pipeline。 |

## 后置生产化 Hardening

- 生产级统一观测：collector、Alertmanager、日志汇聚、SLO、retention。
- 分布式 HA / 故障演练：Redis / Kafka / PostgreSQL 更长时长和多故障组合。
- Repair / DLQ / audit 产品化：审批系统、运维 UI、批量 repair、外部审计。
- 容量和复杂度治理：9 服务 `capacity_summary` 长压 campaign、资源曲线、
  生产 sizing、文件拆分。

## 并行开发规则

可使用多个 sub-agent 并行推进，但必须按服务、文档集、测试面或只读审查问题拆分。
禁止同时改同一 proto、migration、service brief、架构章节或同一客户端 package。
