# Distributed Docker Smoke Index

本文记录跨服务 / 跨机器 / Docker 容器化 smoke。服务级压测仍放在各自目录；这里只放会同时覆盖多个微服务的系统级验证。

## 当前报告

| 报告 | 说明 |
| --- | --- |
| `loadtest-report-20260616-seeded-capacity-baseline.md` | 本地 seeded capacity baseline 覆盖 message / conversation / delivery 三个需要预置业务状态的 runner，原始结果在 H 盘 |
| `loadtest-report-20260612-all-service-docker-smoke.md` | Windows Docker runner 通过有线 `172.31.50.2` 访问 Mac Docker 中的 NexusIM 基础设施和服务容器，覆盖 conversation / message / delivery / push / receipt / contacts / identity 的小规模功能 smoke |

## 原始结果

系统级 Docker smoke 的原始 JSON 和日志放在机械盘：

```text
H:\NexusIM\loadtest-results
```

仓库只保存报告和索引，避免把大体积运行数据写入 E 盘 Git 工作区。
