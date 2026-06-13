package types

type ProjectionFailureRecord struct {
	ConsumerGroup    string
	Topic            string
	PartitionID      int32
	OffsetValue      int64
	EventID          string
	EventType        string
	TenantID         TenantID
	ConversationID   ConversationID
	AggregateVersion int64
	TraceID          string
	FailureClass     string
	LastError        string
}

const (
	ProjectionFailureClassDecode               = "decode_failed"
	ProjectionFailureClassInvalidArgument      = "invalid_argument"
	ProjectionFailureClassProjectionDependency = "projection_dependency"
	ProjectionFailureClassDBRead               = "db_read_failed"
	ProjectionFailureClassDBWrite              = "db_write_failed"
	ProjectionFailureClassUnknown              = "unknown"
)
