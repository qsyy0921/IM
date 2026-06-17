# NexusIM Local Observability

本文件只记录本地开发 / smoke 用的第一阶段观测入口。它不是生产 Prometheus、Tempo、Jaeger、Loki、Alertmanager 或 SIEM 部署手册。

## Debug Metrics / Prometheus Text

各服务的 `/debug/metrics` 仍是本地 JSON 快照入口。当前 9 个服务额外提供第一阶段 Prometheus text endpoint：

```text
GET http://<NEXUSIM_API_GATEWAY_DEBUG_ADDR>/metrics
GET http://<NEXUSIM_IDENTITY_DEBUG_ADDR>/metrics
GET http://<NEXUSIM_MESSAGE_DEBUG_ADDR>/metrics
GET http://<NEXUSIM_CONVERSATION_DEBUG_ADDR>/metrics
GET http://<NEXUSIM_DELIVERY_DEBUG_ADDR>/metrics
GET http://<NEXUSIM_PUSH_DEBUG_ADDR>/metrics
GET http://<NEXUSIM_RECEIPT_DEBUG_ADDR>/metrics
GET http://<NEXUSIM_CONTACTS_DEBUG_ADDR>/metrics
GET http://<NEXUSIM_POLICY_DEBUG_ADDR>/metrics
```

`/metrics` 复用 `/debug/metrics` 的低敏 snapshot。api-gateway 当前覆盖 gRPC request / error / latency、facade / legacy descriptor / other request exposure、legacy descriptor last-seen、auth JWK、rate-limit、runtime 和 OTel trace config 聚合指标；identity-service 当前覆盖 gRPC request / error / latency、identity 风险聚合、challenge delivery outbox、challenge delivery debug、worker retry 和 OTel trace config 聚合指标；message-service 当前覆盖 SendMessage / repository / Kafka / outbox relay fixed-operation latency、batch shape、PG pool、outbox relay retry 和 OTel trace config 聚合指标；conversation-service 当前覆盖 gRPC request / error / latency、conversation / member / member-change saga 聚合、member-change worker retry、PG pool 和 OTel trace config 聚合指标；delivery-service 当前覆盖 gRPC request / error / latency、durable inbox read model、membership projection、delivery outbox、projection failure blocker、timeline worker、outbox relay、PG pool 和 OTel trace config 聚合指标；push-gateway 当前覆盖 WebSocket session、slow eviction、in-memory / Redis resume、Redis route / subscriber、delivery / identity consumer worker、auth JWKS 和 OTel trace config 聚合指标；receipt-service 当前覆盖 gRPC request / error / latency、receipt projection、conversation summary、receipt outbox、delivery projection worker、outbox relay、PG pool 和 OTel trace config 聚合指标；contacts-service 当前覆盖 gRPC request / error / latency、contact request / edge 聚合、contacts outbox、outbox relay、PG pool 和 OTel trace config 聚合指标；policy-service 当前覆盖 gRPC request / error / latency、policy decision、rule store、contacts / conversation projection、Kafka checkpoint、audit outbox、projection worker、outbox relay、PG pool 和 OTel trace config 聚合指标。labels 只允许 method、code、exposure、backend、key_scope、redis_mode、tenant_plan_source、status、failure_class、mode、outcome、operation、state、role、type、class、consumer、event、exporter、scope、action、min_role、worker、topic、bound 等低基数字段；不得输出 token、tenant_id、user_id、device_id、session_id、request_id、trace_id、conversation_id、message_id、event_id、challenge destination、target user、remark、message body、command hash 或 payload。

这个 endpoint 只用于本地 scrape / dashboard 原型，不代表生产 Prometheus、Alertmanager、指标保留策略或 SLO 告警已经完成。

## Local Prometheus

当前 9 个服务 `/metrics` 的本地 scrape / alert rule 原型位于：

