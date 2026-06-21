# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`；
阶段背景看 `current-brief.md`，详细进度看 `development-progress.md` 和 service brief。

## Active Slice

```text
client platform MVP foundation
```

目标：

```text
Browser + PC + Android client architecture + client BFF contract + reusable client packages
```

## 当前状态

- 用户明确切入客户端平台；当前 active slice 固定为
  `client platform MVP foundation`，直到 PC / Android runtime shell 或用户明确切换。
- 目标是先按最高标准冻结三端客户端架构，不写临时 demo：浏览器先可运行，PC
  desktop 和 Android 从第一天保留 package / runtime / packaging 边界，三端复用
  `protocol` / `client-core`。
- `clients/` workspace skeleton 已创建并通过 focused validation：`protocol`、
  `client-core`、`web`、`desktop`、`android` 均有明确 package / runtime contract；
  Web 端已有 Vite shell；PC desktop 和 Android 均已有 first-stage TypeScript
  runtime adapter（development session store、localStorage-backed persistent
  message store、static lifecycle/network ports）；PC desktop 已有 Tauri v2 runner skeleton（仅只读
  `runtime_metadata` command、bundle inactive，`frontendDist` 指向 shared prepared
  `clients/web/dist`）；Android 已有 Kotlin WebView asset shell / metadata
  bridge skeleton（不拥有 token 或消息事实），但仍不产出安装包 / APK。
- `api-gateway` 已新增 first-stage client BFF HTTP/JSON surface：`/api/auth/login`、
  `/api/auth/refresh`、`/api/me`、`/api/conversations`、
  `/api/conversations/{conversation_id}/messages`、`/api/messages/send`、
  `/api/delivery/ack`、`/api/contacts`、`/api/receipts`。BFF 复用既有
  gateway facade 和鉴权 / trusted metadata 注入，不读内部服务私表；BFF HTTP 层已接
  first-stage route metrics / rate-limit adapter，复用 api-gateway 既有限流器和
  `/debug/metrics` / `/metrics` 低敏观测管线，指标 label 只含固定 route / method /
  status_code；`/api/auth/logout` 已接入当前 gateway token 绑定 session 的
  self logout，BFF 只用鉴权上下文构造 identity `RevokeSession`，不接受客户端 body
  指定任意 tenant / user / device / session。
- `clients/web` 已新增 first-stage browser adapters：`BFFClient` 使用
  `api-gateway` HTTP/JSON BFF，`BrowserPushTransport` 使用 `push-gateway`
  WebSocket，`IndexedDBMessageStore` 作为 local cache / cursor store；Web shell
  已能走 login -> push connect -> conversation / manual open -> PullInbox -> send
  -> AckDelivery 的真实 adapter flow；Web shell 已接 first-stage logout UI，
  logout 会调用 BFF 当前 session revoke、断开 WebSocket、清空 IndexedDB local
  cache 和 UI session state；Web shell 现在也通过 browser platform adapter 复用
  `createClientRuntime` 的 auth / send / ack / logout 编排，不再手动复制 BFF /
  send queue 组装。
- `BFFClient` 已下沉到 `@nexusim/client-core`，Web 原路径仅 re-export；
  PC desktop / Android 后续复用同一 HTTP/JSON BFF adapter，不复制 Web 私有代码。
- `WebSocketPushTransport` 已下沉到 `@nexusim/client-core`，Web 原
  `BrowserPushTransport` 路径仅 re-export；PC desktop / Android 后续复用同一
  在线唤醒 transport。
- `@nexusim/client-core` 已新增 `createClientRuntime`，统一组装 BFF API、push
  transport、auth session、inbox sync、send queue 和 ack queue；`clients/desktop`
  / `clients/android` 已分别新增 `createDesktopClientRuntime` /
  `createAndroidClientRuntime`；shared runtime 已提供 first-stage logout 编排，
  会调用 BFF logout、断开 push、清理 secure session store 和 local message store；
  shared runtime 现在也提供 `login` / `refresh` / `restoreSession`，登录和刷新会写入
  平台 secure session store，重启式 runtime 重新创建后可从平台 store hydrate
  auth manager；`ClientShellActions` 现在覆盖 login / refresh / restore /
  logout，Web shell 登录面板已通过该 contract 执行登录、刷新、恢复和登出，
  PC / Android WebView 后续复用同一 UI action contract。
