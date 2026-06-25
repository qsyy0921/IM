# ai-eval-service Brief

状态：foundation-active / low-sensitive eval catalog and gates。

## 已落

- Eval catalog / recorder / gate first path，覆盖 RAG、Summary、Agent、memory、
  retrieval、action safety。
- Required local adapters：RAG / Summary grounding safety、LLM boundary safety、
  action-executor preflight safety。
- Optional service-stack adapters：collaborative memory、profile aggregation、
  public candidate review / temporal update、profile repair approval、RAG-Agent demo。
- RAG-Agent business proposal cases：source-chain audit boundary 和 approved
  conversation note / profile mutation execution。
- Action-executor provider failure ops cases：low-sensitive metrics 和 batch redrive
  operator handoff。
- Provider replay admin / workflow handoff case：验证低敏
  `PROVIDER_REPLAY_REQUEST` admin operation、workflow-service `REPAIR_APPROVAL`
  handoff、action-executor final execution owner，以及 no auto replay / no raw
  payload / immutable DLQ row。
- Provider replay submit operator case：验证 admin operator 从 action-executor handoff
  artifact 创建 `PROVIDER_REPLAY_REQUEST`，校验 payload hash，且 submit 阶段不执行 replay。
- Workflow provider replay queue view case：验证 workflow-service 只列低敏
  `REPAIR_APPROVAL` / action-executor `PROVIDER_REPLAY_REQUEST` 工作流，不执行 replay、
  不读取 raw payload、不绕过 action-executor final execution owner。
- Workflow external approval binding case：验证外部审批 manifest 必须绑定当前
  workflow type / step / target / payload hash / approval policy，binding mismatch 不记录
  decision、不执行 replay。
- Workflow external callback wait case：验证 workflow-service 可创建低敏 wait workflow
  和 decision manifest template，但不记录 decision、不执行 target action。
- Workflow operator queue case：验证 action approval、repair approval、provider replay、
  admin operation 和 compensation queue 只输出低敏 workflow summary，不记录 decision、
  不执行 replay。
- Workflow compensation review bundle case：验证 `COMPENSATION_PENDING` workflow 和
  `ACTIVE` instruction refs 必须按 workflow id / payload hash / target 绑定；审查包只输出
  低敏 refs / hash，不记录 decision、不执行 compensation。
- Workflow compensation review page case：验证低敏审查包渲染为 HTML 时仍只输出
  workflow / instruction refs 和 hash，不执行 compensation、不记录 decision、不泄漏 raw
  payload / reason / path / provider body / EvidencePack。
- Provider readiness / source coverage / vector lane checks 已进入相关 smoke 输出。
- Group-memory ambiguity safety expansion：asker-bound term ambiguity、visible-chain
  incomplete abstention、missing visibility projection fail-closed、audience-language
  profile overgeneralization、no unsupported memory hidden path 和 no raw prompt persistence
  已进入 `profile-agent-output-safety` 本地低敏 adapter。

## 边界

- 不参与线上热路径，不存 raw prompt / raw answer / raw proposal。
- Gate 只输出 case id、pass/fail、failure class、hash 和低敏 diagnostics。
- 真实模型和 provider 只在显式 opt-in adapter 中运行。

## 下一步

- 扩展 provider runtime readiness、更多 Agent action boundary、redrive / repair cases
  和后续 service-stack live adapters。
