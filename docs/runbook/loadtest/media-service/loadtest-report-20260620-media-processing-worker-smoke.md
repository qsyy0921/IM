# media-service processing worker smoke - 2026-06-20

## Scope

This smoke verifies the first real media processing worker path with local mock
processors:

```text
CreateUploadSession -> CompleteUpload
-> media_processing_jobs(SCAN, THUMBNAIL)
-> media-service processing-worker
-> asset READY
-> GetMediaDownloadURL
-> media_outbox -> outbox-relay -> Kafka im.media.events
```

The worker uses mock scanner / thumbnail / transcode adapters. This smoke does
not validate real object storage, external scanner, thumbnail provider,
transcode provider, capacity, Kafka HA, or provider-grade download policy.

## Command

```powershell
.\loadtest\media\run-local-smoke.ps1 -WithOutboxRelay
```

Raw summary:

```text
H:\NexusIM\loadtest-results\media-service-outbox-relay-smoke-20260620-184930\media-grpc-summary.json
```

## Environment

| Field | Value |
| --- | --- |
| Commit | `c53af680` |
| Full commit | `c53af680513a580c90e87ae063141010b1fa9385` |
| Git dirty | `false` |
| PostgreSQL | `nexusim-postgres` local Docker container |
| Kafka | `nexusim-kafka` local Docker container |
| Media target | `127.0.0.1:50951` |
| Kafka brokers | `localhost:9092` |
| Kafka topic | `im.media.events.media-service-outbox-relay-smoke-20260620-184930` |

## Result

| Check | Result |
| --- | --- |
| Overall success | `true` |
| Asset status | `READY` |
| Scan status | `PASSED` |
| Thumbnail status | `PASSED` |
| Transcode status | `SKIPPED` |
| Upload URL safe | `true` |
| Download URL safe | `true` |
| Public asset safe | `true` |
| Access audit allowed rows | `1` |

## Outbox / Kafka

| Metric | Value |
| --- | --- |
| `media_outbox total` | `2` |
| `media_outbox PUBLISHED` | `2` |
| `media_outbox PENDING` | `0` |
| `media_outbox DLQ` | `0` |
| Kafka `MediaEvent` read count | `2` |

Kafka events read back from the smoke topic:

| Event type | Event id | Payload kind | Status |
| --- | --- | --- | --- |
| `media.asset.uploaded.v1` | `asset_78b71c2989660690f17a5eda2c9db2b3-uploaded-v1` | `asset_uploaded` | `UPLOADED` |
| `media.asset.ready.v1` | `asset_78b71c2989660690f17a5eda2c9db2b3-ready-v1` | `asset_ready` | `READY` |

## Conclusion

The first media processing worker slice is verified by a real local process
smoke. `CompleteUpload` now enters `PROCESSING`, the worker completes mock
`SCAN` and `THUMBNAIL` jobs, the asset becomes `READY`, and the ready event is
published through the media outbox relay.

Next work can move to `notification-service` stage switch / implementation.
Real S3, scanner, thumbnail, and transcode providers remain future hardening.
