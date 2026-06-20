# media-service smoke

本目录记录 `media-service` 本地小规模 smoke。原始 summary / log 写入
`H:\NexusIM\loadtest-results`，仓库只保存说明和报告。

## 最小 gRPC smoke

脚本：

```powershell
.\loadtest\media\run-local-smoke.ps1
```

默认行为：

1. 构建 `services/media-service/cmd/media-service` 和 `loadtest/media`。
2. 启动 `NEXUSIM_MEDIA_SERVICE_MODE=grpc`，使用 fake object store。
3. runner 通过真实 gRPC 调用：
   `CreateUploadSession -> CompleteUpload -> GetMediaAsset -> GetMediaDownloadURL`。
4. runner 查询 PostgreSQL，确认：
   - asset 进入 `READY`；
   - scan 状态为 `PASSED`；
   - `media.asset.uploaded.v1` 和 `media.asset.ready.v1` outbox 已落库；
   - `media_access_audit` 写入一次允许访问；
   - public response、fake presigned URL 和 outbox payload 不暴露 `object_key`。

边界：

- 这是 fake object store 下的最小 gRPC smoke，不证明真实 S3 / scanner /
  thumbnail / transcode provider 可用。
- 当前只验证 media 本服务的 upload / complete / download 授权本地链路，不发布
  `im.media.events`。
- processing worker 需要单独 smoke。

## Outbox relay / Kafka smoke

脚本：

```powershell
.\loadtest\media\run-local-smoke.ps1 -WithOutboxRelay
```

默认行为：

1. 确认 / 创建本轮临时 `im.media.events.*` Kafka topic。
2. 启动 `NEXUSIM_MEDIA_SERVICE_MODE=grpc` 和
   `NEXUSIM_MEDIA_SERVICE_MODE=outbox-relay` 两个真实进程。
3. runner 复用 gRPC smoke 主链路，随后等待 `media_outbox` 从 `PENDING`
   变成 `PUBLISHED`。
4. runner 从 Kafka topic 读回 typed protobuf `MediaEvent`，确认：
   - `media.asset.uploaded.v1` 和 `media.asset.ready.v1` 都已发布；
   - payload oneof 分别为 `asset_uploaded` / `asset_ready`；
   - Kafka event 不暴露 `object_key` 或 `download_url`；
   - `media_outbox PENDING=0 / PUBLISHED=2 / DLQ=0`。

报告：

- `docs/runbook/loadtest/media-service/loadtest-report-20260620-media-outbox-relay-smoke.md`

边界：

- 这是单节点本地 Kafka smoke，不证明 Kafka HA / ISR / 网络分区语义。
- 仍使用 fake object store，不证明真实 S3 / scanner / thumbnail / transcode provider。
- raw summary / logs 写入 `H:\NexusIM\loadtest-results`。
