# NexusIM Current Brief

本文件是低 token 阶段入口，只回答“现在在哪个阶段、下一步读哪里”。不要在这里维护
长历史、完整证据或全部待办。

## 按需读取

- 当前执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 单服务事实：`docs/runbook/service-briefs/README.md`，再读对应 service brief。
- 客户端细节：`docs/runbook/client-platform.md`
- 完整目标架构：`docs/architecture/target-architecture-complete.md`
- Fail-closed 规则：`docs/architecture/fail-closed-policy.md`
- 可变推进策略和架构边界：先看 `agent.md` 的 owner table，再读对应 owner document。
- 新功能开发流程：`agent.md` 的 `Feature Development Protocol`。
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`docs/runbook/archive/`
  或 `docs/runbook/history/`。

## 当前阶段

```text
client platform MVP foundation
```

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端，并已扩展到 AI foundation
和 product-active 服务 first paths。当前用户已切入客户端平台，短线优先浏览器端
和 Windows PC 端；Android 后置到用户明确切回。

## 已有服务层级

- Core IM services：api-gateway、identity-service、message-service、
  conversation-service、delivery-service、push-gateway、receipt-service、
  contacts-service、policy-service。
- AI foundation：search-service、memory-service、retrieval-gateway、rag-service、
  summary-service、agent-service、skill-registry、mcp-gateway、action-executor、
  ai-eval-service。
- Product-active first paths：admin-service、audit-service、control-plane-service、
  knowledge-ingestion-service、media-service、model-gateway、notification-service、
  presence-service、vector-index-service、workflow-service。
- Client platform：Web / Windows PC / Android 共用 TypeScript protocol 和
  client-core；native shell 只做薄平台 bridge。

## 当前短线

1. 收口 Web / Windows PC 客户端：账号登录 / 注册、好友列表、好友申请、点击好友发起
   私聊、群聊列表、建群、点击群聊进入会话、群成员添加 / 退群、成员列表、移除成员、
   角色变更 / owner transfer 第一路径、消息列表、发送后本地状态刷新、PullInbox /
   ACK 和本机可运行包体验。2026-06-23 的 clean smoke 已验证双用户好友直聊和群聊
   first path；Web / PC shell 已补第一版会话展示标题、空态、常见错误中文文案、
   显式本地启动脚本和群设置操作区。Windows desktop collected package 已补
   package-local README / launcher support files，并已有 unsigned local portable
   zip bundle 工具；artifact manifest 已区分 desktop executable / installer，
   portable bundle 只接受 executable；`plan:desktop-installer` / `plan:desktop-signing` 已能检查
   Tauri installer 和显式签名输入 readiness；`sign:desktop-artifact` 已补为显式
   `--execute` 门控的签名执行入口，默认仍是低敏 plan-only，release signing 可加
   `--require-valid` 在签名后立即 fail-closed 验证；`verify:desktop-signature`
   已补为只读 Authenticode 验证入口；2026-06-23 已重新 collect 新格式
   `desktop-executable` artifact，manifest 为
   `clients/artifacts/2026-06-22T214826Z/manifest.json`，签名状态为 `NotSigned`；
   installer 已有独立仓库
   profile，不再要求打开默认开发 config 的 `bundle.active`；`build:desktop-installer` 已提供显式
   `--execute` 门控的 installer build 包装器，并用该 profile 调用 Tauri。desktop
   signing、signing execution、signature verification 和 installer planner 会按
   `artifactKind` 选择 artifact，默认 executable，installer 需要显式请求；generic install plan 已按 `artifactKind` 区分 executable / installer，缺 kind 的
   旧 manifest 要重新 collect，installer 不进入 portable launcher 或 direct shell-smoke；
   installer planner 也只接受显式 `desktop-executable` 作为 MSI / NSIS build baseline，
   旧 manifest 或 `desktop-installer` 不会被当作 installer build 输入；
   当前本机 dry-run 已找到新 executable baseline，但缺真实 signing input 和 valid
   signature，因此不执行 bundling 或签名；下一步继续真实 signing input、valid signed artifact 和 MSI / NSIS installer 体验。
   `loadtest/clientweb` 已扩展到群成员列表、
   角色变更、owner transfer 和移除成员的 BFF 链路；2026-06-23 clean committed
   smoke 已通过。
2. 所有客户端能力只走 api-gateway BFF 和 push-gateway，不直连内部服务。
3. 新功能先做简短架构分析，再编码；必须判断是否需要新技术、新中间件、新 provider
   或新服务，并把新增内容归入正确平台层。边界变化要同步 README、middleware catalog、
   service brief、SDD / ADR、current-goal / current-brief / remaining-goals 等 owner docs。
4. 不引入隐藏备用路径；依赖、权限、事实源或投影不确定时 fail-closed，并使用显式
   repair / retry 或重新读取对应事实源。开发相关路径时持续删除旧隐藏兜底 /
   fallback-like 分支，新发现但无法本轮清掉的项写入 `remaining-goals.md`。

## 不变量

- PullInbox 是消息展示事实源，WebSocket 只是在线唤醒。
- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做候选算法和 eval；Go 拥有控制面、状态和审计。
- 新发现待完成工作写入 `docs/runbook/remaining-goals.md`。