```text
deploy/local/docker-compose.prometheus.yml
deploy/local/docker-compose.alertmanager.yml
deploy/local/prometheus.yml
deploy/local/alertmanager.yml
deploy/local/prometheus-api-gateway-alerts.yml
deploy/local/prometheus-identity-service-alerts.yml
deploy/local/prometheus-message-service-alerts.yml
deploy/local/prometheus-conversation-service-alerts.yml
deploy/local/prometheus-delivery-service-alerts.yml
deploy/local/prometheus-push-gateway-alerts.yml
deploy/local/prometheus-receipt-service-alerts.yml
deploy/local/prometheus-contacts-service-alerts.yml
deploy/local/prometheus-policy-service-alerts.yml
```

启动：

```powershell
.\tools\local-up-prometheus.ps1
```

如果需要同时验证本地 Alertmanager route，先启动本地 null receiver Alertmanager：

```powershell
.\tools\local-up-alertmanager.ps1
.\tools\local-up-prometheus.ps1
```

Prometheus 容器通过 `host.docker.internal:19093` 连接本地 Alertmanager。该 Alertmanager 只配置 `local-null` receiver，不发送邮件、短信、Webhook、Slack 或外部告警；它只用于验证本地 alert routing 配置能被真实进程加载。停止：

```powershell
.\tools\local-down-prometheus.ps1
.\tools\local-down-alertmanager.ps1
```

默认启动脚本只使用本机已有镜像，避免误触发外网拉取。确实需要允许 Docker 拉取镜像时显式使用：

```powershell
.\tools\local-up-prometheus.ps1 -AllowImagePull
.\tools\local-up-alertmanager.ps1 -AllowImagePull
```

启动本地观测栈或运行 smoke 前，可以先做不会启动容器、不会拉镜像的本地镜像预检：

```powershell
.\tools\check-local-observability-images.ps1
```

该预检会检查 Prometheus、Grafana、Alertmanager 的当前配置镜像是否已在本机存在；默认只报告 `present` / `missing` 并返回成功，方便纳入 `check-local`。如果某次 smoke 必须依赖这些镜像，可加严格门禁：

```powershell
.\tools\check-local-observability-images.ps1 -RequireImages
```

镜像缺失时不要默认拉取；只有确认网络和流量允许后，再显式使用对应 `-AllowImagePull` 参数。

停止：

```powershell
.\tools\local-down-prometheus.ps1
```

本地端点：

```text
Prometheus UI: http://127.0.0.1:19090
Alertmanager UI: http://127.0.0.1:19093
api-gateway scrape target from container: host.docker.internal:11904
identity-service scrape target from container: host.docker.internal:11905
message-service scrape target from container: host.docker.internal:11910
conversation-service scrape target from container: host.docker.internal:11911
delivery-service scrape target from container: host.docker.internal:11912
push-gateway scrape target from container: host.docker.internal:11913
receipt-service scrape target from container: host.docker.internal:11914
contacts-service scrape target from container: host.docker.internal:11915
policy-service scrape target from container: host.docker.internal:11916
```

