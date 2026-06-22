# NexusIM Current Goal

本文件只维护当前可执行目标，保持短入口。长事实不要放这里：

- 阶段背景：`docs/runbook/current-brief.md`
- 客户端细节：`docs/runbook/client-platform.md`
- 未完成工作：`docs/runbook/remaining-goals.md`
- 长期架构：`docs/architecture/target-architecture-complete.md`

## Active Slice

```text
client platform MVP foundation
```

目标：

```text
Browser + PC + Android client architecture + client BFF contract + reusable client packages
```

## 当前事实

- 该 active slice 继续有效，直到 PC / Android runtime shell 或用户明确切换。
- 浏览器、PC desktop、Android 共用 `clients/packages/protocol` 和
  `clients/packages/client-core`；客户端只连 `api-gateway` 和 `push-gateway`。
- `api-gateway` 已有 first-stage client BFF surface；Web shell 已能通过 BFF /
  push path 跑通本地 smoke。
- PC desktop 已有 Tauri shell、desktop SQLite bridge、standalone artifact、
  metadata smoke 和登录级 WebView smoke 证据。
- Android 已有 WebView asset shell、native metadata / SQLite bridge contract、
  no-toolchain action asset contract、ADB readiness report、Docker builder profile
  和登录级 WebView smoke safe preflight plan。
- 当前 Android 阻塞点仍是 APK baseline：本机缺 JDK 17+ / Gradle / Android SDK，
  Docker builder image `nexusim/client-android-builder:local` 尚未构建。
- 不要隐式下载 Android toolchain；只有用户明确接受下载时，才运行
  `npm --prefix clients run build:android-apk:docker:bootstrap`。

## 下一步优先级

1. 继续做不需要下载工具链的 Android / desktop shell contract、plan、readiness
   和 smoke runner hardening。
2. 若用户明确接受 Android toolchain 下载，运行 Docker builder bootstrap，产出
   first unsigned APK + collected manifest。
3. APK ready 后跑 Android metadata / login WebView smoke，并验证
   `android-sqlite` 真机路径。
4. 客户端切片收口后，再回到 workflow compensation adapter / instruction approval
   UI / ops 管理。
5. 新发现待办写入 `docs/runbook/remaining-goals.md`，不要把长待办复制回本文件。

## Focused Checks

客户端小切片优先跑相关脚本，避免频繁完整门禁：

```powershell
npm --prefix clients run check:no-toolchain
git diff --check; git diff --cached --check
```