- `clients` workspace 已新增本地构建前置检查
  `npm --prefix clients run check:build-prereqs`；该检查只读取本机 Rust /
  Tauri / JDK / Gradle / Android SDK 状态，不安装依赖、不拉包、不使用 `npx`
  远程解析。当前机器可用 `rustc` / `cargo`，且已通过
  `npm --prefix clients install` 安装 repo-local `@tauri-apps/cli`；readiness
  已能识别 `local:tauri`，Windows desktop artifact path ready。Android 侧仍是
  JDK 8 且缺 Gradle / Android SDK，因此 Android APK 仍未 ready。
- `IndexedDBMessageStore` 已有 first-stage persistence test harness，覆盖
  cursor persistence、message seq ordering、pending send、send accepted 后稳定
  seq key 迁移、防 replay duplicate，以及 send failed 本地状态。
- `@nexusim/client-core` 已新增 shared `KeyValueMessageStore`，desktop /
  Android 默认通过 WebView `localStorage` wrapper 获得 first-stage persistent
  local cache；focused test 覆盖 store 实例重开后的 cursor persistence、pending
  send、accepted send 稳定 key 迁移、防 replay duplicate、failed-send 状态和
  conversations-needing-sync 列表。desktop / Android 均已预留 `sqlite` store
  config，且在 native bridge 未实现前 fail-fast；后续生产化再接 native SQLite bridge。
- `LocalMessageStore.listMessages` 已提升为 shared client-core port；
  `MemoryMessageStore`、`KeyValueMessageStore` 和 Web `IndexedDBMessageStore`
  现在都有同一读缓存语义，并补了 pending -> accepted-send 迁移去重测试。
- `clients` workspace 已新增 focused runtime lifecycle smoke：
  `npm --prefix clients run test:runtime-lifecycle` 会编译并实例化 desktop /
  Android runtime，验证 login 持久化 session、restoreSession hydrate auth manager、
  refresh 更新持久 refresh token，以及 logout 清理 secure session store 和 local
  message cache。该测试现在也通过 desktop / Android thin shell actions 调用
  shared login / refresh / restore / logout 编排，证明真实壳层可以接入统一 action contract。该测试不依赖
  Tauri CLI / Android SDK，不替代真实安装包或 APK。
- `clients` workspace 已新增 focused browser platform adapter test：
  `npm --prefix clients run test:web-platform` 覆盖 browser session store、
  browser platform identity、network / lifecycle ports 和 unsupported wakeup
  boundary；Web session store 仅作为 first-stage tab-scoped sessionStorage
  adapter，后续生产 Web 鉴权仍需 httpOnly cookie / provider-grade session 策略。
- `clients` workspace 已新增 focused Web shell lifecycle contract test：
  `npm --prefix clients run test:web-shell-actions` 会确认 Web shell 通过 shared
  `ClientShellActions` 调用 login / refresh / restore / logout，且不直接调用
  runtime auth lifecycle 方法，避免 PC / Android WebView 后续出现另一套 UI action path。
- Web shell 支持 first-stage WebView bridge config：
  `globalThis.__NEXUSIM_CLIENT_SHELL__` 可由 PC / Android 壳层注入 target、
  API / WebSocket 地址、device / installation / app version 和 session key；当前
  focused test 覆盖 `windows-desktop` 与 `android` target selection。该 bridge 只
  做 runtime identity / config 选择，不授予文件系统或 native token 权限。
- Android WebView 已注册只读 `NexusIMNative` JavaScript bridge；该 bridge 现在只暴露
  单个 `runtimeMetadata()` 方法。Web 端可读取它用于诊断，且 focused test 覆盖合法
  metadata、错误 target 和 malformed JSON 的 fail-closed 行为。该 bridge 不暴露
  token、storage、文件系统或 message API。
- Web shell 的运行入口面板已展示 shell target、PC Tauri `runtime_metadata`
  和 Android native bridge metadata；该展示只用于本地诊断，不改变客户端权限边界。
- Web app 会在主 bundle 前加载 `nexusim-shell-config.js`；`clients/desktop` 和
  `clients/android` 已各自提供低敏 `shell-config.example.json`，并由
  `clients/tools/render-shell-config.mjs` 渲染成注入脚本。`test:shell-config` 和
  desktop / Android skeleton validator 会拒绝 unsupported target 与 token /
  secret / password 等敏感字段。