当前 first-stage alert rules 覆盖 api-gateway、identity-service、message-service、conversation-service、delivery-service、push-gateway、receipt-service、contacts-service 和 policy-service。api-gateway 规则包含 gRPC errors、legacy descriptor traffic、rate-limit Redis errors、JWKS refresh failures、OTLP endpoint missing；identity-service 规则包含 gRPC errors、password / MFA / recovery-code lock、challenge delivery failure、challenge delivery outbox DLQ / expired pending rows、worker / relay runtime errors、OTLP endpoint missing；message-service 规则包含 SendMessage / PG pool / Kafka latency、outbox relay runtime error 和 OTLP endpoint missing；conversation-service 规则包含 gRPC errors、metrics query error、member-change failed compensated、worker retry、PG pool canceled acquire 和 OTLP endpoint missing；delivery-service 规则包含 gRPC errors、metrics query error、delivery outbox DLQ / ready pending rows、projection failure blocker、timeline worker retry、outbox relay retry、PG pool canceled acquire 和 OTLP endpoint missing；push-gateway 规则包含 slow session eviction、Redis route / subscriber errors、consumer worker retry、JWKS refresh failure 和 OTLP endpoint missing；receipt-service 规则包含 gRPC errors、metrics query error、receipt outbox DLQ / ready pending rows、delivery projection worker retry、outbox relay retry、PG pool canceled acquire 和 OTLP endpoint missing；contacts-service 规则包含 gRPC errors、metrics query error、contacts outbox DLQ / ready pending rows、outbox relay retry、PG pool canceled acquire、long-lived pending requests 和 OTLP endpoint missing；policy-service 规则包含 gRPC / decision errors、rule / projection / audit metrics query error、audit outbox DLQ / pending、projection worker retry、outbox relay retry、PG pool canceled acquire 和 OTLP endpoint missing。它们用于本地开发和面试演示，不是生产 SLO 阈值；生产化前还需要 Alertmanager route、retention、dashboard、sampling / label governance 和容量验证。

注意：如果本机没有 `NEXUSIM_PROMETHEUS_IMAGE` 指定的镜像或默认 `prom/prometheus:v2.54.1`，且没有显式 `-AllowImagePull`，启动脚本会直接失败并提示先准备镜像。

## Legacy Descriptor Migration Gate

移除 api-gateway legacy service descriptor 前，先对目标环境的 `/debug/metrics` 运行：

```powershell
.\tools\check-api-gateway-legacy-descriptor-migration.ps1 -MetricsUrl http://127.0.0.1:11904/debug/metrics
```

如果只想验证已保存的 JSON snapshot：

```powershell
.\tools\check-api-gateway-legacy-descriptor-migration.ps1 -SnapshotPath H:\NexusIM\loadtest-results\<run>\api-gateway-metrics.json
```

删除 legacy descriptor 前建议使用更强门禁，要求 snapshot 足够新、facade 已有真实流量、无未知 exposure，并满足 quiet window：

```powershell
.\tools\check-api-gateway-legacy-descriptor-migration.ps1 `
  -SnapshotPath H:\NexusIM\loadtest-results\<run>\api-gateway-metrics.json `
  -MaxSnapshotAge 30m `
  -RequireFacadeTraffic `
  -DisallowOtherTraffic `
  -RequiredQuietDuration 7d
```

默认 gate 要求：

```text
runtime.register_legacy_descriptors = false
grpc.legacy_descriptor_requests = 0
grpc.legacy_descriptor_last_seen_unix_ms = 0
```

这只能证明本次 snapshot 没有 legacy descriptor 暴露 / 流量；正式移除前应使用强门禁和目标环境持续 Prometheus alert / dashboard 观察，确认历史客户端已经切到 `GatewayService` facade。

要把一次 live/offline 观察落盘为可复核证据，使用：

```powershell
.\tools\record-api-gateway-legacy-observation.ps1 `
  -MetricsUrl http://127.0.0.1:11904/debug/metrics `
  -RunName api-gateway-legacy-observation-<date> `
  -RequiredQuietDuration 7d `
  -MaxSnapshotAge 30m
```

或使用已保存 snapshot：

```powershell
.\tools\record-api-gateway-legacy-observation.ps1 `
  -SnapshotPath H:\NexusIM\loadtest-results\<run>\api-gateway-metrics.json `
  -RunName api-gateway-legacy-observation-<date> `
  -RequiredQuietDuration 7d `
  -MaxSnapshotAge 30m
