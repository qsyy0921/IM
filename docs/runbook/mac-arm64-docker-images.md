# Mac arm64 Docker 镜像准备说明

目标：Mac 上优先运行 `linux/arm64` 原生镜像，避免 Docker Desktop 用 amd64 emulation 影响性能。

当前已确认：

- `nexusim/conversation-service:local` 是 `arm64/linux`
- `nexusim/message-service:local` 是 `arm64/linux`
- `nexusim/delivery-service:local` 是 `arm64/linux`
- `nexusim/push-gateway:local` 是 `arm64/linux`
- `nexusim/receipt-service:local` 是 `arm64/linux`
- `nexusim/contacts-service:local` 是 `arm64/linux`

所以上面六个业务服务镜像不用重新拉。需要处理的是基础设施镜像。

## 推荐执行方式

优先在 Mac 本机 Terminal 里执行下面命令，不要通过 Windows SSH 执行。原因是 Docker Desktop CLI 在 SSH 会话里可能访问不到 macOS login keychain，导致 `error getting credentials`。

如果必须通过 SSH 执行，先在 Mac 上解锁 keychain，或者在 SSH 会话里执行：

```bash
security unlock-keychain "$HOME/Library/Keychains/login.keychain-db"
```

## 可选代理

如果 Mac 需要走本机代理：

```bash
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
export NO_PROXY=localhost,127.0.0.1,172.31.50.0/24
```

## 拉取 arm64 基础镜像

PostgreSQL 和 Redis 优先直接拉 arm64：

```bash
docker pull --platform linux/arm64 postgres:16-alpine
docker pull --platform linux/arm64 redis:7-alpine
```

Kafka UI 如果需要在 Mac 上运行，也先尝试 arm64：

```bash
docker pull --platform linux/arm64 provectuslabs/kafka-ui:latest
```

## Kafka / Schema Registry 策略

当前从 Windows 传到 Mac 的下面两个 Confluent 镜像是 `amd64/linux`：

```text
confluentinc/cp-kafka:7.7.1
confluentinc/cp-schema-registry:7.7.1
```

不建议在 Mac 上用它们做性能测试。如果需要 Kafka 性能或分布式 smoke，优先让 Kafka / Schema Registry 跑在 Windows，Mac 只跑 NexusIM 服务节点，通过有线 `172.31.50.*` 访问 Windows Kafka。

如果确实要 Mac 原生跑 Kafka，先检查候选镜像是否支持 arm64，再拉取：

```bash
docker buildx imagetools inspect apache/kafka:3.9.0 | grep -E "linux/arm64|Platform"
docker pull --platform linux/arm64 apache/kafka:3.9.0
```

如果 `apache/kafka:3.9.0` 不支持当前 Mac 环境，再换其它 Kafka 镜像，但必须先检查 `linux/arm64`，不要直接拉 amd64。

Schema Registry 不是当前 NexusIM protobuf 链路的硬依赖；如果 arm64 镜像不好找，可以暂时不在 Mac 上运行。

## 验证镜像架构

执行：

```bash
for img in \
  nexusim/conversation-service:local \
  nexusim/message-service:local \
  nexusim/delivery-service:local \
  nexusim/push-gateway:local \
  nexusim/receipt-service:local \
  nexusim/contacts-service:local \
  postgres:16-alpine \
  redis:7-alpine \
  provectuslabs/kafka-ui:latest; do
  printf "%-52s " "$img"
  docker image inspect --format '{{.Architecture}}/{{.Os}}' "$img" 2>/dev/null || echo missing
done
```

期望业务服务、PostgreSQL、Redis 都显示：

```text
arm64/linux
```

如果 Kafka 仍是 amd64，不要用 Mac 跑 Kafka 压测；把 Kafka 放在 Windows。

## 简单启动验证

```bash
docker run --rm postgres:16-alpine postgres --version
docker run --rm redis:7-alpine redis-server --version

for svc in conversation-service message-service delivery-service push-gateway receipt-service contacts-service; do
  docker run --rm "nexusim/$svc:local"
done
```

六个业务服务在未设置 mode 时会输出 idle 提示并退出，退出码为 0 即说明镜像可执行。

## 清理旧 amd64 基础镜像

确认 arm64 镜像可用后，可以删除旧的 amd64 基础镜像：

```bash
docker rmi postgres:16-alpine redis:7-alpine provectuslabs/kafka-ui:latest
```

如果删除后再按上面的 `docker pull --platform linux/arm64 ...` 拉取，Docker Desktop 应显示 arm64 版本。
