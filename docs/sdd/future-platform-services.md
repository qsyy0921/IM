# Future Platform / Product Services SDD v0.1 Draft

本文冻结下一组 future 服务的组合边界和 promotion 顺序。它不是 10 个服务的一次性实现计划；每个服务仍需要自己的 SDD、proto、migration、六层 skeleton、runtime、observability 和 focused smoke。

## 目标

把已登记为 `future` 的平台 / 产品化服务推进到可逐个实现的状态：

```text
media-service
notification-service
audit-service
admin-service
control-plane-service
presence-service
model-gateway
workflow-service
knowledge-ingestion-service
vector-index-service
```

核心原则：

- 服务数量不写死，stage switch 必须由真实边界驱动。
- 不一次性创建全部 `services/<name>` 目录。
- 每个服务 promotion 前必须证明独立数据模型、独立伸缩需求、独立故障边界、独立安全边界，或能显著降低现有服务复杂度。
- 业务事实仍由现有 owner 服务拥有；future 服务不能直接读取其它服务私有表。

## 服务分组

| 分组 | 服务 | 作用 |
| --- | --- | --- |
| IM 产品补全 | `media-service`、`presence-service` | 媒体资产、在线状态、输入中、设备在线 |
| 通知与审计 | `notification-service`、`audit-service` | email/SMS/APNs/FCM/provider delivery，统一审计和 hash-chain |
| 运维控制面 | `admin-service`、`control-plane-service`、`workflow-service` | 管理 API、配置/灰度/quota、审批和长事务 |
| AI / 知识基础设施 | `model-gateway`、`knowledge-ingestion-service`、`vector-index-service` | 模型 provider、知识导入、向量写入和重建 |

## 推荐 promotion 顺序

第一组先做产品边界清晰、依赖少的服务：

```text
media-service -> notification-service -> audit-service
```

第二组做运营控制面：

```text
control-plane-service -> admin-service -> presence-service
```

第三组做 AI provider / knowledge pipeline：

```text
model-gateway -> knowledge-ingestion-service -> workflow-service -> vector-index-service
```

说明：

- `media-service` 先做，因为 message-service 已有媒体引用 payload，但不应承担二进制、缩略图、病毒扫描和转码。
- `notification-service` 可承接 identity challenge delivery，但第一版不能把现有 webhook / SMTP 写成生产级通知平台。
- `audit-service` 可统一归档 identity / policy / agent / action 的低敏审计，但不能替代本地事务内 audit。
- `control-plane-service` 优先于 `admin-service` 的生产化配置能力；admin 只调用公开 operator / control-plane API。
- `presence-service` 不能替代 push-gateway route，也不能成为 delivery / ACK 事实源。
- `model-gateway` 统一 provider 调用，但不拥有 EvidencePack、prompt truth 或 Agent approval。
- `knowledge-ingestion-service` 和 `vector-index-service` 必须服从 retrieval / policy / tombstone / delete-proof 边界。
- `workflow-service` 承载长事务和审批等待，不进入 IM 热路径。

## Promotion Checklist

某个 future 服务从 `future` 切到 active 前，必须完成：

1. `docs/sdd/<service>.md` SDD v0.1。
2. service brief 从 future/draft 改为 active 状态。
3. `docs/runbook/service-registry.json` stage switch。
4. `api/proto/...` 或明确说明第一版无需同步 API。
5. PostgreSQL migration 或明确说明第一版无持久化。
6. `services/<service>` 六层 skeleton。
7. `cmd/<service>` runtime，包含 `/healthz`、`/readyz`、`/debug/metrics` 和 `/metrics`。
8. Docker runtime、local compose、Prometheus rule、Grafana dashboard。
9. focused tests：domain/app/api/repository 按风险覆盖。
10. focused smoke 或明确的 no-runtime contract test。

涉及 proto、migration、service-registry、Docker/compose 或安全边界时，必须扩大门禁；普通文档切片只跑 runbook / future-service boundary / diff check。

## 全局禁止事项

- 不把媒体二进制塞回 message-service。
- 不让 notification-service 保存验证码、token 或 provider response body 明文。
- 不让 audit-service 成为业务状态源。
- 不让 admin-service、control-plane-service、workflow-service 直接改其它服务私有表。
- 不让 presence-service 决定消息投递、ACK 或权限事实。
- 不让 model-gateway 持久化 raw prompt / model output，除非有低敏审计契约。
- 不让 knowledge-ingestion / vector-index 绕过 retrieval-gateway、policy-service、EvidencePack、tombstone 或 delete-proof。

## 与当前主线的关系

当前 9 个 IM 服务和 AI foundation 服务仍作为可运行底座。future 服务 promotion 是产品化 / 平台化推进，不应回滚现有 AI / RAG / Agent 边界。每个新服务必须先小切片闭环，再进入下一个服务。