```

该脚本会写入：

```text
H:\NexusIM\loadtest-results\<run>\api-gateway-metrics.json
H:\NexusIM\loadtest-results\<run>\legacy-gate-output.txt
H:\NexusIM\loadtest-results\<run>\legacy-observation-summary.json
H:\NexusIM\loadtest-results\<run>\legacy-observation-report.md
```

gate 失败时仍会写入证据文件，并以非零 exit code 返回，方便 CI / 本地脚本阻断误删 legacy descriptor。它只能证明该次 snapshot 和所选 gate 参数，不能证明所有环境都完成迁移。

如果已经有多次 observation summary，可以用窗口 gate 汇总持续观察证据：

```powershell
.\tools\check-api-gateway-legacy-observation-window.ps1 `
  -SummaryRoot H:\NexusIM\loadtest-results `
  -RequiredWindow 7d `
  -MaxObservationGap 24h `
  -MinObservations 7 `
  -OutputPath H:\NexusIM\loadtest-results\api-gateway-legacy-window-summary.json
```

该 gate 会检查 observation 数量、持续窗口、最大采样间隔、单次 gate 结果、facade 流量、legacy descriptor 注册状态、legacy traffic 和 other exposure。它用于删除 legacy descriptor 前的目标环境证据汇总；正式移除仍需要在目标环境实际连续运行并归档结果。

## Tenant Quota Snapshot Gate

配置源切到 URL source 或后续控制面输出前，可对 `/debug/metrics` 或离线 snapshot 运行 quota gate：

```powershell
.\tools\check-api-gateway-quota-snapshot.ps1 `
  -SnapshotPath H:\NexusIM\loadtest-results\<run>\api-gateway-metrics.json `
  -RequireRateLimitEnabled `
  -RequiredSource url `
  -RequireVersionedSnapshot `
  -RequireChecksum `
  -RequireChecksumPolicy `
  -RequireURLHTTPS `
  -RequireURLBearerToken `
  -RequireURLTLS `
  -RequireURLClientCert `
  -MaxAllowedAge 30m
