# NexusIM Current Brief

本文件是每轮低 token 入口，只回答“现在处于什么阶段、下一步去哪里看”。
不要在这里维护长历史或完整待办。

## 按需读取

- 当前执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 服务细节：先读 `docs/runbook/service-briefs/README.md`，再读对应 service brief。
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/`
  或 `docs/runbook/history/`。

## 当前阶段

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端。

当前 active slice：

```text
client platform MVP foundation
```

Core services：api-gateway、contacts-service、conversation-service、delivery-service、
identity-service、message-service、policy-service、push-gateway、receipt-service。

AI foundation：action-executor、agent-service、ai-eval-service、mcp-gateway、
memory-service、rag-service、retrieval-gateway、search-service、skill-registry、
summary-service。

future platform / product services 的 10 个目标服务已经进入 product-active
first-stage implementation：admin-service、audit-service、control-plane-service、
knowledge-ingestion-service、media-service、model-gateway、notification-service、
presence-service、vector-index-service、workflow-service。

用户已明确切入客户端平台。当前短线不做临时 demo，而是先以浏览器、PC、Android
三端方式冻结可复用客户端架构：`clients/packages/protocol`、
`clients/packages/client-core`、`clients/web`、`clients/desktop` 和
`clients/android`。浏览器先可运行，PC / Android 复用同一协议和同步核心。
`clients/` workspace skeleton 已创建并通过 focused validation；`loadtest/clientweb`
已提供脚本化 BFF + push client-path smoke runner 和本地私有进程启动脚本。第一轮
本地 Web MVP smoke 已通过并归档；提交后的 clean baseline 也已通过并归档；
Windows wired `172.31.50.1` clean baseline 已通过并归档；BFF HTTP route metrics /
rate-limit adapter 已落。PC / Android runtime shell 已进入 assets prep / native
shell 骨架阶段；artifact / APK build wrapper 已有 dry-run-tested command plan，
artifact collector 已能生成低敏 SHA-256 manifest，且 build wrapper 已支持成功构建后自动归档；Android Docker builder
profile 已接 collector 但尚未运行；readiness report 显示 Docker / Compose 可用但 builder image 尚未构建。本机仍缺 Tauri CLI 与 Android
JDK 17+ / Gradle / SDK。下一步是补本地工具链或显式运行 builder profile，产出本地
Windows artifact、Android unsigned APK 和真实壳层 smoke。

真实业务语言边界已经固定为：Go 负责后端微服务、client BFF、控制面、事实源和
审计；TypeScript 负责 Web / PC / Android 的共享客户端协议、同步核心和 UI；
Rust / Kotlin 只做薄平台 bridge；Python 只做 AI worker、模型算法、eval 和离线
工具，不接管业务事实源。

完整扩展后的业务平台 / 数据平台 / AI Agent 平台 / 中间件平台总览已补到
`docs/architecture/target-architecture-complete.md`；中间件能力、runtime profile
和引入规则见 `docs/platform/middleware-catalog.md`。这些文档只定义长期边界和
adoption rules，不把服务数量、中间件或部署形态写死。

当前短线重点：

- client-platform：`api-gateway` client BFF 和 Web fetch / WebSocket /
  IndexedDB adapter first path 已落，`loadtest/clientweb` 本地 smoke 已通过并归档；
  loopback clean baseline 和 Windows wired `172.31.50.1` clean baseline 已通过，
  BFF HTTP route metrics / rate-limit adapter 已落；PC desktop / Android 已有
  target shell Web assets prep，PC 已有只读 `runtime_metadata` IPC，Android 已用
  WebViewAssetLoader 加载本地 assets，并已注册只读单方法 `NexusIMNative`
  metadata bridge，Web shell 已能展示 PC / Android native metadata，并已通过 shared
  `ClientShellActions` 接入 restore / logout；shell asset prep 已清理 stale
  bundle 并写低敏 hash manifest；artifact / APK wrapper 已能 dry-run 输出命令和缺失工具链，Android
  builder profile 已能静态校验，下一步做本地 artifact / APK 和真实平台 shell smoke。
- admin / audit / workflow：客户端切片完成后继续公开 API handoff、operator
  workflow、低敏审批 review artifact 和补偿边界。
- vector-index：继续 provider backend、pgvector / Milvus / OpenSearch 相关
  focused smoke。

## 不变量

- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做模型 / 算法 / eval 候选层；Go 负责控制面、状态和审计。
- future 服务之间不得读私有表，必须通过公开 API、事件或明确 port 串联。
- 客户端只连 `api-gateway` / `push-gateway`；PullInbox 是事实源，WebSocket 只是
  在线唤醒。
- 新发现待完成工作写入 `docs/runbook/remaining-goals.md`。
