# NexusIM Current Brief

低 token 阶段入口，只回答“现在在哪个阶段、下一步读哪里”。不要维护长历史、
完整证据或全部待办。

## 按需读取

- 当前执行目标：`docs/runbook/current-goal.md`
- 剩余目标：`docs/runbook/remaining-goals.md`
- 单服务事实：`docs/runbook/service-briefs/README.md`
- 客户端细节：`docs/runbook/client-platform.md`
- 完整目标架构：`docs/architecture/target-architecture-complete.md`
- Fail-closed 规则：`docs/architecture/fail-closed-policy.md`
- 可变推进策略和架构边界：`agent.md`
- 历史证据：按关键词查 `docs/runbook/loadtest/`、`archive/` 或 `history/`

## 当前阶段

```text
backend architecture + AI / Agent / RAG demo path
```

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端，并已扩展到 AI foundation
和 product-active 服务 first paths。浏览器端和 Windows PC 端已经收敛到可演示
IM MVP；Android、release signing、MSI / NSIS installer、完整移动端发布、复杂 UI
和深水区群管理全部后置。当前主线切回后端架构完善和 AI / Agent / RAG。

## 已有服务层级

- Core IM：api-gateway、identity-service、message-service、conversation-service、
  delivery-service、push-gateway、receipt-service、contacts-service、policy-service。
- AI foundation：search-service、memory-service、retrieval-gateway、rag-service /
  RAG、summary-service、agent-service、skill-registry、mcp-gateway、action-executor、
  ai-eval-service。
- Product-active first paths：admin-service、audit-service、control-plane-service、
  knowledge-ingestion-service、media-service、model-gateway、notification-service、
  presence-service、vector-index-service、workflow-service。
- Client platform：Web / Windows PC / Android 共用 TypeScript protocol 和
  client-core；native shell 只做薄 bridge。

## 当前短线

1. 客户端只作为演示入口继续保留：Web / Windows PC 已有登录、注册、好友 / 群聊列表、
   点击好友发起私聊、点击群聊进入会话、消息列表、发送、PullInbox、ACK、push
   状态和清晰失败提示；不继续追完整产品级客户端。
2. 已有 clean smoke 覆盖双用户好友直聊、群聊 first path、群资料 BFF
   read/update、群公告和群成员动作；当前群头像上传 / 展示已接 BFF -> media-service ->
   profile update / download URL 的本地 first path，头像更新会保留当前公告；详细证据见 `client-platform.md` 与
   `loadtest/clientweb` 报告。
3. `plan:browser-multiuser-ui-smoke` 已可从成功的 `client-web-summary.json` 生成
   低敏浏览器 / PC 多用户 UI smoke 计划；`smoke:browser-multiuser-ui` 已提供显式
   opt-in 的真实 Chromium CDP runner，覆盖登录、点击好友发起直聊、UI 建群、邀请成员、
   群聊发送、会话管理 controls、PullInbox 和 ACK。2026-06-23 已用
   `loadtest/clientweb/run-local-smoke.ps1 -RunBrowserMultiuserUISmoke` 跑通并归档报告；
   clean commit `8782936b` 覆盖 direct / group / invite 路径；clean commit
   `7e8a890b` 进一步覆盖 direct / group / invite + 会话标签 / 草稿 / 归档
   round-trip 的真实浏览器 / PC 多用户 UI smoke；clean commit `05b8aec6`
   已验证会话 tag / draft / archived-only 筛选的匹配和排除路径。2026-06-23
   `client-demo-mvp-browser-ui-20260623-231711` 追加验证 direct chat、group chat、
   group invite、conversation management 和 receiver ACK 全为 true。默认路径不启动浏览器。
   Web / PC shell 的登录过期可见状态清理也已纳入 focused client contract。
4. 2026-06-23 `client-demo-mvp-desktop-login-20260623-232819` 已通过 Windows desktop
   WebView 登录级真实 smoke：登录、push connected、direct conversation 外部消息触发、
   PullInbox、message observe、AckDelivery 和 `tauri-sqlite` native store readiness
   全部为 true。桌面 smoke 现在显式读取 `client-web-summary.json` 的
   `direct_chat.conversation_id`，避免在群成员管理后用已失效群会话触发 403。