```

这些选项只验证当前进程实际应用的低敏配置源状态：source、version、checksum、checksum-required policy、URL HTTPS / bearer / TLS / client-cert guard、snapshot age/stale 和 reload error。它不证明完整配置中心、审批、灰度、签名发布或审计已经完成。

## Local Grafana

当前 9 个服务的第一阶段 Grafana dashboard provisioning 位于：

```text
deploy/local/docker-compose.grafana.yml
deploy/local/grafana-datasources.yml
deploy/local/grafana-dashboards.yml
deploy/local/grafana/dashboards/api-gateway-observability.json
deploy/local/grafana/dashboards/identity-service-observability.json
deploy/local/grafana/dashboards/message-service-observability.json
deploy/local/grafana/dashboards/conversation-service-observability.json
deploy/local/grafana/dashboards/delivery-service-observability.json
deploy/local/grafana/dashboards/push-gateway-observability.json
deploy/local/grafana/dashboards/receipt-service-observability.json
deploy/local/grafana/dashboards/contacts-service-observability.json
deploy/local/grafana/dashboards/policy-service-observability.json
```

启动：

```powershell
.\tools\local-up-grafana.ps1
```

默认启动脚本只使用本机已有镜像，避免误触发外网拉取。确实需要允许 Docker 拉取镜像时显式使用：

```powershell
.\tools\local-up-grafana.ps1 -AllowImagePull
```

停止：

```powershell
.\tools\local-down-grafana.ps1
```

本地端点：

```text
Grafana UI: http://127.0.0.1:13000
login:      admin / nexusim
datasource: http://host.docker.internal:19090
```

当前 dashboard 覆盖 api-gateway、identity-service、message-service、conversation-service、delivery-service、push-gateway、receipt-service、contacts-service 和 policy-service 的本地 Prometheus 指标。api-gateway 面板包含 request rate、error rate、facade / legacy descriptor / other exposure、legacy descriptor last-seen、latency、rate-limit decisions、rate-limit Redis mode、JWKS refresh failures 和 OTel enabled；identity-service 面板包含 request / error / latency、login / MFA lock、challenge delivery outcomes、challenge outbox status、worker runtime errors、failure class 和 OTel enabled；message-service 面板包含 SendMessage p95、PG pool acquire p95、Kafka publish p95、repository / outbox latency、batch shape、PG pool conns、outbox relay error 和 OTel enabled；conversation-service 面板包含 gRPC errors / latency、conversation/member/member-change saga 聚合、member-change worker retry、PG pool conns 和 OTel enabled；delivery-service 面板包含 gRPC errors / latency、durable read model、delivery outbox、projection blockers、worker / relay retry、PG pool conns 和 OTel enabled；push-gateway 面板包含 connected sessions、slow eviction、resume buffer、Redis route / resume、worker retry、JWKS cache 和 OTel enabled；receipt-service 面板包含 gRPC errors / latency、receipt projection、conversation summary、receipt outbox、worker / relay retry、PG pool conns 和 OTel enabled；contacts-service 面板包含 gRPC errors / latency、contact requests、contact edges、contacts outbox、relay retry、PG pool conns 和 OTel enabled；policy-service 面板包含 gRPC errors / latency、policy decisions、rule store、projection read model、Kafka checkpoints、audit outbox、worker / relay retry、PG pool conns 和 OTel enabled。它用于本地开发和面试演示，不是生产 Grafana 部署；生产化前还需要权限、datasource secret 管理、retention、SLO 阈值和告警路由。

注意：如果本机没有 `NEXUSIM_GRAFANA_IMAGE` 指定的镜像或默认 `grafana/grafana-oss:11.2.0`，且没有显式 `-AllowImagePull`，启动脚本会直接失败并提示先准备镜像。

## Local Observability Smoke

本地观测栈 smoke 会启动 Prometheus 和 Grafana，验证 Prometheus rule groups 已加载、Grafana 9 个服务 dashboard 已 provision，然后默认清理本轮启动的容器：

```powershell
.\tools\run-local-observability-smoke.ps1
```

需要同时验证本地 Alertmanager target discovery：

```powershell
.\tools\run-local-observability-smoke.ps1 -IncludeAlertmanager
```

需要把本次本地 smoke 结果沉淀成可复核证据时，显式启用 summary：

```powershell
.\tools\run-local-observability-smoke.ps1 `
  -RecordSummary `
  -RunName local-observability-smoke-<date>
```

默认写入：

```text
H:\NexusIM\loadtest-results\<run>\observability-smoke-summary.json
H:\NexusIM\loadtest-results\<run>\observability-smoke-report.md
```

summary 会记录 Prometheus rule group 数、Grafana 9 个服务 dashboard UID 覆盖和本地 smoke 边界；若本次同时使用 `-IncludeAlertmanager`，还会记录 Prometheus 发现的 active Alertmanager target。该格式由 `tools/check-observability-smoke-summary.ps1` 自测，并可用 `tools/validate-observability-smoke-summary.ps1` 离线校验已有 summary，不需要 Docker 即可进入 `check-local`。

如果需要把某次本地或目标环境观测 smoke 结果纳入统一证据索引，使用：

```powershell
.\tools\add-observability-evidence.ps1 `
  -Name "target observability smoke <date>" `
  -Kind prometheus-grafana-smoke `
  -SummaryPath "H:\NexusIM\loadtest-results\<run>\observability-smoke-summary.json" `
  -ReportPath "H:\NexusIM\loadtest-results\<run>\observability-smoke-report.md" `
  -ExpectedDashboardCount 9 `
  -Note "Target Prometheus/Grafana dashboard smoke; not production SLO evidence."
