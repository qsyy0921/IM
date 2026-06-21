# NexusIM Platform Documents

本目录维护跨服务的平台能力文档，不写单个服务的内部实现。

长期完整架构以 `../architecture/target-architecture-complete.md` 为准；平台文档只把
中间件、数据平台、AI runtime、观测、安全和部署 profile 这些跨服务能力拆出来，避免
把它们散落到各服务 SDD 中。

| Document | Scope |
| --- | --- |
| `middleware-catalog.md` | 中间件能力分类、adoption rules、runtime profile 和替换 / 迁移登记模板。 |

维护规则：

- 服务内部设计继续放在 `docs/sdd/` 和 `docs/runbook/service-briefs/`。
- 新增中间件或替换中间件前，先在 `middleware-catalog.md` 登记 capability、owner、
  source-of-truth、runtime profile、健康检查、最小 smoke、迁移和安全边界。
- 中间件 adapter 只能落在对应服务 `internal/infrastructure/<adapter>/`；domain /
  app 层不能 import 具体中间件 client。
