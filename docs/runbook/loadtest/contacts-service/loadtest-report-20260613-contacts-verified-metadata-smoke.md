# contacts-service verified metadata auth smoke

Date: 2026-06-13

## Conclusion

This smoke verifies that `contacts-service` can run its core accept-flow user RPCs with gateway verified identity metadata:

```text
SendContactRequest(metadata auth)
-> ListContactRequests(INCOMING, PENDING, metadata auth)
-> RespondContactRequest(ACCEPT, metadata auth)
-> ListContactRequests(INCOMING, ACCEPTED, metadata auth)
-> ListContacts / GetContactState(metadata auth)
-> contacts_outbox
-> contacts-service outbox-relay
-> Kafka im.contact.events
```

The result proves the first-stage metadata-auth entrypoint for contacts-service. It is not a complete API gateway, not an mTLS rollout, and not a capacity result.

## Command

```powershell
.\loadtest\contacts\run-local-smoke.ps1 `
  -VerifiedAuthMetadata `
  -RunName "contacts-verified-metadata-smoke-20260613-185057"
```

## Raw Result

```text
H:\NexusIM\loadtest-results\contacts-verified-metadata-smoke-20260613-185057\contacts-summary.json
```

## Baseline

```text
commit=6ebb8ff
git_dirty=false
scenario=accept
verified_auth_metadata=true
tls_enabled=false
target=127.0.0.1:50829
tenant_id=tenant-contacts-accept-smoke-20260613-185057
contact_topic=im.contact.events.contacts-accept-smoke.20260613-185057
```

## Key Evidence

Request lifecycle:

```text
SendContactRequest.status=CONTACT_REQUEST_STATUS_PENDING
RespondContactRequest.status=CONTACT_REQUEST_STATUS_ACCEPTED
receiver_incoming_pending_before_respond.request_count=1
receiver_incoming_pending_after_respond.request_count=0
receiver_incoming_terminal_after_respond.request_count=1
```

Contact read model:

```text
sender_list.contact_count=1
sender_list.contact_user_ids=[contacts-receiver]
receiver_list.contact_count=1
receiver_list.contact_user_ids=[contacts-sender]
sender_state.status=CONTACT_EDGE_STATUS_ACTIVE
receiver_state.status=CONTACT_EDGE_STATUS_ACTIVE
sender_state.version=1
receiver_state.version=1
```

Outbox and Kafka:

```text
contacts_outbox.total=2
contacts_outbox.published=2
contacts_outbox.pending=0
contacts_outbox.dlq=0
contact_kafka_events[0].event_type=contact.request.created.v1
contact_kafka_events[1].event_type=contact.request.accepted.v1
aggregate_version=1 -> 2
```

## Boundary

- `NEXUSIM_CONTACTS_AUTH_MODE=metadata` makes contacts-service derive caller identity from gateway verified gRPC metadata.
- Request body identity fields remain only for the default body-auth compatibility mode.
- This smoke does not validate certificate issuance, certificate rotation, dynamic service identity, full API gateway policy, or multi-tenant production auth governance.
- contacts-service still owns only contact facts. It does not write `conversation_members`, does not create direct conversations, and does not require message-service to synchronously depend on contacts-service.
