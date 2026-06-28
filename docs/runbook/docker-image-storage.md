# Docker Image Storage Runbook

本文只管理 Docker 镜像归档和各机器 SSD 加载策略。它不迁移 Docker Desktop /
Docker Engine 的运行数据目录，也不把正在运行的 container 当作长期资产。

## 当前磁盘策略

| 位置 | 用途 |
| --- | --- |
| `H:\NexusIM\docker-images` | 机械硬盘镜像归档仓库，保存完整 `docker save` tar 和 manifest。 |
| Windows 本机 SSD 可用约 300GB | 开发、少量热镜像、客户端和本机 smoke。不要长期堆全量历史镜像。 |
| Ubuntu SSD 可用约 400GB | 优先承载 PostgreSQL / Kafka / Redis / observability 等 infra 容器和必要服务镜像。 |
| MacBook SSD 可测试容量不超过 30GB | 只加载最小 arm64 应用镜像；不要加载 Kafka、Schema Registry、Kafka UI 或全量镜像包。 |

后续如果迁到 SSD，只迁当前 active profile 需要的镜像，不把 H 盘全量归档一次性加载到每台机器。

## 三机资源定位

| 机器 | 当前资源 | 后续资源 | 推荐角色 |
| --- | --- | --- | --- |
| Windows | i5-12600KF + 48GB DDR4，本机 SSD 可用约 300GB；RTX 3080 20GB 显存改版卡 | 不变 | 主开发机、客户端调试、少量本机 smoke、镜像构建入口、本地小模型 / embedding / rerank / eval GPU 节点。 |
| Ubuntu | 双路 Xeon E5-2686 v4 + 48GB DDR3，SSD 可用约 400GB | 计划升级到 128GB DDR3 | 重型 Docker 节点，优先放 PostgreSQL / Kafka / Redis / observability / 压测依赖和热点群聊压测。 |
| MacBook Air | M1 + 16GB，SSD 可测试容量不超过 30GB | 不变 | arm64 兼容性验证、轻量客户端/少量应用服务 smoke，不承载重型中间件。 |

资源策略：

```text
重型中间件和压测优先放 Ubuntu。
Windows 保持开发体验和客户端调试空间；需要 GPU 的 AI 小模型实验优先放 Windows。
MacBook 只做轻量 arm64 验证，不追求完整分布式运行。
```

Windows 当前 Docker Desktop 已配置 16 CPU / 40GB 内存，并可通过 NVIDIA runtime 使用本机 GPU：

```powershell
docker run --rm --gpus all nvidia/cuda:12.4.1-base-ubuntu22.04 nvidia-smi
```

已验证输出中 GPU 为 `NVIDIA GeForce RTX 3080`，显存 `20480MiB`。该 GPU 只作为本地
AI runtime profile 的小模型、embedding、rerank、memory extraction 和 eval 加速节点；不要把
单卡结果写成生产推理容量或线上 SLA。

## 镜像和容器边界

```text
image = 可复用安装包，可归档、复制、load。
container = 某次运行实例，可删除重建，不作为长期资产。
volume = 数据状态，测试环境优先通过 migration / seed 重建，不依赖旧匿名 volume。
```

## 当前 Docker 内容准备状态

截至当前三机有线实验网准备阶段：

| 机器 | Docker 内容 | 当前用途 |
| --- | --- | --- |
| Windows `172.31.50.1` | 已构建完整 `nexusim/*:local` linux/amd64 服务镜像；保留核心中间件镜像和 NVIDIA CUDA smoke 镜像。 | 镜像构建、导出归档、压测客户端、客户端调试、GPU 小模型实验。 |
| Ubuntu `172.31.50.2` | 已加载完整 `nexusim/*:local` linux/amd64 服务镜像；已加载 PostgreSQL / Redis / Kafka / Schema Registry / Kafka UI。 | 当前主 Docker 运行节点；承载中间件、核心后端服务和 worker/relay。 |
| MacBook `172.31.50.3` | 只构建了最小 arm64 `nexusim/api-gateway:local` 和 `nexusim/push-gateway:local`。 | arm64 兼容性和轻量 gateway / receiver 验证；不承载完整集群。 |

Ubuntu 当前 runtime 目录：

```text
/home/qsyy0921/nexusim-runtime
```

该目录中的 compose 文件是有线实验网 runtime 副本，已经把跨机访问地址改成
`172.31.50.2`，Kafka UI 端口改成 `19090:8080`，避免和 Ubuntu mihomo controller
的 `9090` 冲突。仓库里的 `deploy/local/*.yml` 仍是通用模板，不直接写死某台机器。

当前主运行策略：

```text
Ubuntu: 跑 PostgreSQL / Redis / Kafka / 后端服务容器。
Windows: 保留完整镜像、构建和压测能力；必要时跑客户端和 GPU AI 小模型。
MacBook: 保留最小 arm64 镜像；只做轻量验证。
```

不要把 MacBook 准备成完整后端节点；它的磁盘和内存不适合长期放 Kafka、Schema Registry
或全量服务镜像。

MacBook 的所有轻量职责都必须能被 Windows 接管。当前 MacBook 只保留
`api-gateway` / `push-gateway` 两个 arm64 镜像；Windows 已保留同名 linux/amd64 镜像。
因此如果 MacBook 不在线，直接在 Windows 或 Ubuntu 使用同名镜像启动对应轻量角色即可。