```

证据索引位于 `docs/runbook/observability-evidence.json`。该索引只保存低敏路径和边界说明，不复制指标内容；可用 `tools/validate-observability-evidence.ps1 -RequireFiles` 复核 H 盘 summary / report 是否仍存在并符合 schema。当前索引只收录已有 policy-service debug metrics smoke；真实目标环境 9 服务 dashboard smoke 仍需要另跑并归档。

如果本机尚未准备 Prometheus / Grafana 镜像，脚本默认失败而不会拉取镜像。确实需要允许拉取时显式使用：

```powershell
.\tools\run-local-observability-smoke.ps1 -AllowImagePull
```

建议在运行本地 smoke 前先执行：

```powershell
.\tools\check-local-observability-images.ps1 -RequireImages -SkipAlertmanager
```

如果本次 smoke 使用 `-IncludeAlertmanager`，不要加 `-SkipAlertmanager`，让预检同时要求 Alertmanager 镜像。如果预检提示 `missing`，先准备本机镜像，或在确认可以消耗网络流量时再使用 `-AllowImagePull`。

需要准备缺失镜像时，先 dry-run 生成明确的 pull 清单：

```powershell
.\tools\prepare-local-observability-images.ps1 -IncludeAlertmanager
```

需要把 dry-run 计划保存到 H 盘便于复核时：

```powershell
.\tools\prepare-local-observability-images.ps1 `
  -IncludeAlertmanager `
  -OutputDir H:\NexusIM\loadtest-results\observability-image-prepare-<date>
```

生成后的计划可离线校验：

```powershell
.\tools\validate-observability-image-prepare-plan.ps1 `
  -PlanPath H:\NexusIM\loadtest-results\observability-image-prepare-<date>\observability-image-prepare-plan.json `
  -RequireReport
```

确认网络和流量预算允许后，再显式拉取：

```powershell
.\tools\prepare-local-observability-images.ps1 -IncludeAlertmanager -AllowImagePull
```

不需要验证 Alertmanager 的普通本地 smoke 可以不传 `-IncludeAlertmanager`，只准备 Prometheus / Grafana。该脚本不会启动容器；它只用于把镜像准备动作和 smoke 启动动作拆开。

该 smoke 只证明本地观测配置可被真实进程加载，不证明生产 Alertmanager、SLO、retention、权限、统一日志或容量基线已完成。

## Target Observability Endpoint Smoke

目标环境已经有 Prometheus / Grafana 端点时，可以不启动本地 Docker，直接验证 Prometheus rules、Grafana 9 服务 dashboard 和可选 Alertmanager discovery：

```powershell
.\tools\run-observability-target-smoke.ps1 `
  -PrometheusBaseUrl http://<prometheus-host>:9090 `
  -GrafanaBaseUrl http://<grafana-host>:3000 `
  -GrafanaUsername admin `
  -GrafanaPassword <password> `
  -RunName target-observability-smoke-<date>
```

需要同时验证 Prometheus 已发现 Alertmanager target：

```powershell
.\tools\run-observability-target-smoke.ps1 `
  -PrometheusBaseUrl http://<prometheus-host>:9090 `
  -GrafanaBaseUrl http://<grafana-host>:3000 `
  -GrafanaUsername admin `
  -GrafanaPassword <password> `
  -IncludeAlertmanager `
  -RunName target-observability-smoke-<date>
```

该脚本会写入 summary、report 和 validation JSON，并复用 `tools/validate-observability-smoke-summary.ps1` 做离线校验。它用于目标环境 dashboard smoke 取证；仍不证明生产 Alertmanager 路由、SLO、retention、权限、统一日志或容量基线已完成。

## OpenTelemetry Collector

默认本地基础设施不会启动 collector。需要验证 OTLP trace exporter 时单独启动：

```powershell
.\tools\local-up-otel.ps1
```

停止：

```powershell
.\tools\local-down-otel.ps1
```

本地端点：

```text
OTLP gRPC: 127.0.0.1:4317
OTLP HTTP: http://127.0.0.1:4318
health:    http://127.0.0.1:13133
```

collector 配置位于 `deploy/local/otel-collector.yml`，当前只把 traces / metrics 输出到 collector debug exporter，方便本地看日志确认 span 到达；不做持久化、不做采样治理、不做告警。

