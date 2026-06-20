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
- `media_outbox -> im.media.events` relay 代码切片已落，但还需要单独真实 Kafka
  smoke 验证 `PENDING -> PUBLISHED` 和 protobuf oneof payload。
- processing worker 需要单独 smoke。
