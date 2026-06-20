# media-service Stage-Switch Review

Date: 2026-06-20

## Result

`media-service` SDD v0.1 is ready to enter the implementation stage. No P0/P1
blocker was found in the stage-switch review.

This is not the implementation itself. `services/media-service` must still be
created in the next slice together with proto, migration, runtime, Docker and
observability files.

## Why Promotion Is Justified

- Independent data model: media assets, upload sessions, processing jobs,
  access audit, outbox and object-key metadata are not message facts.
- Independent scale profile: binary upload/download, thumbnailing, virus scan
  and transcode jobs scale differently from message send.
- Independent failure boundary: object storage / scanner / transcoder outages
  should not break text message facts or conversation timeline.
- Security boundary: download authorization, object key hygiene, quarantine and
  delete proof require a dedicated owner.
- Complexity reduction: message-service already supports media attachment refs;
  keeping binary lifecycle there would mix send-path facts with object lifecycle.

## Boundary Checks

- message-service keeps only `asset_id` / low-sensitive attachment metadata.
- media-service owns object key, asset metadata, upload session, processing
  status and delete proof.
- conversation / policy visibility must be checked before download URL issuance.
- object keys, presigned URLs, scanner raw errors and provider bodies must not
  enter Kafka events, public responses, metrics or audit exports.
- media tombstone / retention events must be auditable and replayable.

## Gate Impact For Next Slice

The implementation slice is broader than a docs-only change. It must update:

- `docs/runbook/service-registry.json`: add an active stage strategy for
  product services and switch `media-service` out of `future`.
- `tools/check-future-service-boundary.ps1`: allow the chosen active product
  stage to own a service directory without weakening future-service protection.
- `api/proto/nexusim/media/v1/media_service.proto`.
- `migrations/postgres/media/000001_media_core.sql`.
- `services/media-service` six-layer skeleton and `cmd/media-service`.
- Docker runtime, local compose, Prometheus rules and Grafana dashboard.

Because this touches registry, proto, migration, Docker/compose and public API,
the implementation slice should run the expanded gate set or full `check-local`.

## First Implementation Scope

Keep the first code slice narrow:

```text
CreateUploadSession
CompleteUpload
GetMediaAsset
GetMediaDownloadURL
DeleteMediaAsset
```

Use fake object storage / scanner / thumbnail / transcode adapters first. Real
S3-compatible storage and provider-grade scanning are later hardening.

## Focused Acceptance For First Smoke

- upload session create is idempotent.
- complete upload verifies size and sha256 from object storage metadata.
- download URL requires visibility / policy checks and never returns object key.
- quarantined / deleted / not-ready assets fail closed.
- media outbox events do not contain object key, URL, scanner body or raw error.
- malformed processing payloads do not publish false ready events.