- `clients/tools/prepare-shell-web-assets.mjs` 已把 Web build 与目标 shell config
  注入串起来：`build:shell-assets:desktop` 会构建 Web 并把 `windows-desktop`
  config 写入 `clients/web/dist/nexusim-shell-config.js`；`build:shell-assets:android`
  会构建 Web、复制到 Android app assets，并写入 `android` config。Android native
  skeleton 已改成 `WebViewAssetLoader` 加载本地 app assets，禁用 file / content
  access；Gradle `preBuild` 会调用同一资产准备脚本。shell asset prep 会在
  source / output 不同时先清理目标目录，避免 APK / shell 包混入旧 bundle，并写入
  低敏 `nexusim-shell-assets-manifest.json`（relative path / bytes / SHA-256）。artifact
  build wrapper 会在调用 Tauri / Gradle 前验证 prepared assets 与 manifest 一致；desktop
  wrapper 会在已准备并校验 manifest 后设置
  `NEXUSIM_SKIP_SHELL_ASSET_PREP=true`，避免 Tauri `beforeBuildCommand` 重复跑同一
  Web build，而直接运行 Tauri 仍会自动准备 assets；Android wrapper 会在已准备并校验
  manifest 后传 `-Pnexusim.skipWebAssetPrep=true`，避免 Gradle `preBuild` 重复跑同一
  Web build，而直接运行 Gradle 仍会自动准备 assets。该流程已通过 focused
  `test:shell-web-assets`、desktop / Android validators 和实际 shell asset build。
- `clients/tools/build-desktop-artifact.mjs` 与
  `clients/tools/build-android-apk.mjs` 已提供 first-stage artifact / APK build
  wrapper；`test:artifact-builders` 覆盖 dry-run 命令计划和低敏输出。Windows
  desktop wrapper 已能用 repo-local Tauri CLI 构建 first-stage standalone
  `nexusim-desktop.exe`；当前仍未启用 MSI / NSIS installer bundle。Android 缺
  JDK 17+ / Gradle / Android SDK，所以 local APK wrapper 仍会 fail fast。Android
  wrapper 现在也支持 custom shell config path，可用于后续 metadata smoke 注入临时
  loopback callback config，dry-run 不输出本机绝对路径。
- `clients/tools/collect-client-artifacts.mjs` 已提供 first-stage artifact
  collector；`test:artifact-collector` 覆盖 fake APK / Windows artifact 归档、
  SHA-256 manifest、dry-run 不写文件和不泄露本机绝对路径。真实 artifact / APK
  产出后可用 `collect:client-artifacts` 写入 ignored `clients/artifacts/<run-id>/`
  并生成低敏 manifest；`build:desktop-artifact:collect` 和
  `build:android-apk:collect` 会在 native build 成功后自动执行该归档步骤。`plan:artifact-install`
  会读取 collected manifest 并输出低敏 Windows / Android 安装 checklist；它现在也报告
  Android `adb` availability 与 Windows installer launch support，但仍不启动安装器、
  不连接设备、不安装 APK，也不输出本机绝对路径。2026-06-21 已产出第一份
  Windows desktop standalone exe 归档 manifest：
  `clients/artifacts/2026-06-21T202009Z/manifest.json`（ignored local artifact
  storage）；`npm --prefix clients run smoke:desktop-artifact-launch` 已验证该
  standalone exe 能启动为进程、保持 5 秒并被工具干净终止。该 launch sanity smoke
  不等同于登录 / PullInbox / WebSocket 的完整 PC UI smoke；`npm --prefix clients run
  smoke:desktop-composed` 已提供低敏组合 smoke，可消费现有 clientweb BFF / push
  summary，并与 desktop artifact launch 结果合并。它证明公开客户端链路证据和
  desktop 进程启动证据能被同一工具归档，但仍不声称 Tauri WebView 内完成登录级
  GUI 自动化；当前仍没有 Android APK baseline。
