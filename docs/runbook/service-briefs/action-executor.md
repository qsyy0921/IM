# action-executor Brief

状态：foundation-active / approved execution + guarded adapters + provider replay audit handoff path。

## 已落

- `ExecuteApprovedAction` gRPC 强制 proposal / approval / prepared audit，校验 agent-service、
  skill-registry、policy-service 后写 execution audit 和 tool result。
- adapters：本地安全 echo、guarded external HTTP、外部 MCP failure 分类、tool output safety、
  Conversation note / profile opt-in business adapters；业务 adapter 只走公开 API。
- Provider failure lifecycle、metrics、batch handoff、operator UI、admin/workflow handoff、
  review/readiness/invocation、受控 redrive execution、result manifest、audit append handoff、
  external audit append operator 和 audit append result manifest 已落。
- `RedriveProviderFailure` 只针对 DLQ source，要求 fresh proposal / approval / prepared audit、
  匹配 skill / tool / resource、新 input 和 reason hash，并复用正常执行链。
- Docker / Prometheus / Grafana wiring、focused tests、PG integration、ai-eval action safety cases 已落。

## 边界

- 不保存 raw `input_json`、provider raw error / output、secret 或 PII。
- 未配置 adapter 的业务 tool 必须 `executed=false`，不得伪造成功。
- Redrive 是专用 API，不恢复旧 raw input，不自动 replay 旧 provider output。
- UI / handoff / readiness / invocation / result / audit handoff 都不是执行证明。
- 真实 mutation 必须新增显式 adapter、公开业务 API、operator / policy 边界。

## 下一步

- 更多 Agent action boundary / repair cases、provider-grade replay UI，以及 group memory /
  retrieval / eval redrive / repair cases。
