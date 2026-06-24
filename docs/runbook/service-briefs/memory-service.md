# memory-service Brief

状态：foundation-active / collaborative memory and profile projection。
First slice：`memory_service.proto`、projection usecase、candidate review path。

## 已落

- Group memory projection、StructuredMemoryEvent、source refs、visibility window、
  revoke hidden、supersession、review state。
- Rules-v0.2 extraction cue classifier 和 Python memory extraction candidate adapter。
- Public candidate submit / review / approve / reject / supersede temporal update path。
- Profile aggregate recompute / archive first path、profile repair approval / negative gate。
- Memory graph edges 和 profile evidence 已进入 retrieval / RAG / Agent EvidencePack。

## 边界

- 不把单条群聊直接升级为个人画像。
- Python 只输出候选、hash 和 citation metadata；最终 memory 必须经 Go-side review。
- 不绕过 conversation / delivery / policy visibility。
- 不保存 raw provider body 或不必要敏感 payload。

## 下一步

- 深化结构过滤、BM25 / vector、rerank、repair audit、capacity 和更多 group-memory eval。