- Web shell 已新增 first-stage WebView metadata smoke callback 契约：目标 shell
  config 可注入 loopback-only `smokeCallbackURL` / `smokeRunID` / `smokeMode=metadata`，
  WebView 内部读到 PC Tauri 或 Android bridge metadata 后会 POST 低敏 metadata
  report；该 report 不包含 token / password / 本机路径，也不执行登录或业务动作。
  2026-06-22 已跑通真实 Tauri WebView metadata callback smoke：
  `npm --prefix clients run smoke:desktop-webview-metadata`，证明 WebView 内能读取
  PC Tauri `runtime_metadata` 并回调低敏 report；该 smoke 仍不声明登录、
  PullInbox、WebSocket 或 AckDelivery 已在 Tauri WebView 内完成。Android 也已新增
  `npm --prefix clients run smoke:android-webview-metadata` runner；dry-run 已可验证
  低敏 plan，真实运行会构建注入临时 metadata callback 的 APK、通过 `adb reverse`
  暴露 loopback callback、安装并启动 `com.nexusim.android/.MainActivity`，等待
  `NexusIMNative.runtimeMetadata()` 回调。该 Android runner 仍受 APK toolchain
  阻塞，尚未形成真实设备 baseline。
- PC Web shell 已新增登录级自动化前置：Web UI 暴露稳定 `data-testid`
  automation contract 和 `ack-status` 诊断，`npm --prefix clients run
  smoke:desktop-webview-login` 可通过 WebView2/CDP 外部驱动 Tauri WebView 登录、
  接收 externally-triggered `delivery.notify`、PullInbox 并 AckDelivery；现有
  `loadtest/clientweb/run-local-smoke.ps1 -RunDesktopWebViewLoginSmoke` 会在本地
  BFF / push 栈存活期间生成临时 fixture 并调用该 driver。2026-06-22 已在提交
  `c72ea512` 上完成 clean 登录级 Tauri WebView smoke，归档见
  `docs/runbook/loadtest/client-platform/loadtest-report-20260622-desktop-webview-login-smoke.md`；
  summary 记录 `git_dirty=false`，覆盖 WebView 内 login、externally-triggered
  `delivery.notify`、PullInbox、message observe 和 AckDelivery。
- Android 已新增 opt-in Docker builder profile：
  `deploy/docker/client-android-builder.Dockerfile` 和
  `deploy/local/docker-compose.client-builders.yml`。`validate:builder-profile`
  只做静态校验，不拉镜像；`build:android-apk:docker` 现在是受控 wrapper，默认要求
  `nexusim/client-android-builder:local` 已存在，不会隐式下载 toolchain；
  `build:android-apk:docker:bootstrap` 是显式 bootstrap 入口，会在缺镜像时下载 /
  构建 Node、Gradle 与 Android SDK toolchain。该 profile 已接 `build:android-apk:collect`，成功构建后会把
  APK 和低敏 manifest 写入 `clients/artifacts/android/docker-android-debug/`；镜像 build context 已缩到
  `deploy/docker`，避免发送整个仓库。当前仍未构建镜像，也未声称 APK baseline。
- `clients/tools/report-client-artifact-readiness.mjs` 已提供低敏 readiness
  report；`test:artifact-readiness` 覆盖 schema、无敏感字段和无本机绝对路径。
  报告已区分 Android Docker builder image build command 与实际 builder run command，
  会显示 prepared shell asset manifest verification 状态，并输出低敏
  `nextActions`，不会自动下载或构建。当前报告显示 Windows desktop ready
 （通过 repo-local `local:tauri`），Docker / Compose 可用、Android builder profile
  可解析，但 `nexusim/client-android-builder:local` image 尚未构建；Android 本地路径
  仍缺 JDK 17+ / Gradle / Android SDK。readiness 下一步现在指向受控
  `npm --prefix clients run build:android-apk:docker:bootstrap`，并显式标记会下载 toolchain。
- `clients/tools/plan-client-shell-smoke.mjs` 已提供低敏 browser / desktop /
  Android shell smoke plan；它汇总 toolchain readiness、prepared asset
  verification、artifact presence、collected-artifact install readiness、安全构建命令、
  artifact install plan 命令、per-target manual smoke checklist 和 shared BFF / push
  smoke 命令，不启动服务、不下载工具链、不连接设备、不安装 artifact、不声称已有
  installer / APK。Windows desktop smoke plan 在 collected artifact ready 时会包含
  `smoke:desktop-artifact-launch` 作为 launch sanity step。其 native artifact 状态现在区分 raw build output discovery 与
  collected artifact manifest readiness，避免已归档产物和本地 build 输出源混淆；Android
  plan 在 collected APK + adb ready 时会包含 `smoke:android-webview-metadata`
  作为 metadata-only WebView bridge smoke。
