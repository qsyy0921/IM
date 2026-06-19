# agent-service

状态：foundation-active / proposal-only path + real adapter smoke passed.

定位：受控 Agent proposal 边界服务。它只消费 retrieval-gateway 返回的
`EvidencePack`，并通过 policy-service `CheckToolAction` 做 tool action
precheck；第一版不执行业务动作，不直接读 message / conversation / search /
memory / policy 私有表。

当前已落：

- `agent_service.proto`、SDD、六层 skeleton、`grpc` runtime mode、debug
  `/metrics`、registry / Docker / compose / Prometheus / Grafana wiring
- app usecase 先调用 policy port；policy deny 时返回 `BLOCKED` 且不检索证据
- policy allow 后调用 retrieval port 获取 EvidencePack
- 默认本地 extractive provider 生成 deterministic proposal，保留 citations /
  EvidencePack、`generated_by_llm=false`
- provider 输出后统一运行 citation verifier，无法匹配 EvidencePack 则
  fail closed
- 第一版所有 proposed response 都保持 proposal-only，不执行工具动作
- 真实本地 `retrieval-gateway -> policy-service -> agent-service` adapter
  smoke 已通过，验证 tool policy allow、EvidencePack、citation 和
  proposal-only 边界

下一步：

- 接 `skill-registry`，登记可被 Agent 调用的技能、输入输出合约、权限和
  审计元数据。
- 后续再接 `mcp-gateway` 和 `action-executor`。
- 外部 LLM adapter 后续仍必须走 ProposalProvider port 和 citation verifier。
