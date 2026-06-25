# ai-eval-service Brief

状态：foundation-active / low-sensitive eval catalog and gates。

## 已落

- Eval catalog / recorder / gate first path，覆盖 RAG、Summary、Agent、memory、retrieval、
  tool policy、Python worker output、action execution safety 和 required local adapters。
- Optional service-stack adapters：collaborative memory、profile aggregation、
  public candidate review / temporal update、profile repair approval、RAG-Agent demo。
- RAG-Agent business proposal cases：source-chain audit boundary 和 approved
  conversation note / profile mutation execution。
- Provider replay / repair cases 覆盖 provider failure metrics、operator handoff、
  admin / workflow handoff、redrive invocation / execution、external audit append。
- Workflow action boundary cases 覆盖 provider replay queue、external approval binding、callback
  wait / delivery / redrive、operator queues、compensation review / execution readiness。
- Provider readiness / source coverage / vector lane checks 已进入相关 smoke 输出。
- Group-memory ambiguity safety expansion 覆盖 no unsupported memory hidden path 和
  no raw prompt persistence。

## 边界

- 不参与线上热路径，不存 raw prompt / raw answer / raw proposal。
- Gate 只输出 case id、pass/fail、failure class、hash 和低敏 diagnostics。
- 真实模型和 provider 只在显式 opt-in adapter 中运行。

## 下一步

- 扩展 provider runtime readiness、更多 Agent action boundary、redrive / repair cases 和 service-stack live adapters。
