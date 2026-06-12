# Mac container full smoke - 2026-06-12

This report records a small functional smoke for the `nexusim-mac-*` Docker container set on Mac. It is not a capacity test and not a production HA validation.

## Scope

Mac host:

```text
host: 172.31.50.2
runtime: Docker Desktop
containers: 22 nexusim-mac-* containers
```

The test used Mac-local infrastructure containers:

```text
nexusim-mac-postgres
nexusim-mac-redis
nexusim-mac-kafka
nexusim-mac-schema-registry
nexusim-mac-kafka-ui
```

PostgreSQL schema was initialized from the repository migrations. Kafka topics were created:

```text
conversation.timeline.events
im.delivery.events
im.receipt.events
im.contact.events
im.identity.events
```

## Container startup

All 22 containers were started for the smoke.

Readiness checks passed on Mac-local ports:

```text
conversation_grpc=12096 OK
message_grpc=12095 OK
message_debug=12094 OK
delivery_grpc=12097 OK
receipt_grpc=12099 OK
contacts_grpc=12100 OK
contacts_debug=12101 OK
identity_grpc=12106 OK
identity_debug=12107 OK
push_all_ws=12098 OK
push_all_debug=12093 OK
push_ws=12198 OK
push_ws_debug=12193 OK
push_consumer_debug=12293 OK
```

Container status after startup:

```text
running_count=22
exited_count=0
```

Windows reached the same Mac ports over wired `172.31.50.2`, including PostgreSQL, Redis, Kafka, Kafka UI, Schema Registry, service gRPC ports, and push-gateway WebSocket ports.

## Full smoke

Runner:

```powershell
go run ./loadtest/pushgateway `
  -scenario full `
  -result-dir H:\NexusIM\loadtest-results\mac-container-full-smoke-20260612 `
  -pg-dsn "postgres://nexusim:nexusim@172.31.50.2:15432/nexusim?sslmode=disable" `
  -conversation-target 172.31.50.2:12096 `
  -message-target 172.31.50.2:12095 `
  -delivery-target 172.31.50.2:12097 `
  -push-url ws://172.31.50.2:12098 `
  -push-metrics-url http://172.31.50.2:12093/debug/metrics `
  -route-backend redis
```

Result:

```text
summary: H:\NexusIM\loadtest-results\mac-container-full-smoke-20260612\pushgateway-summary.json
commit=27b7c0e
git_dirty=false
success=true
```

Functional evidence:

```text
CreateMemberChange JOIN boundary_seq=1 permission_version=2
SendMessage conversation_seq=2
WebSocket delivery.notify source_event_type=message.persisted.v1
PullInbox item_count=1 max_seq=2
AckDelivery delivery.ack.ok last_received_seq=2
device_delivery_cursors.last_received_seq=2
delivery_outbox_total=2
delivery_outbox_published=2
delivery_outbox_pending=0
delivery_outbox_dlq=0
```

This proves the Mac-local Docker set can run the current minimal IM chain:

```text
conversation-service
-> message-service
-> message outbox relay
-> Kafka conversation.timeline.events
-> delivery-service timeline consumer
-> delivery-service PullInbox/AckDelivery
-> delivery outbox relay
-> Kafka im.delivery.events
-> push-gateway WebSocket notify
```

## Cleanup

All Mac containers were stopped after the smoke:

```text
running=0
exited=22
```

Containers and volumes were preserved for later reuse.

## Limits

This smoke proves startup and one happy-path message flow on one Mac Docker Desktop instance. It does not prove:

```text
production HA
capacity limits
multi-broker Kafka
PostgreSQL failover
Redis quorum or network partition behavior
long-running stability
real authentication policy
```