```text
MacBook: nexusim/api-gateway:local   linux/arm64
MacBook: nexusim/push-gateway:local  linux/arm64
Windows: nexusim/api-gateway:local   linux/amd64
Windows: nexusim/push-gateway:local  linux/amd64
```

后续如果给 MacBook 增加新的轻量镜像，必须同步确认 Windows 也存在同名 amd64 镜像；
否则不要把该能力写成可用的分布式测试节点能力。

当前 H 盘归档：

| 归档 | 平台 | 覆盖内容 |
| --- | --- | --- |
| `nexusim-all-windows-amd64-20260627.tar` | `linux/amd64` | Windows 当前全部非 dangling 镜像：NexusIM 服务、中间件、CUDA smoke、当前本机工具镜像。 |
| `nexusim-services-amd64-20260627-full29.tar` | `linux/amd64` | 29 个 NexusIM 服务镜像。 |
| `nexusim-middleware-core-amd64-20260625.tar` | `linux/amd64` | PostgreSQL / Redis / Kafka core。 |
| `nexusim-middleware-optional-amd64-20260625.tar` | `linux/amd64` | Schema Registry / Kafka UI。 |
| `nexusim-mac-arm64-20260627.tar` | `linux/arm64` | MacBook 轻量 `api-gateway` / `push-gateway` 镜像。 |

`nexusim-all-windows-amd64-20260627.tar` 是当前最完整的 Windows/Ubuntu 离线恢复包；
其它拆分包保留用于只加载服务或中间件的场景。

## 推荐归档分组

| 归档 | 内容 | 目标机器 |
| --- | --- | --- |
| `nexusim-services-amd64-<date>.tar` | 当前 Windows / Ubuntu 可用的 `nexusim/*:local` 服务镜像 | Windows、Ubuntu |
| `nexusim-middleware-core-amd64-<date>.tar` | `postgres:16-alpine`、`redis:7-alpine`、`confluentinc/cp-kafka:7.7.1` | Ubuntu 为主 |
| `nexusim-middleware-optional-amd64-<date>.tar` | Schema Registry、Kafka UI 等非主链路镜像 | 只在明确需要时加载 |
| `nexusim-services-arm64-<date>.tar` | Mac 需要的 arm64 应用镜像 | MacBook |

MacBook 的 30GB 限制下，优先只放：

```text
push-gateway
api-gateway 或少量演示服务
必要 client / runner
```

不要在 MacBook 上长期放：

```text
Kafka
Schema Registry
Kafka UI
OpenSearch
Milvus
全量 NexusIM 服务镜像
```

## 导出当前 Windows 镜像

列出当前可归档镜像：

```powershell
.\tools\export-docker-image-archive.ps1 -ListAvailable
```

导出 NexusIM 服务镜像到机械硬盘：

```powershell
.\tools\export-docker-image-archive.ps1 `
  -Name nexusim-services-amd64-<date> `
  -IncludeNexusIMServices
```

导出核心中间件：

```powershell
.\tools\export-docker-image-archive.ps1 `
  -Name nexusim-middleware-core-amd64-<date> `
  -IncludeCoreMiddleware
```

导出可选中间件时要显式执行，避免占用 Mac 或 Windows SSD：

```powershell
.\tools\export-docker-image-archive.ps1 `
  -Name nexusim-middleware-optional-amd64-<date> `
  -IncludeOptionalMiddleware
```

输出位置：

```text
H:\NexusIM\docker-images\archives\<name>.tar
H:\NexusIM\docker-images\archives\<name>.manifest.json
```

## 在目标机器加载

Windows / Ubuntu：

```powershell
docker load -i H:\NexusIM\docker-images\archives\nexusim-services-amd64-<date>.tar
```

Ubuntu 通过 SSH / scp 拿到 tar 后：

```bash
docker load -i nexusim-services-amd64-<date>.tar
docker load -i nexusim-middleware-core-amd64-<date>.tar
```

MacBook 只加载 arm64 小包。不要把 amd64 镜像当作 Mac 的默认运行形态；如果临时
加载 amd64 镜像，必须在报告里说明会走 emulation，性能不可作为结论。

## 当前有线实验网

镜像归档不依赖 IP。当前三机已经按交换机实验网固定为：

| 机器 | 有线地址 | 当前角色 |
| --- | --- | --- |
| Windows | `172.31.50.1/24` | 主开发机、镜像构建入口、压测客户端。 |
| Ubuntu | `172.31.50.2/24` | 重型 Docker 节点，优先承载中间件和后端服务容器。 |
| MacBook | `172.31.50.3/24` | 轻量客户端 / receiver / arm64 验证节点。 |

Wi-Fi 地址只用于上网、代理和 SSH recovery；NexusIM 服务调用、Docker 暴露端口和压测流量优先使用
`172.31.50.0/24`。

如果后续切换网络，只切换 `.env.wifi` / `.env.wired` 里的 host：

```text
Wi-Fi: 192.168.0.x
Wired: 172.31.50.x
```

容器应该通过 compose 重新创建，不搬运运行中的 container。
