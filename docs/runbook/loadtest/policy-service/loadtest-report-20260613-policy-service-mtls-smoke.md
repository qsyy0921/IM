# policy-service mTLS Smoke Report

This report records the first-stage static TLS / mTLS smoke for `policy-service` public gRPC.

This is not a full service-mesh rollout, certificate lifecycle system, or dynamic service identity registry. It only proves that a real local `policy-service` process can require a client certificate, enforce an exact-match client DNS SAN allowlist, and still serve `CheckMessageAction` for the four message actions.

## Command

The run used local smoke-only CA / server / client certificates generated under:

```text
H:\NexusIM\loadtest-results\policy-mtls-certs-20260613-192908
```

Command:

```powershell
.\loadtest\policy\run-local-smoke.ps1 `
  -RunName "policy-service-mtls-smoke-20260613-193045" `
  -PolicyGrpcTlsCertFile "H:\NexusIM\loadtest-results\policy-mtls-certs-20260613-192908\policy-server.crt" `
  -PolicyGrpcTlsKeyFile "H:\NexusIM\loadtest-results\policy-mtls-certs-20260613-192908\policy-server.key" `
  -PolicyGrpcTlsClientCaFile "H:\NexusIM\loadtest-results\policy-mtls-certs-20260613-192908\ca.crt" `
  -PolicyGrpcTlsRequireClientCert "true" `
  -PolicyGrpcTlsClientAllowedDnsNames "message-service.nexusim.local" `
  -PolicyTlsCaFile "H:\NexusIM\loadtest-results\policy-mtls-certs-20260613-192908\ca.crt" `
  -PolicyTlsServerName "policy-service.nexusim.local" `
  -PolicyTlsClientCertFile "H:\NexusIM\loadtest-results\policy-mtls-certs-20260613-192908\loadtest-client.crt" `
  -PolicyTlsClientKeyFile "H:\NexusIM\loadtest-results\policy-mtls-certs-20260613-192908\loadtest-client.key"
```

## Baseline

| Item | Value |
| --- | --- |
| Commit | `cf91cb0` |
| Full commit | `cf91cb067676b868de0f0c816ec1c62705633c66` |
| Git dirty | `false` |
| Result dir | `H:\NexusIM\loadtest-results\policy-service-mtls-smoke-20260613-193045` |
| Allow summary | `H:\NexusIM\loadtest-results\policy-service-mtls-smoke-20260613-193045\allow\policy-summary.json` |
| Deny summary | `H:\NexusIM\loadtest-results\policy-service-mtls-smoke-20260613-193045\deny\policy-summary.json` |
| Combined summary | `H:\NexusIM\loadtest-results\policy-service-mtls-smoke-20260613-193045\policy-smoke-summary.json` |
| Server name | `policy-service.nexusim.local` |
| Client allowed DNS SAN | `message-service.nexusim.local` |

## Evidence

Allow scenario:

```json
{
  "success": true,
  "git_dirty": false,
  "policy_tls_enabled": true,
  "expected_allowed": true,
  "expected_permission_version": 31,
  "expected_classification": "CONTACT_ALLOWED",
  "actions": [
    {"action": "SEND", "allowed": true, "permission_version": 31, "classification": "CONTACT_ALLOWED"},
    {"action": "EDIT", "allowed": true, "permission_version": 31, "classification": "CONTACT_ALLOWED"},
    {"action": "REVOKE", "allowed": true, "permission_version": 31, "classification": "CONTACT_ALLOWED"},
    {"action": "DELETE", "allowed": true, "permission_version": 31, "classification": "CONTACT_ALLOWED"}
  ]
}
```

Deny scenario:

```json
{
  "success": true,
  "git_dirty": false,
  "policy_tls_enabled": true,
  "expected_allowed": false,
  "expected_permission_version": 32,
  "expected_classification": "CONTACT_BLOCKED",
  "expected_reason": "blocked by policy smoke",
  "actions": [
    {"action": "SEND", "allowed": false, "permission_version": 32, "classification": "CONTACT_BLOCKED"},
    {"action": "EDIT", "allowed": false, "permission_version": 32, "classification": "CONTACT_BLOCKED"},
    {"action": "REVOKE", "allowed": false, "permission_version": 32, "classification": "CONTACT_BLOCKED"},
    {"action": "DELETE", "allowed": false, "permission_version": 32, "classification": "CONTACT_BLOCKED"}
  ]
}
```

Debug metrics were also read from each process:

```text
allow: grpc.total_requests=4, decisions.total=4, decisions.allowed=4, decisions.denied=0
deny:  grpc.total_requests=4, decisions.total=4, decisions.allowed=0, decisions.denied=4
```

## Conclusion

This smoke proves:

- `policy-service` gRPC server can start with `NEXUSIM_POLICY_GRPC_TLS_CERT_FILE` / `KEY_FILE`.
- `NEXUSIM_POLICY_GRPC_TLS_CLIENT_CA_FILE` plus `REQUIRE_CLIENT_CERT=true` forces mTLS.
- `NEXUSIM_POLICY_GRPC_TLS_CLIENT_ALLOWED_DNS_NAMES=message-service.nexusim.local` accepts the loadtest client certificate only through the configured DNS SAN identity.
- The loadtest client can connect with CA, server name, client cert and client key.
- `CheckMessageAction` still works for `SEND / EDIT / REVOKE / DELETE` in both allow and deny scenarios.
- `/debug/metrics` still reports aggregate gRPC and decision counters under mTLS.

## Boundaries

- This is a direct policy-service gRPC smoke, not a `message-service -> policy-service` mTLS integration smoke.
- It does not prove certificate issuance, rotation, revocation, cross-host distribution, service mesh identity, or all-service mTLS rollout.
- The generated certificates are local smoke material only and are stored under `H:\NexusIM\loadtest-results`.
