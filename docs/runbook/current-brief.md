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
reason 和 cross-tenant evidence isolation。2026-06-24 EvidencePack memory
graph edge 扩展已落地：retrieval-gateway 通过 memory-service 公开
`GetMemoryEvent` 读取 current memory graph edges，并把
`EvidenceMemoryGraphEdge` 透传给 RAG / Agent；`loadtest/retrieval`、
`loadtest/rag`、`loadtest/agent` 会断言跨群 source refs 与 `SUPPORTS`
graph edge 被保留，lookup 失败 fail-closed。
2026-06-24 profile aggregate evidence 也已进入 EvidencePack：retrieval-gateway
通过 memory-service 公开 `ListProfileAggregates` 查询当前用户 ACTIVE profile
aggregate，并以 `PROFILE_AGGREGATE` evidence 暴露给 RAG / Summary / Agent；
downstream 只消费 EvidencePack，不直连 memory 私表。`loadtest/retrieval`、
`loadtest/rag`、`loadtest/agent` 会断言 profile subject、aggregate type/key、
supporting memory ids 和 source coverage 被保留，profile lookup 失败 fail-closed。
同日 memory-service 补了公开 `RecomputeProfileAggregate` first path：profile
aggregate 由多个当前用户可见的 ACTIVE / APPROVED `PROFILE_SIGNAL` memory events
重算，支持数量不足时归档既有 active / pending profile；`loadtest/memory` 不再
手工写 active profile，而是通过该 RPC 验证 multi-source profile evidence。
ai-eval memory-service live adapter 已把该行为登记为
`must_recompute_profile_via_public_api` 断言。
`loadtest/memoryprofile` 已提供 first-stage profile repair operator：默认 plan-only，
显式 `--execute` 才调用公开 recompute RPC，输出低敏 hash / count 报告。
同日该 operator 已补 profile repair batch approval path：batch manifest 默认只生成
低敏 plan；批量执行必须通过 workflow-service 公开 `GetWorkflow` 校验
`REPAIR_APPROVAL/APPROVED`，并要求 workflow target / payload hash 与本次 batch plan
匹配；显式 `--request-approval` 只创建低敏 repair approval workflow，不执行 repair。
memory-service timeline worker 已升级 `rules-v0.2` group memory extraction：只有明确
memory cue 或显式 memory metadata 的群消息会投影成 PENDING StructuredMemoryEvent；
普通聊天不生成 memory fact；profile / preference / role signal 保持 NEEDS_REVIEW，
避免单条群聊事实直接升级成个人画像。
`loadtest/ragagent` 已提供 RAG-Agent demo first path：复用 `loadtest/rag` 和
`loadtest/agent`，围绕同一 tenant / conversation 生成低敏总报告，断言 RAG grounded
answer、Agent proposal、approval、action-executor audit、EvidencePack graph edges
和 profile aggregate evidence 均成立；不保存 raw answer / proposal text。2026-06-24
`ai-eval-rag-agent-demo-live-20260624-current-image-fixed` 已通过真实 service-stack
gate：4 adapters、27 cases、27 passed、0 failed、0 skipped；`rag-agent-demo`
已接入 ai-eval optional service-stack adapter、gate policy 和 service-stack 路由。
2026-06-24 `ai-eval-rag-agent-demo-live-20260624-public-candidate-review-v3`
进一步通过真实 service-stack gate：4 adapters、27 cases、27 passed、0 failed、
0 skipped；该 run 证明 memory-service 公开 candidate review / approval path 产生的
`ACTIVE + APPROVED` memory 会进入 RAG / Agent EvidencePack。
2026-06-24 `ai-eval-rag-agent-demo-live-20260624-temporal-update-v2` 再次通过真实
service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped；
该 run 证明 public candidate replacement 经公开审批后会 supersede 旧 memory，
旧 memory 进入 `SUPERSEDED` 并被当前 EvidencePack 排除，RAG / Agent 只消费当前
`ACTIVE + APPROVED` replacement。
同日 `loadtest/ragagent` 已补 profile repair approval 组合断言：先用公开
candidate review path 写入 `PROFILE_SIGNAL`，再通过 `loadtest/memoryprofile`
请求 `REPAIR_APPROVAL` workflow，workflow-service 审批后才执行 batch recompute；
组合报告会断言修复后的 profile aggregate 同时被 RAG / Agent EvidencePack 消费。
2026-06-24 `ai-eval-rag-agent-demo-live-20260624-profile-repair-approval-v3`
已通过真实 service-stack gate：4 adapters、27 cases、27 passed、0 failed、
0 skipped；该 run 确认 profile repair 先经 workflow-service `REPAIR_APPROVAL`
审批，再通过 memory-service 公开 `RecomputeProfileAggregate` 执行 batch recompute，
修复后的 profile aggregate 同时进入 RAG / Agent EvidencePack。期间修复了
memory-service 在既有 non-deterministic `profile_id` 但 subject/type/key 相同的
profile aggregate 上重算时可能触发唯一约束的问题，并以真实 PostgreSQL 集成测试覆盖。
同日 `loadtest/ragagent` 已接入 profile repair 负向门禁：未审批 workflow 不能执行
batch recompute，已审批 workflow 的 payload hash 与当前 batch manifest 不匹配时
必须 fail-closed；`run-ai-eval-ragagent-adapter.ps1` 已把该负向门禁纳入 summary
必检断言。
同日 `ai-eval-rag-agent-demo-live-20260624-profile-repair-negative-v1` 已通过真实
service-stack gate：4 adapters、27 cases、27 passed、0 failed、0 skipped；该 run
确认未审批 workflow 和 approval payload hash mismatch 均 fail-closed，之后匹配的
已审批 workflow 才会执行 memory-service public recompute，并把修复后的 profile
aggregate 放入 RAG / Agent EvidencePack。
同日 `ai-eval-rag-agent-demo-live-20260624-group-memory-answer-proposal-gate-v1`
已通过真实 service-stack gate：4 adapters、27 cases、27 passed、0 failed、
0 skipped；该 run 确认 `loadtest/ragagent` 会通过 memory-service 公开
candidate review path 构造 `DECISION` / `BLOCKER` / `FILE` 三类 group memory，
并要求 RAG answer 和 Agent proposal 均保留 3 条 memory evidence、6 个 source refs
和 3 个 cross-group source refs。
同日 Python AI Worker 已补 memory extraction candidate first path：
`ai/python/nexusim_ai_memory` 只从显式 low-sensitive message batch 的
`decision:` / `task:` / `status:` / `blocker:` / `file:` / `profile_signal:`
cue 生成 hash-only candidates；普通聊天不抽取；profile signal 必须 review。
该能力不写 memory fact、不返回 raw text。2026-06-24 Go-side memory extraction
candidate adapter / ai-eval 接入已补齐：`internal/ai/memorycandidate` 负责调用
Python batch CLI 并校验低敏 request / result，`tools/memory-extraction-go-adapter-smoke`
和 `run-ai-eval-memory-extraction-candidate-adapter.ps1` 覆盖 explicit cue hash-only、
ordinary-chat zero candidates、profile review required 和 unsafe input fail-closed。
同日 memory-service 公开 candidate review / approval / persistence path 已补齐：
`SubmitMemoryCandidate` 会校验 source refs 可见、fact text 与 `fact_sha256` 匹配，
并只写入 `PENDING + NEEDS_REVIEW`；`ReviewMemoryCandidate` 才能把 candidate
显式推进为 `ACTIVE + APPROVED` 或 `REJECTED`。`loadtest/memory` 和 memory-service
ai-eval adapter 已新增 public candidate review 检查。
同日 action-executor 的 Agent action boundary cases 已补一轮 preflight safety
覆盖：`action-preflight-safety` smoke / eval catalog 从 11 个扩到 14 个 case，
新增 approval id、prepared audit id、resource id 与已批准 proposal 绑定不一致时的
`PROPOSAL_MISMATCH`；这些 case 都要求在 approved-proposal verification 阶段
fail-closed，不写 execution audit、不写 tool result projection、不调用 tool executor。

## 不变量

- PullInbox 是消息展示事实源，WebSocket 只是在线唤醒。
- RAG / summary / Agent 只能消费权限过滤后的 EvidencePack。
- 真实写动作必须走 policy、proposal / approval、executor 和 audit。
- Python AI Worker 只做候选算法和 eval；Go 拥有控制面、状态和审计。

## 下一步

- public candidate review、temporal update 和 profile repair approval 已进入
  RAG-Agent 真实 service-stack gate 归档。
- 下一步继续深化 RAG-Agent demo 的 EvidencePack / approval / audit 展示：
  EvidencePack source-chain coverage 和真实业务 proposal 场景。
