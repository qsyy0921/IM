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
client platform MVP foundation
```

NexusIM 已有本地 / 双机可运行的最小分布式 IM 后端，并已扩展到 AI foundation
和 product-active 服务 first paths。当前短线优先浏览器端和 Windows PC 端；
Android 后置到用户明确切回。

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

1. Web / Windows PC 客户端继续收口：登录、注册、好友申请、好友私聊、群聊、
   群成员管理、权限感知群设置、群资料、消息列表、发送、PullInbox 和 ACK。
2. 已有 clean smoke 覆盖双用户好友直聊、群聊 first path、群资料 BFF
   read/update 和群成员动作；详细证据见 `client-platform.md` 与
   `loadtest/clientweb` 报告。
3. Windows desktop 已有本地 artifact / signing / installer plan first paths；签名 /
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
   传入公开 expected signer subject policy 时声明该检查；
   `build:desktop-installer` 执行后的收集步骤只读取选中 `bundle/<target>` 目录并要求
   `desktop-installer` artifact kind；installer plan / builder 的低敏 execution policy
   也会声明 signing profile 读取、显式 expected signer subject policy 检查、artifact collection 和
   manifest 写入；installer plan 的 signing summary 也会携带低敏 `signaturePolicy`；
   下一步仍是真实证书输入、
   valid signed artifact 和 MSI / NSIS installer 体验。
4. 所有客户端能力只走 api-gateway BFF 和 push-gateway，不直连内部服务。
5. 新功能先做简短架构分析再编码；新增服务 / 中间件 / provider 必须归属正确平台层并同步 owner docs。
6. 不引入隐藏 fallback；开发相关路径时清理旧 fallback-like 分支，无法本轮清理的写入
   `remaining-goals.md`。

## 不变量

- PullInbox 是消息展示事实源，WebSocket 只是在线唤醒。
- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做候选算法和 eval；Go 拥有控制面、状态和审计。
