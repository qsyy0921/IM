package types

import "strings"

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

type ProjectionFailureResolveOptions struct {
	ConsumerGroup string
	Topic         string
	PartitionID   int32
	OffsetValue   int64
	Operator      string
	Reason        string
	DryRun        bool
}

type ProjectionFailureResolveStats struct {
	Requested int
	Audited   int
	Resolved  int
}

const (
	ProjectionFailureClassDecode               = "decode_failed"
	ProjectionFailureClassInvalidArgument      = "invalid_argument"
	ProjectionFailureClassProjectionDependency = "projection_dependency"
	ProjectionFailureClassDBRead               = "db_read_failed"
	ProjectionFailureClassDBWrite              = "db_write_failed"
	ProjectionFailureClassUnknown              = "unknown"
)

func ProjectionFailurePublicMessage(failureClass string) string {
	switch strings.TrimSpace(failureClass) {
	case ProjectionFailureClassDecode:
		return "delivery projection decode failed"
	case ProjectionFailureClassInvalidArgument:
		return "delivery projection invalid event"
	case ProjectionFailureClassProjectionDependency:
		return "delivery projection dependency failed"
	case ProjectionFailureClassDBRead:
		return "delivery projection read failed"
	case ProjectionFailureClassDBWrite:
		return "delivery projection write failed"
	default:
		return "delivery projection failed"
	}
}