`check:no-toolchain` 先验证自身 dry-run plan 不含下载 / 安装 / 启动设备类操作，
且该 dry-run plan 必须带 `executionPolicy.planOnly=true`，声明它只描述 focused
gate，不执行 checks、不运行 npm scripts、不读取设备状态。
再聚合 client workspace validation、workspace TypeScript、Web PWA、shell config、
desktop / Android native skeleton validation、Web platform、shared runtime /
local-store / IndexedDB contracts、Web shell lifecycle /
automation / smoke-report contracts、shell asset / prep-wrapper、desktop /
Android action asset、desktop artifact launch / composed smoke dry-run contracts、
clientweb smoke hook contract、artifact readiness / install-plan / builder /
collector contracts、Android builder profile / wrapper contracts、desktop WebView
metadata / login dry-run contracts、Android metadata / login dry-run contracts、
Android device / WebView devtools readiness and parser contracts、Android
platform readiness 和 shell smoke plan checks；它不构建 native artifact
或 APK、不启动 Docker、不安装 APK、不启动 Activity、不打开 `adb reverse`、
不下载工具链。它会通过 Android readiness report 只读查询 ADB / device state。
`plan:shell-smoke` 也会把它作为默认 focused gate 暴露出来，并且顶层
`executionPolicy.planOnly=true` 必须声明它不会执行 checklist 命令、不会启动服务、
不会下载工具链、不会启动 Docker、不会接触设备。需要定位失败时再跑单项脚本。
Android Docker builder bootstrap 若出现在 shell smoke plan 中，必须带有
`downloadsToolchain=true` 和 `requiresExplicitUserOptIn=true`，只能作为显式用户
同意后的下一步，不属于 no-toolchain gate。Android Docker builder plan / dry-run
本身也必须带 execution policy，声明 dry-run 只读 Docker builder 状态、不运行 Docker
命令、不构建 image / APK、不写 artifact manifest、不安装或接触设备；bootstrap 路径
必须暴露 `plannedDownloadsToolchain=true`，但 dry-run 不能实际下载。`plan:artifact-install` 输出的
`adb install` / `Start-Process` 也只是 manual checklist，相关步骤必须带
`manualOnly=true` 和安装 / 启动 / 设备接触风险字段，脚本本身不得安装或启动
artifact；顶层 `executionPolicy.planOnly=true` 必须声明它不会执行 checklist
命令、不会下载工具链、不会启动 Docker、不会接触设备。
Build prerequisites report 必须带 execution policy，声明它是 report-only 本机能力
探测；可运行本机工具链版本检查、读取环境变量和 repo-local node bin 状态，但不得
构建 artifact、启动服务 / Docker、安装或接触设备、下载工具链、泄露本地绝对路径或
原始 command output。
Artifact readiness report 必须带 execution policy，声明它是 report-only 本机状态
探测；可读取本机工具链、Docker builder、shell asset manifest 和 native-store
source readiness，但不得构建 artifact / Docker image、准备或收集 artifact、写
manifest、启动服务 / Docker、安装或接触设备、下载工具链或泄露本地绝对路径。
Desktop / Android artifact builder 的 `--dry-run` 输出也必须带 execution policy，
声明不会执行 Tauri / Gradle build、不会准备或验证 shell assets、不会收集 artifact、
不会启动 Docker、不会安装或接触设备。
Artifact collector 的 `--dry-run` 输出必须带 execution policy，声明它只发现候选
artifact source 并读取元数据，不复制 artifact、不创建输出目录、不写 manifest。
Desktop artifact launch smoke 的 `--dry-run` 输出必须带 execution policy，声明它会
只读校验 manifest / artifact hash，但不会启动或终止 artifact 进程。
Desktop composed smoke 在 `--clientweb-summary + --launch-dry-run` 模式下也必须带
execution policy，声明它只读取既有 clientweb summary、只读校验 artifact manifest /
bytes、嵌套 desktop launch dry-run，不启动服务、不启动 desktop artifact、不联网、
不启动 Docker、不安装或接触设备、不下载工具链。
`plan:android-webview-login-smoke` 也是 plan-only：它可以列出 APK build、adb
install、Activity start、adb forward 和 runner 命令，但顶层 execution policy
必须声明这些命令不会被 plan 脚本执行。
Desktop WebView metadata / login smoke 的 `--dry-run` 输出也必须带 execution
policy，声明不会构建 artifact、不会启动 artifact、不会启动 callback / CDP
自动化、不会连接 BFF 或发送消息。
Android metadata smoke 的 `--dry-run` 输出也必须带 execution policy，声明不会构建
APK、不会安装、不会启动 Activity、不会打开 `adb reverse`、不会接触设备。
Android login-level WebView smoke runner 的 `--dry-run` 输出也必须带 execution
policy，声明不会构建或收集 APK、不会安装、不会启动 Activity、不会打开 adb
forward、不会连接 BFF 或发送消息。
Android platform / device / WebView devtools readiness reports 也必须带
execution policy，声明它们是 report-only 本机状态探测；可只读查询本机工具链、
Docker builder 状态、ADB device list 或 WebView devtools socket evidence，但不得
下载工具链、构建 artifact / Docker image、安装 APK、启动 Activity、打开 adb
reverse / forward 或泄露 raw device / socket identifier。

只有跨服务、生成代码、migration、service-registry、Docker / compose、安全边界、
提交推送前或用户明确要求时，才扩大到完整 `.\tools\check-local.ps1`。

## 硬边界

- 客户端 local store 只做缓存 / 离线队列，不成为服务端事实源。
- 客户端不得直接调用内部微服务，不读取任何服务私表。
- PullInbox 是消息展示事实源；WebSocket 只做在线唤醒。
- TypeScript 负责三端共享客户端协议、同步核心和 UI；Rust / Kotlin 只做薄平台桥。
- Python 只做 AI worker / eval / 离线工具，不接管客户端主链路或业务事实源。
- 不回滚用户已有修改。
