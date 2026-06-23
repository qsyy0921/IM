# media-service

状态：product-active。第一版 proto、migration、六层 skeleton、Docker、观测、
gRPC smoke、outbox relay smoke 和 mock processing worker smoke 已落。

Stage-switch 记录：`docs/runbook/stage-switch/media-service.md`。

定位：IM 媒体资产服务，负责图片、语音、视频、文件上传下载、对象存储元数据、
缩略图、病毒扫描、语音转码和时长 / hash 探测。

边界：

- message-service 只保存媒体引用、低敏 metadata 和 message payload，不保存二进制。
- media-service 拥有 asset metadata、upload session、scan/transcode 状态和对象存储 key。
- 下载必须经过 identity / policy / conversation visibility 校验，不能只凭 object key。
- 删除 / 撤回 / retention 必须产生可审计 tombstone 或 delete proof。

已落第一版：

- `CreateUploadSession` / `CompleteUpload` / `GetMediaDownloadURL`。
- PostgreSQL asset metadata + S3-compatible object storage port。
- 图片 thumbnail 和 virus-scan 状态先做 mock adapter + audit。
- 本地 fake object HTTP adapter 可用于 Web / PC 群头像上传 / 展示 first path；浏览器显式
  PUT 到 media-service 返回的 upload URL，再由 api-gateway BFF 完成上传并更新
  conversation profile；展示时 BFF 通过 media-service 换取短期 download URL，fake
  adapter 只在本地内存保存并返回已上传对象内容。

下一步：

- 当前 active slice 是 client platform MVP foundation；media-service 只处理阻塞
  客户端或消息媒体引用链路的 P0/P1。
- 真实 S3-compatible adapter、scanner、thumbnail / transcode provider 和 CDN /
  download policy 后置。
