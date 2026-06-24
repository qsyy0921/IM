# retrieval-gateway Brief

状态：foundation-active / EvidencePack boundary。

## 已落

- 聚合 search / memory / vector / policy precheck，输出统一 EvidencePack。
- Memory graph depth 0..3、profile aggregate evidence、actor attribution、source-chain
  signal 和 RRF-style lane fusion。
- Vector-index `SearchVectors` opt-in adapter first path；只消费 refs / hash /
  visibility metadata，不传 raw text 或 embedding vector。
- Retrieval positive / negative / miss / source coverage / provider readiness checks。

## 边界

- 不调用 LLM，不生成 answer / proposal。
- 不直接读业务私表；只走服务公开 API / port。
- 缺失 policy、malformed vector result、provider dependency error 必须 fail-closed。
- External candidate backend 只召回候选，最终 visibility / tombstone 仍由 owned projection 过滤。

## 下一步

- 真实 OpenSearch、pgvector、Milvus / OpenSearch vector provider smoke 和 coverage 深化。
