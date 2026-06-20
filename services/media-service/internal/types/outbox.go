package types

import "time"

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusDLQ       = "DLQ"

	MediaEventAssetUploaded    = "media.asset.uploaded.v1"
	MediaEventAssetReady       = "media.asset.ready.v1"
	MediaEventAssetDeleted     = "media.asset.deleted.v1"
	MediaEventAssetQuarantined = "media.asset.quarantined.v1"
)

type OutboxMessage struct {
	ID               int64
	EventID          string
	TenantID         TenantID
	AssetID          string
	EventType        string
	EventVersion     int32
	PartitionKey     string
	Producer         string
	PayloadJSON      []byte
	RetryCount       int
	OccurredAt       time.Time
	TraceID          string
	CorrelationID    string
	CausationID      string
	AggregateVersion int64
}

type OutboxRelayStats struct {
	Fetched      int
	Published    int
	Retried      int
	DeadLettered int
}

type KafkaPublishRecord struct {
	Key   []byte
	Value []byte
}
