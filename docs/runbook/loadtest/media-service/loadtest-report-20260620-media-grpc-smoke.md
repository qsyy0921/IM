# media-service gRPC smoke report - 2026-06-20

## Scope

This was a local, small-scale `media-service` gRPC smoke. It validates the first
fake object-store path only:

```text
CreateUploadSession -> CompleteUpload -> GetMediaAsset -> GetMediaDownloadURL
```

It does not validate real S3-compatible storage, scanner, thumbnail / transcode
workers, `im.media.events` publishing, provider-grade download policy, capacity
or production SLOs.

## Environment

- Commit: `f61da25e feat: add media service grpc smoke runner`
- `git_dirty`: `false`
- Result dir: `H:\NexusIM\loadtest-results\media-service-grpc-smoke-20260620-clean`
- gRPC target: `127.0.0.1:59961`
- TLS: disabled
- PostgreSQL DSN: local NexusIM development database

## Result

Passed.

Key summary:

```text
asset_status = READY
scan_status = PASSED
thumbnail_status = SKIPPED
transcode_status = SKIPPED
upload_url_safe = true
download_url_safe = true
public_asset_safe = true
outbox.uploaded = 1
outbox.ready = 1
outbox.pending = 2
outbox.dlq = 0
access_audit_allowed = 1
```

## Verified Invariants

- `CreateUploadSession` returns an upload session and a fake presigned PUT URL.
- `CompleteUpload` verifies fake object metadata and moves the asset to `READY`.
- `GetMediaAsset` returns public media metadata without `object_key`.
- `GetMediaDownloadURL` writes one allowed access audit row and returns a safe
  fake presigned GET URL.
- `media.asset.uploaded.v1` and `media.asset.ready.v1` rows are written to
  `media_outbox`.
- The runner rejects public responses, fake presigned URLs and outbox payloads
  that expose `object_key`, raw object key values or `download_url` in event
  payloads.

## Next

- Implement `media_outbox -> im.media.events` relay and event schema smoke.
- Add processing worker slices for scan / thumbnail / transcode before replacing
  fake providers with real adapters.
