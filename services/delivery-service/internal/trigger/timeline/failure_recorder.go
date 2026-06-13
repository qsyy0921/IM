package timeline

import (
	"context"
	"errors"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
	"google.golang.org/protobuf/proto"
)

type FailureRecorder interface {
	RecordFailure(context.Context, types.ProjectionFailureRecord) error
}

func classifyProjectionFailure(err error) string {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return types.ProjectionFailureClassInvalidArgument
	case errors.Is(err, types.ErrProjectionDependency):
		return types.ProjectionFailureClassProjectionDependency
	case errors.Is(err, types.ErrDBReadFailed):
		return types.ProjectionFailureClassDBRead
	case errors.Is(err, types.ErrDBWriteFailed):
		return types.ProjectionFailureClassDBWrite
	default:
		return types.ProjectionFailureClassUnknown
	}
}

func bestEffortProjectionFailureRecord(consumerGroup string, message types.TimelineMessage, err error) types.ProjectionFailureRecord {
	record := types.ProjectionFailureRecord{
		ConsumerGroup: consumerGroup,
		Topic:         message.Topic,
		PartitionID:   int32(message.Partition),
		OffsetValue:   message.Offset,
		FailureClass:  classifyProjectionFailure(err),
		LastError:     err.Error(),
	}

	var event conversationtimelinev1.ConversationTimelineEvent
	if unmarshalErr := proto.Unmarshal(message.Value, &event); unmarshalErr != nil {
		record.FailureClass = types.ProjectionFailureClassDecode
		return record
	}
	record.EventID = event.GetEventId()
	record.EventType = event.GetEventType()
	record.TenantID = types.TenantID(event.GetTenantId())
	record.ConversationID = types.ConversationID(event.GetAggregateId())
	record.AggregateVersion = event.GetAggregateVersion()
	record.TraceID = event.GetTraceId()
	return record
}
