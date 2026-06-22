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

- 该 active slice 继续有效；当前优先浏览器端和 Windows PC 端，Android 登录级
  smoke 后置。
- Web / PC / Android 共用 `clients/packages/protocol` 和
  `clients/packages/client-core`；客户端只连 `api-gateway` BFF 和 `push-gateway`。
- Web / PC shell 已接账号密码登录、注册、好友工作台、点击好友打开私聊、
  点击群聊进入会话、建群、消息列表、发送后本地状态刷新、PullInbox 和 ACK。
- 会话刷新会保留当前选中；gateway token 过期会清理本地 session / push /
  会话展示状态并提示重新登录。
- 本地调试默认使用 `127.0.0.1:8080/8088`；`loadtest/clientweb/run-local-dev.ps1`
  负责留住本机 client backend。
- Android 已有 WebView shell / bridge / APK 历史产物和 metadata smoke；当前 shell
  不宣称 F 盘 Android toolchain ready，后续切回 Android 时再重新加载 toolchain env。

## 下一步优先级

1. 补真实双用户客户端 smoke，验证好友直聊和群聊 first path。
2. 收口 Web / Windows PC shell 的剩余体验：会话标题、空态、错误文案和启动脚本。
3. Windows PC 端继续 installer / 启动脚本 / 可运行包体验。
4. Android 后续只在用户切回时继续：login-level WebView smoke、APK baseline
   报告和真机 UI polish。
5. 客户端切片阶段性收口后，再回到 workflow compensation adapter / instruction approval
   UI / ops 管理。
6. 新发现待办写入 `docs/runbook/remaining-goals.md`，不要把长待办复制回本文件。

## Focused Checks

客户端小切片优先跑相关脚本，避免频繁完整门禁：

```powershell
npm --prefix clients run check:no-toolchain
git diff --check; git diff --cached --check
```

详细 no-toolchain、artifact、Android 和 desktop dry-run 执行策略见
`docs/runbook/client-platform.md` 与 `clients/README.md`，不要复制回本文件。

只有跨服务、生成代码、migration、service-registry、Docker / compose、安全边界、
提交推送前或用户明确要求时，才扩大到完整 `.\tools\check-local.ps1`。

## 硬边界

- 客户端 local store 只做缓存 / 离线队列，不成为服务端事实源。
- 客户端不得直接调用内部微服务，不读取任何服务私表。
- PullInbox 是消息展示事实源；WebSocket 只做在线唤醒。
- 不引入隐藏备用路径。客户端、BFF 或服务端遇到依赖、权限、事实源、投影或
  provider 不确定时，按 `docs/architecture/fail-closed-policy.md` fail-closed，
  使用显式 retry / repair，或重新读取对应事实源。
- TypeScript 负责三端共享客户端协议、同步核心和 UI；Rust / Kotlin 只做薄平台桥。
- Python 只做 AI worker / eval / 离线工具，不接管客户端主链路或业务事实源。
- 不回滚用户已有修改。