- `loadtest/clientweb` 已新增 first-stage scriptable client smoke runner：
  准备阶段用 identity / api-gateway gRPC 注册用户、seed 会话并创建 JOIN；真实客户端
  验证段只走 `api-gateway` HTTP BFF 和 `push-gateway` WebSocket，覆盖 BFF login、
  push hello、BFF SendMessage、`delivery.notify`、BFF PullInbox、BFF conversation
  list 和 BFF AckDelivery。`loadtest/clientweb/run-local-smoke.ps1` 可启动本地私有
  非 TLS 后端+BFF+push 进程并运行该 runner；它是客户端链路 smoke 底座，不替代
  既有 secure mTLS demo。
- 2026-06-21 已跑通第一轮本地 Web client -> BFF -> push smoke，归档见
  `docs/runbook/loadtest/client-platform/loadtest-report-20260621-client-web-bff-push-smoke.md`。
  该报告记录 `git_dirty=true`，只能作为本轮 WIP 验证。
- 2026-06-21 已在提交 `6069b45a` 上重跑 clean baseline，归档见
  `docs/runbook/loadtest/client-platform/loadtest-report-20260621-client-web-bff-push-clean-baseline.md`；
  summary 记录 `git_dirty=false`，覆盖 BFF login、push hello、BFF SendMessage、
  `delivery.notify`、BFF PullInbox、BFF conversation list 和 BFF AckDelivery。
- `loadtest/clientweb/run-local-smoke.ps1` 已支持 `-BindHost` / `-ClientHost`，
  可以把本地私有后端+BFF+push 栈绑定到 wired LAN 私有地址。2026-06-21 已用
  `172.31.50.1` 跑通第一轮 WIP wired-address smoke，归档见
  `docs/runbook/loadtest/client-platform/loadtest-report-20260621-client-web-bff-push-wired-172-smoke.md`；
  summary 记录 `git_dirty=true`，只作为脚本改动验证，仍需提交后 clean 复跑。
- 2026-06-21 已在提交 `4148e3c9` 上重跑 `172.31.50.1` wired-address clean
  baseline，归档见
  `docs/runbook/loadtest/client-platform/loadtest-report-20260621-client-web-bff-push-wired-172-clean-baseline.md`；
  summary 记录 `git_dirty=false`，覆盖同一 BFF login、push hello、BFF
  SendMessage、`delivery.notify`、BFF PullInbox、BFF conversation list 和 BFF
  AckDelivery 链路。
- 真实业务语言选择：后端和 client BFF 继续使用 Go；浏览器、PC desktop 和
  Android 的共享协议 / 同步核心 / UI 使用 TypeScript；Tauri 的 Rust、Android
  Kotlin 只作为薄平台桥；Python 只用于 AI worker / eval / 离线工具，不进入
  客户端主链路和后端事实源。
- 客户端只面向 `api-gateway` 和 `push-gateway`；PullInbox 是消息事实源，WebSocket
  是在线唤醒。
- v0.1 SDD 见 `docs/sdd/client-platform.md`，短 brief 见
  `docs/runbook/client-platform.md`。
- 长期完整架构以 `docs/architecture/target-architecture-complete.md` 为准；客户端、
  业务平台、数据平台、AI / Agent 平台和中间件平台的后续开发必须对齐该文档的边界。
- 10 个目标服务的 SDD v0.1 draft 已存在，组合 promotion 边界见
  `docs/sdd/future-platform-services.md`。
- 10 个目标服务均已进入 product-active first-stage implementation。
- 最新焦点在 admin / audit / workflow / vector-index 之间补公开 API handoff、
  compensation boundary、provider backend 和 focused smoke。
- `audit-service` 已新增 first-stage `CreateAuditExport` / `GetAuditExport`
  job metadata API；只保存低敏 filter hash / redaction profile / requester refs。
- `admin-service` 已新增 `AUDIT_EXPORT_REQUEST -> audit-service.CreateAuditExport`
  公开 API adapter；不读 audit-service 私有表。
- `audit-service` 已新增 first-stage `admin-consumer`，消费公开
  `im.admin.events` 并映射为低敏 `AppendAuditRecord`；Kafka offset 只在 append
  成功后提交，持久 ingestion checkpoint / rewind 仍是后续项。
