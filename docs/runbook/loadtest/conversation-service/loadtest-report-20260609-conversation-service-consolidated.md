# conversation-service 阶段总报告

## 阶段定位

`conversation-service` 当前阶段只完成第一条 read path：

```text
GetSendContext
-> conversations / conversation_members
-> 返回发送上下文
-> message-service SendMessage 使用真实 gRPC 依赖
```

它不是完整的会话服务。成员变更命令、成员变更 Saga、群主/管理员规则、ACL 投影和成员边界 timeline event 后续单独实现。

## 当前能力

| 能力 | 状态 |
| --- | --- |
| 独立六层 DDD 目录 | 已完成 |
| gRPC proto | 已完成，`api/proto/nexusim/conversation/v1/conversation_service.proto` |
| PostgreSQL migration | 已完成，`migrations/postgres/conversation/000001_conversation_core.sql` |
| `GetSendContext` app/domain/repository | 已完成 |
| gRPC handler | 已完成 |
| message-service gRPC client | 已完成，可通过 `NEXUSIM_CONVERSATION_SERVICE_ADDR` 启用 |
| 真实 PostgreSQL repository 集成测试 | 已通过 |
| message-service -> conversation-service smoke | 已通过 |

## 核心结论

第一轮 smoke 证明：

- `message-service` 不再只能依赖 conversation strict mock。
- 会话发送上下文已经由真实 `conversation-service` 读取。
- `message-service` 仍保持边界：只读 conversation context，不写成员事实。
- 当前链路足以支撑后续继续开发 `delivery-service` / `push-gateway`，不需要继续在 `message-service` 上做大规模压测。

## 验证记录

| 报告 | 结论 |
| --- | --- |
| `loadtest-report-20260609-send-context-smoke.md` | 725 / 725 成功，p99 13.26ms，跨服务 read path 打通 |

## 面试讲法

可以这样讲：

> 第一阶段我先把 `message-service` 的 SendMessage 主链路做完整。后面没有继续只刷压测数字，而是把 mock 依赖拆出来，落了第二个真实微服务 `conversation-service`。它拥有 conversations 和 conversation_members 两张核心事实表，提供 `GetSendContext` gRPC 接口。message-service 通过端口读取 member_version、permission_version、conversation_mode 和 fanout_mode，再进入本地事务写消息。这样服务边界更真实，也能解释为什么 message-service 不能直接写成员事实。

## 下一步

1. 给 `conversation-service` 增加本地运行 runbook。
2. 补 `GetSendContext` 的更多错误路径测试：会话 archived/deleted、成员 left/banned、不存在成员。
3. 冻结 `conversation-service-member-change-saga.md`，再实现成员变更命令。
4. 进入 `delivery-service` 或 `push-gateway` 前，先明确它们的 SDD 和最小可运行链路。
