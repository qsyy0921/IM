# 本地双机压测 Runbook

## 1. 机器与网络

当前本地双机压测只用于开发阶段，不代表目标态生产拓扑。

| 角色 | 地址 | 用途 |
| --- | --- | --- |
| Windows 本机 | `192.168.0.141` | 服务端、开发机 |
| MacBook | `192.168.0.182` | 压测客户端、双向 callback/mock receiver |

两端本地 Git/HTTP 代理统一使用：

```text
127.0.0.1:7890
```

## 2. 端口分配

`8080` 不作为 NexusIM 本地压测端口。

| 端口 | 方向 | 用途 |
| ---: | --- | --- |
| `10495` | MacBook -> Windows；Windows -> MacBook 可对称使用 | 主 HTTP/API 压测入口 |
| `10496` | MacBook -> Windows | push-gateway WebSocket 压测入口 |
| `10497` | MacBook -> Windows | metrics/debug，只在压测窗口开放 |
| `10498` | Windows -> MacBook | callback/mock receiver，用于双向新建连接场景 |
| `10499` | MacBook <-> Windows | load coordinator / report endpoint |
| `10500-10510` | 按需双向 | 预留给服务级 SDD、故障注入、临时对照实验 |

两台机器可以使用相同端口号，因为监听地址不同，例如：

```text
192.168.0.141:10495
192.168.0.182:10495
```

这两个监听不冲突。

## 3. 防火墙约束

Windows 防火墙规则：

```text
规则名: NexusIM LoadTest 10495-10510 from MacBook
协议: TCP
本地端口: 10495-10510
远端地址: 192.168.0.182
动作: Allow
```

如 MacBook 开启系统防火墙，只允许 Windows 访问同一端口段：

```text
192.168.0.141 -> 10495-10510
```

非压测窗口不启动这些端口上的服务。

## 4. 服务监听要求

被压测服务必须监听：

```text
0.0.0.0:<port>
```

不能只监听：

```text
127.0.0.1:<port>
```

否则对端机器无法访问。

## 5. 验证命令

MacBook 验证 Windows 服务端口：

```bash
nc -vz 192.168.0.141 10495
curl -I http://192.168.0.141:10495/healthz
```

Windows 验证 MacBook callback/mock receiver：

```powershell
Test-NetConnection 192.168.0.182 -Port 10498
```

## 6. 压测原则

- 第一轮只压真实服务进程，不压固定字符串 toy endpoint。
- 压测脚本不能写死 IP、端口、并发和持续时间，必须通过参数或环境变量传入。
- 每次压测记录目标 commit、机器、端口、并发、请求数、p95/p99、错误率。
- 压测结果输出到 `loadtest/results/<date>/`。
- 先跑短压测确认功能，再跑长压测观察资源和稳定性。

推荐参数形式：

```bash
loadtest/sendmessage \
  --target=http://192.168.0.141:10495 \
  --vus=100 \
  --duration=60s \
  --result-dir=loadtest/results/2026-06-08
```

等价环境变量形式：

```bash
NEXUSIM_TARGET=http://192.168.0.141:10495
NEXUSIM_VUS=100
NEXUSIM_DURATION=60s
NEXUSIM_RESULT_DIR=loadtest/results/2026-06-08
```
