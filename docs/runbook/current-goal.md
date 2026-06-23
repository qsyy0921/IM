# NexusIM Current Goal

短入口，只维护当前可执行目标。长事实分别放在：

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

- 短线优先浏览器端和 Windows PC 端；Android 后置到用户明确切回。
- Web / PC / Android 共用 `clients/packages/protocol` 和
  `clients/packages/client-core`；客户端只连 `api-gateway` BFF 和 `push-gateway`。
- Web / PC shell 已接账号密码登录、注册、好友工作台、好友私聊、群聊列表、建群、
  群成员添加 / 退群、从好友列表邀请入群、成员列表、移除成员、角色变更、owner
  transfer、成员搜索 / 角色过滤 / 分页、群资料卡、权限感知群设置入口、邀请来源提示、
  群公告、消息列表、发送、PullInbox 和 ACK。
- 群设置 UI 已按资料、成员、操作分区；资料和成员事实仍只来自 conversation-service
  经 api-gateway BFF 暴露的公开接口，不维护本地假成员列表。
- 群标题 / 头像 URI / 群公告已接 first-stage read/update：`conversation-service` 拥有事实，
  `api-gateway` 暴露 BFF，Web / PC shell 只通过 BFF 读写。群头像上传 first path
  已通过 `api-gateway` BFF -> `media-service` 上传会话 / 完成上传 ->
  `conversation-service` profile update 串起来；头像展示会再通过 BFF 校验当前
  profile avatar URI 并向 `media-service` 换取短期 download URL；当前仍是本地 fake object HTTP adapter，
  不宣称真实 S3、缩略图、扫描或 CDN 已完成。
- 会话刷新会保留当前选中；gateway token 过期会清理本地 session / push / 会话展示状态。
- 本地发送失败的消息会保留在消息列表中，并提供显式重新编辑或作为新消息重发入口；
  客户端不会把失败缓存项静默标记为成功。
- `plan:browser-multiuser-ui-smoke` 已可从成功的 `loadtest/clientweb`
  `client-web-summary.json` 生成低敏浏览器 / PC 多用户 UI smoke 计划，覆盖直聊、
  群聊、群设置和稳定 selector，不保存密码、gateway token、push token 或
  refresh token；这仍是计划 / 契约，不宣称真实浏览器自动化已跑。
- `smoke:browser-multiuser-ui` 已提供显式 opt-in 的真实浏览器 / PC 多用户 UI runner：
  使用两个隔离 Chromium profile 通过 CDP 驱动 Web shell，覆盖登录、点击好友发起直聊、
  UI 建群、邀请成员、群聊发送、PullInbox 和 ACK；`loadtest/clientweb/run-local-smoke.ps1`
  可通过 `-RunBrowserMultiuserUISmoke` 调用。2026-06-23 已在 clean commit
  `8782936b` 跑通并归档报告；默认路径不启动浏览器。
- 已有 clean smoke 覆盖真实双用户好友直聊、群聊 first path、群资料 BFF
  read/update 和群成员动作链路；证据见 `docs/runbook/client-platform.md`。
- Windows desktop 已有 artifact / signing / installer plan first paths；签名 / installer
  工具已支持显式 local signing profile 输入，并有只读 release readiness report 汇总
  签名输入、低敏 `signtool` 候选提示、签名验证和 installer 阻塞；候选工具不会
  自动用于 readiness；timestamp URL 禁止携带账号密码、query 或 fragment；
  仓库已有 PFX 和 Windows cert-store 两个低敏 signing profile 示例，实际发布时需复制成
  untracked 本机 profile 并填入真实本机路径 / thumbprint；`init:desktop-signing-profile`
  是显式 copy helper，要求 `--source` 和 `--output`，不读取证书 / 密钥 / 密码；
  PFX 输入会做只读可读性 / signing key / 过期检查；Windows cert-store thumbprint
  会做只读本机证书 / signing key / 过期检查；profile 可声明预期公开 signer subject，
  valid signature 必须匹配该发布者策略；signing plan 可携带 CLI、
  `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 或 profile 中的公开 signer subject policy，但不宣称已验证；
  `verify:desktop-signature` 也可通过 CLI、
  `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 或 profile 读取公开 signer subject policy，
  但不会使用证书源签名或修改 artifact；signing executor 也可通过 CLI、
  `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 或 profile 读取公开 signer subject policy；
  其低敏 execution policy 会声明 profile 读取和 `--require-valid` 下的 signer subject enforcement；
  release readiness report 的 top-level 和 nested signing execution policy 也会声明 profile
  读取 / signer subject policy 检查，并输出低敏 executable / installer `signaturePolicy`
  摘要以表明公开 signer policy 是否配置和匹配；同时会对已收集的
  `desktop-installer` artifact 做独立 post-build 签名验证；install plan 也会在
  `desktop-installer` 未通过 read-only Authenticode 验证时 fail-closed，且 installer
  安装路径必须显式请求 `--artifact-kind desktop-installer`，并会在通过 CLI 或
  `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 传入公开 expected signer subject policy 时声明该检查，
  且 installer 签名摘要会输出低敏 `signaturePolicy`；`build:desktop-installer`
  执行后的 artifact 收集只读取选中 `bundle/<target>` 目录并要求
  `desktop-installer` kind，避免 standalone exe 混入 installer manifest；installer plan / builder
  的低敏 execution policy 也会声明 signing profile 读取、显式 expected signer subject policy 检查、artifact
  collection 和 manifest 写入；installer plan 的 signing summary 也会携带低敏
  `signaturePolicy`；当前不宣称已有生产签名 installer。
- Android 已有 WebView shell / bridge / APK 历史产物和 metadata smoke；后续切回时重新加载 toolchain env 或 Docker builder。

## 下一步优先级

1. Windows PC 端继续真实 signing input、valid signed artifact、MSI / NSIS installer 和签名 installer 体验。
2. 客户端产品能力继续补入群审批 / 禁言等更深群设置，以及
   media-service 真实 S3-compatible / thumbnail / scanner / CDN provider 后续链路。
3. Android 后续只在用户切回时继续 login-level WebView smoke、APK baseline 报告和真机 UI polish。
4. 客户端切片阶段性收口后，回到 workflow compensation adapter、instruction approval UI 和 ops 管理。
5. 新发现待办写入 `docs/runbook/remaining-goals.md`，不要把长待办复制回本文件。

## Focused Checks

客户端小切片优先跑：

```powershell
npm --prefix clients run check:no-toolchain
git diff --check; git diff --cached --check
```

跨服务、生成代码、migration、service-registry、Docker / compose、安全边界、
提交推送前或用户明确要求时，扩大到完整 `.\tools\check-local.ps1`。

## 硬边界

- 每个新客户端功能先做简短架构分析再编码。
- 客户端 local store 只做缓存 / 离线队列，不成为服务端事实源。
- 客户端不得直接调用内部微服务，不读取任何服务私表。
- PullInbox 是消息展示事实源；WebSocket 只做在线唤醒。
- 不引入隐藏 fallback；依赖、权限、事实源、投影或 provider 不确定时 fail-closed。
- TypeScript 负责三端共享客户端协议、同步核心和 UI；Rust / Kotlin 只做薄平台桥。
- Python 只做 AI worker / eval / 离线工具，不接管客户端主链路或业务事实源。
- 边界变化必须同步 README、service brief、相关 SDD / ADR、current-brief 和 remaining-goals。
- 不回滚用户已有修改。
