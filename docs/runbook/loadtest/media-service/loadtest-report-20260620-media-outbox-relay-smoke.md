# media-service outbox relay smoke - 2026-06-20

## Scope

This smoke verifies the first real-process media event publish path:

```text
CreateUploadSession -> CompleteUpload -> media_outbox
-> media-service outbox-relay -> Kafka im.media.events
-> typed MediaEvent readback
```

It also keeps the existing public response safety checks: object keys must not
appear in upload URL, download URL, or public asset response.

This is still a small local smoke. It does not validate real S3-compatible
storage, scanner provider, thumbnail/transcode provider, Kafka HA, capacity, or
provider-grade download policy.

## Command

```powershell
.\loadtest\media\run-local-smoke.ps1 -WithOutboxRelay
```

Raw summary:

```text
H:\NexusIM\loadtest-results\media-service-outbox-relay-smoke-20260620-183255\media-grpc-summary.json
```

## Environment

| Field | Value |
| --- | --- |
| Commit | `020376a9` |
| Full commit | `020376a9470e7deaf7c35eff24943318a9474a07` |
| Git dirty | `false` |
| PostgreSQL | `nexusim-postgres` local Docker container |
| Kafka | `nexusim-kafka` local Docker container |
| Media target | `127.0.0.1:65509` |
| Kafka brokers | `localhost:9092` |
| Kafka topic | `im.media.events.media-service-outbox-relay-smoke-20260620-183255` |

## Result

| Check | Result |
| --- | --- |
| Overall success | `true` |
| Asset status | `READY` |
| Scan status | `PASSED` |
| Thumbnail status | `SKIPPED` |
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
| `media.asset.uploaded.v1` | `asset_dc724acdcf96bf55bc94c10eff120013-uploaded-v1` | `asset_uploaded` | `UPLOADED` |
| `media.asset.ready.v1` | `asset_dc724acdcf96bf55bc94c10eff120013-ready-v1` | `asset_ready` | `READY` |

## Conclusion

`media_outbox -> im.media.events` is now verified by a real local process smoke:
the relay published both expected media events, marked both outbox rows
`PUBLISHED`, and the runner read back typed protobuf `MediaEvent` records from
Kafka.

Next media-service work should move to the processing worker path
(`scanner/thumbnail/transcode` as mock adapters first) or switch to the next
product service stage-switch.
