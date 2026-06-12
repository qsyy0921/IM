# Win/Mac all-service Docker check - 2026-06-12

This report records a small functional Docker check for the current NexusIM service set. It is not a capacity test and not a production HA validation.

## Scope

Windows remained the infrastructure and core process host for this check:

- PostgreSQL: `172.31.50.1:5432`
- Redis: `172.31.50.1:6379`
- Kafka: `172.31.50.1:9092`
- Schema Registry: `172.31.50.1:18081`
- Kafka UI: `172.31.50.1:9090`

Mac used wired Ethernet at `172.31.50.2` and Docker Desktop `linux/arm64` images.

## Mac image sync

`tools/sync-mac-service-docker-images.ps1` now builds and syncs all seven NexusIM service images:

```text
conversation-service
message-service
delivery-service
push-gateway
receipt-service
contacts-service
identity-service
```

The sync was done over the wired `172.31.50.*` link. No external image pull was required for NexusIM service images.

Mac image check after sync:

```text
nexusim/conversation-service:local  arm64
nexusim/message-service:local       arm64
nexusim/delivery-service:local      arm64
nexusim/push-gateway:local          arm64
nexusim/receipt-service:local       arm64
nexusim/contacts-service:local      arm64
nexusim/identity-service:local      arm64
```

## Connectivity

Mac reached the Windows infrastructure ports over the wired link:

```text
172.31.50.1:5432=OK
172.31.50.1:6379=OK
172.31.50.1:9092=OK
172.31.50.1:18081=OK
172.31.50.1:9090=OK
```

## Distributed smoke

Full route result:

```text
result: H:\NexusIM\loadtest-results\push-gateway-win-mac-all-services-check-full-20260612\pushgateway-summary.json
success=true
push_url=ws://172.31.50.2:11598
delivery_outbox_published=2
delivery_outbox_pending=0
delivery_outbox_dlq=0
cursor_last_received_seq=2
```

Cross-instance resume result:

```text
result: H:\NexusIM\loadtest-results\push-gateway-win-mac-all-services-check-resume-20260612\pushgateway-summary.json
success=true
initial_push_url=ws://172.31.50.2:11598
reconnect_push_url=ws://127.0.0.1:11599
delivery_outbox_published=2
delivery_outbox_pending=0
delivery_outbox_dlq=0
cursor_last_received_seq=2
redis_resume_replay_count=1
```

These checks prove the Mac Docker WebSocket gateway can receive Windows-side delivery events through Redis route, and Redis-backed resume can replay a lightweight `delivery.notify` after reconnecting to a different gateway.

## All-service Mac container boot check

Each service image was also started on Mac in its main runtime mode with Windows PostgreSQL or Redis where required. Temporary containers were removed after the check.

Ports verified on Mac:

```text
conversation_port_13096=OK
message_port_13095=OK
delivery_port_13097=OK
receipt_port_13099=OK
contacts_port_13100=OK
identity_port_13106=OK
push_port_13198=OK
contacts_debug_13101=OK
identity_debug_13107=OK
push_debug_13197=OK
```

Startup logs confirmed:

```text
conversation-service gRPC server started
message-service gRPC server started
delivery-service gRPC server started
receipt-service gRPC server started
contacts-service grpc listening
identity-service grpc listening
push-gateway websocket started
```

## Interpretation

This is enough to say:

```text
The current seven NexusIM service images can run as native arm64 Docker containers on Mac, and the existing Win/Mac wired distributed smoke still passes.
```

Do not overstate it as:

```text
All services have production-grade HA, capacity, failover, observability, or security validation.
```

Remaining production work includes Kafka HA, PostgreSQL failover, Redis partition/quorum tests, service discovery, deployment orchestration, mTLS, structured tracing, alerting, and larger capacity tests.
