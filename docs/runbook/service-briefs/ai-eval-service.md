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
- Provider readiness / source coverage / vector lane checks 已进入相关 smoke 输出。

## 边界

- 不参与线上热路径，不存 raw prompt / raw answer / raw proposal。
- Gate 只输出 case id、pass/fail、failure class、hash 和低敏 diagnostics。
- 真实模型和 provider 只在显式 opt-in adapter 中运行。

## 下一步

- 扩展 group memory、retrieval miss、provider runtime readiness、Agent action boundary
  和 redrive / repair cases。
