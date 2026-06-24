# action-executor Brief

状态：foundation-active / approved execution + guarded adapters + redrive first path。

## 已落

- `ExecuteApprovedAction` gRPC：强制 proposal / approval / prepared audit，校验 agent-service、skill-registry、policy-service 后写 execution audit 和 tool result。
- 本地安全 `nexusim.local.echo`、guarded external HTTP provider adapter、外部 MCP failure 稳定分类、tool output safety gate。
- Conversation note / profile opt-in business adapters：只走 conversation-service public API，低敏 output，不读写私表。
- Provider failure lifecycle：retryable -> `RETRY_PENDING`，non-retryable / unsafe -> `DLQ`；worker 只做 bounded retry bookkeeping，不重放 tool。
- Provider failure audit / redrive-plan operator handoff：只输出低敏 artifact，不修改 failure row。
- Provider failure metrics：`/metrics` / `/debug/metrics` 输出 status、retryable、
  due retry 和 classification 聚合，不输出 raw provider error、tool input / output、
  secret 或 PII。
- Batch redrive operator handoff：redrive plan 输出 batch id、candidate count、
  fresh proposal / approval / prepared audit requirements，不自动 replay。
- `RedriveProviderFailure`：只针对 `DLQ` source，要求 fresh proposal / approval /
  prepared audit、匹配 skill / tool / resource、新 input 和 reason hash，复用正常执行链；
  execution audit 记录 source failure id 和 reason hash。
- Docker / Prometheus / Grafana wiring、focused tests、PG integration、ai-eval action preflight safety adapter。

## 边界

- 不保存 raw `input_json`、provider raw error、provider output、secret 或 PII。
- 未配置 adapter 的业务 tool 必须 `executed=false`，不得伪造成功。
- Redrive 是专用 API，不是普通 repair / DLQ tool action；不恢复旧 raw input，不自动 replay 旧 provider output。
- 真实业务 mutation 必须新增显式 adapter、公开业务 API、operator / policy 边界。

## 下一步

- Provider replay approval / audit first path、operator UI。