5. Windows desktop 已有本地 artifact / signing / installer plan first paths；签名 /
   installer 工具已支持显式 local signing profile 输入，并有只读 release readiness
   report 汇总签名输入、低敏 `signtool` 候选提示、签名验证和 installer 阻塞；
   候选工具不会自动用于 readiness；timestamp URL 禁止携带账号密码、query 或
   fragment；仓库已有 PFX 和 Windows cert-store 两个低敏 signing profile 示例；
   `init:desktop-signing-profile` 可显式复制示例到 untracked 本机 profile，且不读取
   证书 / 密钥 / 密码；
   PFX 输入会做只读可读性 / signing key / 过期检查；Windows cert-store
   thumbprint 会做只读本机证书 / signing key / 过期检查；profile 可声明预期公开
   signer subject，valid signature 必须匹配该发布者策略；signing plan 可携带 CLI、
   `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 或 profile 中的公开 signer subject policy，但不宣称已验证；
   read-only verifier 也可通过
   CLI、`NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT` 或 profile 读取公开 signer subject policy
   但不会使用证书源签名；signing executor 也可读取同一公开策略；其低敏
   execution policy 会声明 profile 读取和 `--require-valid` 下的 signer subject enforcement；
   release readiness report 的 top-level 和 nested signing execution policy 也会声明
   profile 读取 / signer subject policy 检查，并输出低敏 executable / installer
   `signaturePolicy` 摘要以表明公开 signer policy 是否配置和匹配；同时会对已收集的
   `desktop-installer` artifact 做独立 post-build 签名验证；
   install plan 会在通过 CLI 或 `NEXUSIM_DESKTOP_SIGN_EXPECTED_SUBJECT`
   传入公开 expected signer subject policy 时声明该检查，且 installer 签名摘要会输出低敏
   `signaturePolicy`；
   `build:desktop-installer` 执行后的收集步骤只读取选中 `bundle/<target>` 目录并要求
   `desktop-installer` artifact kind；installer plan / builder 的低敏 execution policy
   也会声明 signing profile 读取、显式 expected signer subject policy 检查、artifact collection 和
   manifest 写入；installer plan 的 signing summary 也会携带低敏 `signaturePolicy`；
   `smoke:desktop-local-signing` 已补显式本地开发签名 smoke，签名临时 artifact
   副本、临时信任 CurrentUser code-signing certificate、验证 Authenticode `Valid` 并清理；
   2026-06-23 本机运行已通过，原 collected artifact 未变。这些能力保留为后置
   release backlog，不作为当前主线。
6. 所有客户端能力只走 api-gateway BFF 和 push-gateway，不直连内部服务。
7. 新功能先做简短架构分析再编码；新增服务 / 中间件 / provider 必须归属正确平台层并同步 owner docs。
8. 不引入隐藏 fallback；开发相关路径时清理旧 fallback-like 分支，无法本轮清理的写入
   `remaining-goals.md`。

## 当前 AI / Agent 方向

当前 active slice 已切到：

```text
backend architecture + AI / Agent / RAG demo path
```

目标演示链路是：

```text
IM 消息 -> search / memory projection -> EvidencePack -> RAG / Agent answer -> approval / audit
```

2026-06-23 当前低敏 collaborative-memory eval 已扩到 73 个 catalog cases /
20 个 profile-Agent safety fixture cases，并已完成 memory-service /
retrieval-gateway optional live adapters 的第一轮接入；RAG / Summary / Agent
live adapters 也会断言 multi-hop actor/source-chain completeness。2026-06-24
`ai-eval-service-stack-live-20260624-collab-memory-v4` 先通过真实
service-stack gate：8 adapters、51 cases、47 passed、0 failed、4 skipped。
随后 `ai-eval-service-stack-live-20260624-retrieval-negative` 补上
retrieval-gateway negative / miss adapter，完整 service-stack gate 达到
9 adapters、51 cases、51 passed、0 failed、0 skipped。当前检索边界已覆盖
empty memory source coverage、superseded memory 排除、source refs / dedupe
reason 和 cross-tenant evidence isolation。

## 不变量

- PullInbox 是消息展示事实源，WebSocket 只是在线唤醒。
- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做候选算法和 eval；Go 拥有控制面、状态和审计。
