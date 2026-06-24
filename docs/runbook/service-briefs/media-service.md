# media-service Brief

状态：product-active / media asset first path。

## 已落

- Proto、migration、six-layer skeleton、Docker、observability、gRPC smoke。
- `CreateUploadSession`、`CompleteUpload`、`GetMediaDownloadURL`。
- Asset metadata、upload session、S3-compatible object storage port。
- Mock processing worker、outbox relay、fake object HTTP adapter。
- Web / PC 群头像上传 / 展示 first path。

## 边界

- message-service 只保存媒体引用和低敏 metadata，不保存二进制。
- media-service 拥有 asset metadata、upload session、scan / transcode 状态和 object key。
- 下载必须经过 identity / policy / conversation visibility 校验，不能只凭 object key。
- 删除 / 撤回 / retention 必须产生可审计 tombstone 或 delete proof。

## 下一步

- 真实 S3-compatible adapter、scanner、thumbnail / transcode provider、CDN /
  download policy、retention / delete proof。
