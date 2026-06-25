# action-executor Brief

状态：foundation-active / approved execution + guarded adapters + provider replay audit handoff path。

## 已落

- `ExecuteApprovedAction` gRPC 强制 proposal / approval / prepared audit，校验 agent-service、skill-registry、policy-service 后写 execution audit 和 tool result。
- 本地安全 echo、guarded external HTTP、外部 MCP failure 分类、tool output safety gate、Conversation note / profile opt-in adapter 已落；业务 adapter 只走公开 API。
- Provider failure lifecycle / metrics / redrive-plan / batch handoff 已落；retryable 只进入 bounded retry bookkeeping，non-retryable / unsafe 进入 DLQ，不自动 replay。
- `RedriveProviderFailure` 只针对 DLQ source，要求 fresh proposal / approval / prepared audit、匹配 skill / tool / resource、新 input 和 reason hash，复用正常执行链。
- Provider replay UI、admin/workflow handoff、review page、readiness page、redrive invocation manifest 都是低敏只读或 contract 生成，不执行 tool、不修改 DLQ、不泄漏 raw provider artifact。
- `loadtest/actionexecutor -mode provider-replay-redrive` 默认 preflight，校验低敏 invocation manifest 与仓库外 raw resource id / new input / reason hash；显式 `-execute` 才调用 `RedriveProviderFailure`。
- `write-provider-replay-redrive-result-manifest.ps1` 绑定 redrive invocation 与执行 summary，输出低敏 result refs / status，不执行 redrive、不追加 audit、不修改 DLQ。
- `write-provider-replay-redrive-audit-append-manifest.ps1` 从低敏 redrive result manifest 生成 external audit append manifest，不调用 audit-service、不执行 redrive、不携带 raw operator input。
- `loadtest/actionexecutor -mode external-audit-append` 默认 preflight，校验仓库外低敏 audit append manifest、`attributes_json` hash 和 forbidden raw provider artifact；显式 `-execute` 才通过 audit-service 公开 `AppendAuditRecord` 追加审计。
- `write-provider-replay-redrive-audit-append-result-manifest.ps1` 绑定 external audit append 执行 summary 与 audit record hash，输出低敏最终审计证据，不调用 audit-service。
- Docker / Prometheus / Grafana wiring、focused tests、PG integration、ai-eval action safety cases 已落。

## 边界

- 不保存 raw `input_json`、provider raw error / output、secret 或 PII。
- 未配置 adapter 的业务 tool 必须 `executed=false`，不得伪造成功。
- Redrive 是专用 API，不是普通 repair / DLQ tool action；不恢复旧 raw input，不自动 replay 旧 provider output。
- Provider replay UI / handoff / review / readiness / invocation 都不是 redrive 已执行证明；只有显式 `-execute` 且 action-executor `RedriveProviderFailure` 成功，才进入最终执行链。
- Provider replay result manifest 只是执行后证据绑定；redrive audit append manifest handoff 只是 audit append contract 生成器，不是 audit-service caller。
- External audit append 是 operator 追加审计路径，不是 action-executor 热路径同步写 audit-service；audit append result manifest 只是执行后证据绑定，不能绕过 audit-service 公开 API，也不能携带 raw provider artifacts。
- 真实业务 mutation 必须新增显式 adapter、公开业务 API、operator / policy 边界。

## 下一步

- 更多 Agent action boundary / repair cases、provider-grade replay UI，以及 group memory / retrieval / eval redrive / repair cases。
