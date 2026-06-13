# identity-service api-gateway facade smoke - 2026-06-14

## 结论

本轮 smoke 证明 identity-service 的基础 public / self-service 链路已经可以通过 `api-gateway` 的 `nexusim.gateway.v1.GatewayService` facade 完成，而不是客户端直连 `identity-service`。

链路：

```text
loadtest/identity
-> api-gateway GatewayService facade
-> identity-service RegisterUser
-> RequestVerificationChallenge(outbox mode)
-> identity_challenge_delivery_outbox(PENDING)
-> challenge-delivery-worker
-> local webhook receives raw token
-> ConfirmVerificationChallenge via GatewayService facade
-> identity_challenges(CONSUMED, delivery_status=DELIVERED)
-> Login via GatewayService facade
-> RefreshGatewayToken via GatewayService facade
```

这仍是本地小规模 smoke，不是容量测试，也不是完整生产 API gateway 部署。传输层本轮使用 plaintext；验证重点是 identity public facade、correlation 透传、challenge delivery outbox、worker webhook delivery、Login token issuing 和 refresh token rotation。

## 原始结果

- Result dir: `H:\NexusIM\loadtest-results\identity-api-gateway-login-refresh-smoke-20260614-clean`
- Summary: `H:\NexusIM\loadtest-results\identity-api-gateway-login-refresh-smoke-20260614-clean\identity-summary.json`
- Code commit in summary: `d07373a`
- `git_dirty`: `false`
- Target: `127.0.0.1:49479` (api-gateway)
- `gateway_facade`: `true`

## 关键证据

Summary:

```text
success=true
gateway_facade=true
register_user.status=USER_STATUS_ACTIVE
request_verification_challenge.channel=VERIFICATION_CHANNEL_EMAIL
request_verification_challenge.dev_challenge_token_set=false
webhook.received=true
webhook.token_set=true
webhook.authorization_ok=true
confirm_verification_challenge.channel=VERIFICATION_CHANNEL_EMAIL
login.audience=api-gateway
login.token_type=Bearer
login.session_id_set=true
login.gateway_token_set=true
login.refresh_token_set=true
refresh_gateway_token.audience=api-gateway
refresh_gateway_token.token_type=Bearer
refresh_gateway_token.session_id_set=true
refresh_gateway_token.gateway_token_set=true
refresh_gateway_token.refresh_token_set=true
refresh_gateway_token.refresh_token_rotated=true
challenge_delivery_outbox.total=1
challenge_delivery_outbox.delivered=1
challenge_delivery_outbox.pending=0
challenge_delivery_outbox.dlq=0
challenge_delivery_outbox_row.status=DELIVERED
challenge_delivery_outbox_row.retry_count=0
challenge_row.status=CONSUMED
challenge_row.delivery_status=DELIVERED
challenge_row.delivery_attempt_count=1
```

api-gateway access log shows the client-facing identity calls used the facade descriptor:

```text
/nexusim.gateway.v1.GatewayService/RegisterUser OK trace_id=identity-challenge-delivery-outbox-smoke request_id=identity-smoke-register
/nexusim.gateway.v1.GatewayService/RequestVerificationChallenge OK trace_id=identity-challenge-delivery-outbox-smoke request_id=identity-smoke-request-challenge
/nexusim.gateway.v1.GatewayService/ConfirmVerificationChallenge OK trace_id=identity-challenge-delivery-outbox-smoke request_id=identity-smoke-confirm-challenge
/nexusim.gateway.v1.GatewayService/Login OK trace_id=identity-challenge-delivery-outbox-smoke request_id=identity-smoke-login
/nexusim.gateway.v1.GatewayService/RefreshGatewayToken OK trace_id=identity-challenge-delivery-outbox-smoke request_id=identity-smoke-refresh
```

## 边界

- api-gateway 不拥有 identity facts，不写 `identity_users / identity_challenges / identity_challenge_delivery_outbox`。
- 本轮覆盖 `RegisterUser -> RequestVerificationChallenge -> ConfirmVerificationChallenge -> Login -> RefreshGatewayToken`；password reset / MFA facade methods 已有代码接入，后续可按需要补专门 smoke。
- identity facade 是 pre-auth public / self-service 转发入口，不暴露 `IssueGatewayToken / RevokeDevice / RevokeSession / GetDeviceState`。
- 本轮没有覆盖 api-gateway 到 identity-service 的 TLS / mTLS；下游 TLS 配置能力仍保留在 api-gateway 和 identity-service。
- challenge token 仍不落库；Confirm 使用的是 webhook worker 收到的 token，不是 dev token。
- summary 只记录 token 是否返回和 refresh 是否轮换，不写 gateway token 或 refresh token 明文。
