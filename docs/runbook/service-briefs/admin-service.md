# admin-service

状态：future / SDD v0.1 draft 已存在。当前不得创建 `services/admin-service`
目录，直到完成 stage switch。

定位：管理后台 API，负责租户管理、封禁、配置操作、repair 审批、运维操作和
operator workflow 入口。

边界：

- admin-service 不直接写其他服务私有表，必须通过公开 API / operator command。
- 高风险操作必须走 policy precheck、approval、audit 和幂等键。
- 不承载普通用户 IM 流量，不替代 api-gateway 的客户端入口。
- 管理操作默认最小权限、可追溯、可撤销或可补偿。

第一切片建议：

- 具体边界见 `docs/sdd/admin-service.md`。
- 统一 operator approval manifest 和 repair request API。
- 对接 identity / policy / contacts 的现有 operator。
- 输出低敏 audit event，后续归档到 audit-service。
