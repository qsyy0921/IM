# NexusIM Current Goal

本文件只维护当前可执行目标。Codex 目标框短 prompt 见根目录 `prompt.md`，
不要把长 prompt 复制到这里。

## 当前 Active Slice

```text
media-service v0.1 promotion
```

用户已点名把下一步目标指向 `media-service`。当前任务不是一次性开发全部 future
服务，而是把 `media-service` 从 future 规划推进到可实现的第一切片。

## 为什么先做 media-service

- IM 产品完整化迟早需要图片、语音、视频、文件和对象存储。
- message-service 目前只应保存媒体引用和低敏 metadata，不应承担二进制上传、
  缩略图、病毒扫描、语音转码或下载授权。
- media 边界清晰，适合作为 future 服务 promotion 的第一个样板。

## 下一步默认动作

1. 读取 `prompt.md`、`agent.md`、本文件和
   `docs/runbook/service-briefs/media-service.md`。
2. 起草 / 更新 `docs/sdd/media-service.md`，冻结 v0.1 边界：
   upload session、asset metadata、object storage port、download URL、
   scan / thumbnail / transcode 状态、policy / visibility 校验。
3. 明确非目标：不保存 message 正文，不替代 message-service，不绕过
   identity / policy / conversation visibility，不直接暴露 object key。
4. 准备 stage switch 方案：service-registry、proto、migration、六层 skeleton、
   cmd runtime、Docker、Prometheus、Grafana、service brief 和 focused tests。
5. 只有完成 SDD v0.1 和门禁影响确认后，才创建 `services/media-service`。

## 硬边界

- 不一次性 promotion 其它 future 服务。
- 不把媒体二进制塞回 message-service。
- 不直接读其它服务私有表；跨服务只走公开 API、事件或明确 port。
- 不回滚用户已有修改。
- 小改跑 focused checks；涉及 service-registry / Docker / compose / proto /
  migration / 安全边界时再扩大门禁。

## 文档路由

- 当前阶段背景：`docs/runbook/current-brief.md`
- 剩余待办：`docs/runbook/remaining-goals.md`
- media 入口：`docs/runbook/service-briefs/media-service.md`
- 未来服务总表：`docs/runbook/service-registry.json`
- 新发现待办写入 `docs/runbook/remaining-goals.md`
