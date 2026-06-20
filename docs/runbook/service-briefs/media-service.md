# media-service

状态：product-active / SDD v0.1 draft 已存在 / stage-switch review passed /
implementation slice in progress。当前允许创建 `services/media-service` 目录，但只限
第一切片的 proto、migration、六层 skeleton、Docker 和观测覆盖。

Stage-switch 记录：`docs/runbook/stage-switch/media-service.md`。

定位：IM 媒体资产服务，负责图片、语音、视频、文件上传下载、对象存储元数据、
缩略图、病毒扫描、语音转码和时长 / hash 探测。

边界：

- message-service 只保存媒体引用、低敏 metadata 和 message payload，不保存二进制。
- media-service 拥有 asset metadata、upload session、scan/transcode 状态和对象存储 key。
- 下载必须经过 identity / policy / conversation visibility 校验，不能只凭 object key。
- 删除 / 撤回 / retention 必须产生可审计 tombstone 或 delete proof。

第一切片建议见 `docs/sdd/media-service.md`：

- `CreateUploadSession` / `CompleteUpload` / `GetMediaDownloadURL`。
- PostgreSQL asset metadata + S3-compatible object storage port。
- 图片 thumbnail 和 virus-scan 状态先做 mock adapter + audit。