- `workflow-service` 已新增 first-stage
  `ListWorkflowCompensationInstructions` 公开查询 API，按 workflow 返回低敏
  compensation instruction refs / version / status；不读 admin-service 私表。
- `loadtest/workflow` 已新增 first-stage workflow operator CLI，通过 workflow-service
  公开 gRPC get workflow、record decision、查询 compensation instruction metadata；
  它只输出低敏 refs / version / status，不输出 payload / reason 原文，并在
  `record-decision` 本地拒绝明显敏感的 decider / policy / reason / evidence ref；
  `-decision-manifest` 可作为 first-stage external approval binding；本地 writer /
  validator 可生成和校验仓库外低敏 decision manifest。
- 已新增本地 workflow compensation instruction manifest writer / validator /
  self-test，用于生成和校验仓库外低敏 control-plane rollback instruction JSON；
  manifest 只保存 workflow / payload hash / config target / operator / reason ref，
  不保存 rollback payload、operator reason 原文或本机文件路径。
- `workflow-service compensation-instruction-import` 已纳入机器可读
  `repair-operators.catalog.json`，可进入本地 approval request / decision /
  invocation 链路；导入 instruction metadata，不直接执行 rollback mutation。
- approved repair invocation 会在 `workflow-service compensation-instruction-import`
  执行前校验 instruction manifest，只在 summary 中记录 manifest hash / count，
  不输出 manifest 路径、payload ref 文件正文或 operator reason 原文。
- 已新增本地静态 repair approval review page writer，用于把 plan / request /
  decision / invocation / audit bundle 渲染为低敏 HTML 审批页；页面只展示 hash、
  path hash、env key 和 preflight 摘要，不复制 reason、payload、manifest path
  或 evidence 原文。

## 下一步优先级

1. 补 Android JDK 17+ / Gradle / Android SDK，或显式运行
   `npm --prefix clients run build:android-apk:docker:bootstrap` 构建 Android Docker builder
   image；镜像存在后运行 `npm --prefix clients run build:android-apk:docker` 产出首个 APK + manifest。
2. 在真实 Android shell UI 中接入现有 shell action，并在工具链 ready 后跑平台 shell smoke。
3. 后续把 desktop / Android first-stage localStorage store 替换为 native
   SQLite bridge，并补真实平台 runtime smoke。
4. 再回到 workflow compensation adapter / instruction approval UI / ops 管理；
   当前已有本地 workflow get / decision / decision manifest / instruction list CLI，
   低敏 compensation instruction manifest 生成 / 校验，以及 catalog-backed import
   approval 链路、invocation preflight 和静态 review page；后续可接正式审批 UI。
5. 继续明确其它下游公开 admin API adapter。
6. 在镜像可用后补 vector-index focused pgvector smoke；后续再接 Milvus /
   OpenSearch backend、provider repair 和真 provider backfill smoke。
7. 可继续 notification SMTP / SMS / APNs / FCM adapter 或 bounce-suppression。
8. 新发现待办写入 `docs/runbook/remaining-goals.md`。

## 工作方式

- 按服务小切片闭环：代码、必要测试、文档一起收。
- 当前任务涉及哪个服务，只读对应 service brief 和必要 SDD 章节。
- 不一次性 promotion 全部 future 服务，不铺空目录。
- 小改跑 focused checks；proto、migration、跨服务 adapter、安全边界或提交推送前再扩大门禁。

## 硬边界

- 不把媒体二进制塞回 message-service。
- 客户端不直接调用内部微服务，不读取任何服务私表。
- 客户端 local store 只做缓存 / 离线队列，不成为服务端事实源。
- 不把 identity 局部 webhook / SMTP 扩成完整 notification-service 的生产承诺。
- admin / control-plane / workflow / audit 之间只能走公开 API、事件或明确 port。
- model / vector / ingestion 不得绕过 retrieval、policy、EvidencePack、approval 和 audit。
- 不回滚用户已有修改。

## 文档路由

- 当前阶段背景：`docs/runbook/current-brief.md`
- 长期完整架构：`docs/architecture/target-architecture-complete.md`
- 中间件能力目录：`docs/platform/middleware-catalog.md`
- 剩余待办：`docs/runbook/remaining-goals.md`
- 服务入口：`docs/runbook/service-briefs/<service>.md`
- 总览：`docs/runbook/development-progress.md`
