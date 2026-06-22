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
  点击群聊进入会话、建群、群成员添加 / 退群第一路径、群设置信息条、消息列表、
  发送后本地状态刷新、PullInbox 和 ACK。
- 会话刷新会保留当前选中；gateway token 过期会清理本地 session / push /
  会话展示状态并提示重新登录。
- Web / PC shell 已补第一版会话展示标题、空态、常见错误中文文案和本地
  `clients/start-local-backend.ps1` / `clients/start-local-web.ps1` 启动入口；
  这些都是客户端展示 / 本地开发体验，不改变服务端事实源。
- 2026-06-23 clean committed smoke 已跑通真实双用户 client path：
  注册 -> 登录 -> 好友申请 / 接受 -> BFF 打开 direct 会话 -> direct 消息 notify /
  PullInbox / ACK；以及建群 -> receiver JOIN -> group 消息 notify / PullInbox / ACK。
  原始结果在 `H:\NexusIM\loadtest-results\client-web-bff-push-smoke-20260623-015417\client-web-summary.json`，
  记录 `commit=6a08fb14`、`git_dirty=false`。
- 本地调试默认使用 `127.0.0.1:8080/8088`；`clients/start-local-backend.ps1`
  显式启动本机 client backend，`clients/start-local-web.ps1` 显式启动 Web UI。
- Android 已有 WebView shell / bridge / APK 历史产物和 metadata smoke；当前 shell
  不宣称 F 盘 Android toolchain ready，后续切回 Android 时再重新加载 toolchain env。

## 下一步优先级

1. Windows PC 端继续 installer / 可运行包体验，并保持 Web / PC shell 的 UI 细节
   随真实调试反馈继续收口。
2. 若继续客户端产品能力，优先补群成员列表、角色变更 / owner transfer、移除成员和
   更完整群设置的 BFF 契约和 UI，而不是直接调用 conversation-service 私有接口。
3. Android 后续只在用户切回时继续：login-level WebView smoke、APK baseline
   报告和真机 UI polish。
4. 客户端切片阶段性收口后，再回到 workflow compensation adapter / instruction approval
   UI / ops 管理。
5. 新发现待办写入 `docs/runbook/remaining-goals.md`，不要把长待办复制回本文件。

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

- 每个新客户端功能先做简短架构分析再编码：确认是否属于 client-core、Web / PC
  shell、api-gateway BFF、push-gateway 或某个后端服务；确认数据事实源、API / 事件、
  权限、审计、失败语义、是否需要新技术 / 新中间件 / 新 provider，以及需要维护的文档。
- 如果新功能需要新增中间件，必须归入中间件平台并同步
  `docs/platform/middleware-catalog.md`；需要数据处理能力则归入数据平台；需要模型 /
  Agent 能力则归入 AI / Agent 平台；需要产品业务能力则归入对应业务 / 产品平台。
- 客户端 local store 只做缓存 / 离线队列，不成为服务端事实源。
- 客户端不得直接调用内部微服务，不读取任何服务私表。
- PullInbox 是消息展示事实源；WebSocket 只做在线唤醒。
- 不引入隐藏备用路径。客户端、BFF 或服务端遇到依赖、权限、事实源、投影或
  provider 不确定时，按 `docs/architecture/fail-closed-policy.md` fail-closed，
  使用显式 retry / repair，或重新读取对应事实源。
- 开发当前客户端链路时，遇到相关旧隐藏兜底 / fallback-like 分支要顺手删除；
  如果清理范围超过当前切片，必须把 owning service、文件范围和风险写入
  `docs/runbook/remaining-goals.md`，不能继续扩散。
- 新代码不得新增隐藏兜底；本地测试 adapter、compat window、repair / redrive
  必须显式命名、显式 profile、显式文档边界。
- TypeScript 负责三端共享客户端协议、同步核心和 UI；Rust / Kotlin 只做薄平台桥。
- Python 只做 AI worker / eval / 离线工具，不接管客户端主链路或业务事实源。
- 如果当前切片新增服务、服务能力、中间件或 runtime profile，必须同步根 README、
  对应 service brief、相关 SDD / ADR、`current-brief.md` 和 `remaining-goals.md`。
- 不回滚用户已有修改。
