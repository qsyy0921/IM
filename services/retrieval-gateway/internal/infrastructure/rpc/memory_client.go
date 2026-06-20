package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type MemoryClient struct {
	client  memoryv1.MemoryServiceClient
	timeout time.Duration
}

func NewMemoryClient(client memoryv1.MemoryServiceClient, timeout time.Duration) MemoryClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return MemoryClient{client: client, timeout: timeout}
}

func DialMemoryClient(_ context.Context, addr string, timeout time.Duration) (MemoryClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return MemoryClient{}, nil, errors.New("memory-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return MemoryClient{}, nil, err
	}
	return NewMemoryClient(memoryv1.NewMemoryServiceClient(conn), timeout), conn.Close, nil
}

func (client MemoryClient) QueryMemoryEvents(ctx context.Context, query types.MemoryQuery) (types.MemoryResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, query.AuthContext)
	response, err := client.client.QueryMemoryEvents(callCtx, &memoryv1.QueryMemoryEventsRequest{
		AuthContext: &memoryv1.AuthContext{
			TenantId:  string(query.AuthContext.TenantID),
			UserId:    string(query.AuthContext.UserID),
			DeviceId:  query.AuthContext.DeviceID,
			SessionId: query.AuthContext.SessionID,
			TraceId:   query.AuthContext.TraceID,
			RequestId: query.AuthContext.RequestID,
		},
		Scope:             memoryv1.MemoryScope_MEMORY_SCOPE_CONVERSATION,
		ScopeId:           string(query.ConversationID),
		ConversationId:    string(query.ConversationID),
		Query:             query.Query,
		Statuses:          memoryStatusesToProto(query.Statuses),
		AfterValidFromSeq: query.AfterValidFromSeq,
		AtConversationSeq: query.AtConversationSeq,
		Limit:             int32(query.Limit),
	})
	if err != nil {
		return types.MemoryResult{}, mapMemoryError(err)
	}
	items := make([]types.MemoryEventEvidence, 0, len(response.GetItems()))
	for _, item := range response.GetItems() {
		items = append(items, memoryEventFromProto(item))
	}
	return types.MemoryResult{
		Items:             items,
		ProjectionVersion: response.GetProjectionVersion(),
	}, nil
}

func memoryEventFromProto(item *memoryv1.StructuredMemoryEvent) types.MemoryEventEvidence {
	sourceRefs := make([]types.EvidenceSourceRef, 0, len(item.GetSourceRefs()))
	for _, ref := range item.GetSourceRefs() {
		sourceRefs = append(sourceRefs, types.EvidenceSourceRef{
			SourceType:      sourceTypeFromProto(ref.GetSourceType()),
			SourceID:        ref.GetSourceId(),
			SourceEventID:   ref.GetSourceEventId(),
			ConversationID:  types.ConversationID(ref.GetConversationId()),
			ConversationSeq: ref.GetConversationSeq(),
			OccurredAt:      unixMillisToTime(ref.GetOccurredAtUnixMs()),
		})
	}
	return types.MemoryEventEvidence{
		MemoryEventID:     item.GetMemoryEventId(),
		ConversationID:    types.ConversationID(item.GetConversationId()),
		Topic:             item.GetTopic(),
		Status:            memoryStatusFromProto(item.GetStatus()),
		ReviewState:       reviewStateFromProto(item.GetReviewState()),
		FactText:          item.GetFactText(),
		ActorUserIDs:      item.GetActorUserIds(),
		AudienceUserIDs:   item.GetAudienceUserIds(),
		SourceRefs:        sourceRefs,
		ValidFromSeq:      item.GetValidFromSeq(),
		ValidToSeq:        item.GetValidToSeq(),
		ValidFromAt:       unixMillisToTime(item.GetValidFromUnixMs()),
		Confidence:        item.GetConfidence(),
		VisibilityVersion: item.GetVisibilityVersion(),
		ExtractionVersion: item.GetExtractionVersion(),
	}
}

func memoryStatusesToProto(statuses []string) []memoryv1.MemoryEventStatus {
	out := make([]memoryv1.MemoryEventStatus, 0, len(statuses))
	for _, status := range statuses {
		switch status {
		case types.MemoryStatusPending:
			out = append(out, memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_PENDING)
		case types.MemoryStatusActive:
			out = append(out, memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE)
		case types.MemoryStatusSuperseded:
			out = append(out, memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_SUPERSEDED)
		case types.MemoryStatusArchived:
			out = append(out, memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ARCHIVED)
		}
	}
	return out
}

func memoryStatusFromProto(status memoryv1.MemoryEventStatus) string {
	switch status {
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_PENDING:
		return types.MemoryStatusPending
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE:
		return types.MemoryStatusActive
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_SUPERSEDED:
		return types.MemoryStatusSuperseded
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ARCHIVED:
		return types.MemoryStatusArchived
	default:
		return ""
	}
}

func reviewStateFromProto(state memoryv1.MemoryReviewState) string {
	switch state {
	case memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_UNREVIEWED:
		return "UNREVIEWED"
	case memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_NEEDS_REVIEW:
		return "NEEDS_REVIEW"
	case memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_APPROVED:
		return "APPROVED"
	case memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_REJECTED:
		return "REJECTED"
	default:
		return ""
	}
}

func sourceTypeFromProto(sourceType memoryv1.MemorySourceType) string {
	switch sourceType {
	case memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_MESSAGE:
		return "MESSAGE"
	case memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_TIMELINE_EVENT:
		return "TIMELINE_EVENT"
	case memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_PROFILE_AGGREGATE:
		return "PROFILE_AGGREGATE"
	case memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_SYSTEM:
		return "SYSTEM"
	default:
		return ""
	}
}
