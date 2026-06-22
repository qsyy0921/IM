# NexusIM Fail-Closed Policy

NexusIM 不允许隐藏业务兜底。依赖、权限、事实源、投影或 provider 状态不确定时，
默认行为必须是 fail-closed、显式 retry / repair/redrive、或返回稳定错误。不能用
一个看似可用的备用分支继续推进业务事实。

本文只约束当前和未来代码。历史 loadtest 报告、旧架构草案和归档文档可以保留当时
的原文，但不能作为新实现的依据。

## 1. 禁止的做法

- 权限 / policy / ReBAC / ownership check 失败后改走 allow 分支。
- identity、token、trusted metadata、MFA、OIDC / JWKS 不可用时改走本地身份。
- conversation、membership、delivery、receipt、search、memory 投影缺失时伪造事实。
- provider、Kafka、Redis、PostgreSQL、object storage 不可用时静默吞错并宣称成功。
- Agent / MCP / tool action 在 policy、approval、audit 缺失时继续执行。
- 为了让 smoke 通过，在生产路径里写 mock / noop / fake / compatibility 分支。
- 新增生产代码标识符、注释或文档时继续使用模糊备用路径表达业务降级。

## 2. 允许但必须显式命名的形态

这些能力必须用精确的名字和边界：

| 形态 | 允许条件 | 推荐命名 |
| --- | --- | --- |
| 本地测试 adapter | 只在 local smoke / test profile 启用，公网或生产 profile 启动失败 | `local-test`, `test-adapter`, `dev-profile` |
| 空实现 runtime | 只用于服务 skeleton / command health，不处理真实业务请求 | `disabled`, `dev-noop` |
| 兼容入口 | 显式 opt-in，有 deadline、metrics 和移除计划 | `compat`, `migration-window` |
| 在线体验退化 | 不推进 durable 事实，只提示客户端重新拉取事实源 | `pull-from-source`, `degraded-online` |
| 配置默认值 | 只表达 env / flag 默认值，不影响业务事实 | `defaultValue` |
| repair / redrive | 需要 operator、审批、audit 和幂等保护 | `repair`, `redrive` |

## 3. 服务实现规则

1. App / domain 层不能依赖具体 mock / noop / fake provider。
2. Infrastructure adapter 可以有 local-test 实现，但 cmd wiring 必须要求显式 mode。
3. Public listener 使用 local-test auth 时必须限制 loopback / RFC1918 私网，否则启动失败。
4. 数据新鲜度、projection version、membership window、permission version 不确定时必须
   fail-closed。
5. 事件 consumer 对 unknown / malformed event 必须 retry、DLQ 或阻塞；不能提交 offset
   后丢弃业务事实。
6. 客户端离线缓存不是事实源；恢复路径必须回到 `PullInbox`、公开 API 或对应服务事实。
7. AI / Agent 只消费 EvidencePack / public API；缺 evidence、policy 或 approval 时拒绝。
8. 新增模糊备用路径、`mock`、`noop`、`fake`、`legacy`、`best-effort` 等词必须先说明为什么
   不是隐藏业务语义。默认不允许新增。

## 4. 文档规则

- 当前架构、SDD、runbook 和 prompt 文档应使用 fail-closed、explicit retry、compat window、
  local-test adapter 等精确术语。
- 不再把备用路径写成正向设计目标。
- 历史报告可以保留原文，但新报告应写清：在线通知可退化，可靠恢复来自 durable
  `PullInbox / AckDelivery`，不是在线兜底成功。
- 如果发现新需求需要备用路径，先写 ADR 或 SDD 小节，证明它不绕过权限、事实源、审计
  和 repair。

## 5. Gate

`tools/check-fail-closed-policy.ps1` 是一个 diff gate：它检查当前未提交 / staged diff
新增行，不允许在普通代码和文档里引入新的隐藏备用路径词汇。只有本 policy、agent /
prompt 入口和明确治理文档可以讨论这些边界。

完整门禁会调用该脚本；小切片也可以单独运行：

```powershell
.\tools\check-fail-closed-policy.ps1
```