## 最小 OTLP Smoke

用 `policy-service` 作为低依赖 gRPC 服务验证 OTLP trace 链路：

```powershell
.\tools\local-otel-policy-smoke.ps1
```

该脚本会：

```text
1. 如 collector 未运行，启动本地 collector；
2. 临时启用 policy-service 的 OTLP gRPC trace exporter；
3. 复用 policy-service 本地 smoke 触发 CheckMessageAction；
4. 从本次运行后的 collector debug logs 中查找 policy-service span；
5. 将 summary 和 collector log tail 写入 H:\NexusIM\loadtest-results\<run>。
```

默认脚本结束时会关闭由它启动的 collector；如需保留：

```powershell
.\tools\local-otel-policy-smoke.ps1 -KeepCollector
```

## Sampling Policy

第一阶段 trace sampling governance 只提供静态策略和本地检查，不提供动态采样控制面：

```text
deploy/local/otel-sampling-policy.json
tools/check-otel-sampling-policy.ps1
tools/check-otel-span-attributes.ps1
```

默认 profile：

```text
local_smoke = 1.0
dev_interactive = 0.25
production_starting_point = 0.05
high_volume_starting_point = 0.01
```

`api-gateway`、`message-service`、`delivery-service`、`push-gateway` 这类高吞吐入口 / 主链路默认使用 `high_volume_starting_point`；其它已接入 OTel server span 的后端服务默认使用 `production_starting_point`。本地 smoke 可以显式使用 `local_smoke=1.0`，但生产 full sampling 必须是有过期时间的临时排障动作，不能作为常态配置。

该策略文件不自动改任何服务环境变量；它用于 review、runbook 和 `.\tools\check-local.ps1` 的静态门禁。`check-otel-span-attributes.ps1` 会扫描生产 Go 代码，禁止把 `trace_id`、`request_id`、tenant / user / device / session / conversation / message id 这类高基数字段写成 OTel span attributes。生产化前仍需要集中配置、动态采样、trace retention、PII / 高基数属性审计和 collector 侧治理。

## 服务端启用方式

各服务默认关闭 trace。需要验证某个进程时，只打开该服务自己的 env 前缀：

```powershell
$env:NEXUSIM_MESSAGE_OTEL_TRACES_ENABLED = "true"
$env:NEXUSIM_MESSAGE_OTEL_TRACES_EXPORTER = "otlp-grpc"
$env:NEXUSIM_MESSAGE_OTEL_TRACES_OTLP_ENDPOINT = "127.0.0.1:4317"
$env:NEXUSIM_MESSAGE_OTEL_TRACES_OTLP_INSECURE = "true"
```

已支持第一阶段 gRPC server span 的前缀：

```text
NEXUSIM_CONTACTS_OTEL_*
NEXUSIM_IDENTITY_OTEL_*
NEXUSIM_MESSAGE_OTEL_*
NEXUSIM_CONVERSATION_OTEL_*
NEXUSIM_DELIVERY_OTEL_*
NEXUSIM_RECEIPT_OTEL_*
NEXUSIM_POLICY_OTEL_*
```

api-gateway 还支持入口 server span 和下游 gRPC client span：

```text
NEXUSIM_API_GATEWAY_OTEL_*
```

push-gateway 支持第一阶段 WebSocket connection span：

```text
NEXUSIM_PUSH_OTEL_*
```

该 span 只记录低敏连接形态字段，例如 auth mode、route backend、TLS 是否启用和 gateway id 是否配置；不记录 tenant / user / device / session id、token、payload、conversation id 或 message id。

## 边界

- span 只能记录低敏 transport / correlation 字段。
- 不记录 token、tenant/user/device/session id、conversation/message id、payload、rule 参数、SQL 错误或 provider body。
- 本地 collector 只证明 OTLP 链路可接收，不证明生产级观测。
