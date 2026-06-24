package grpc

import (
	"context"
	"errors"
	"time"

	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	"github.com/qsyy0921/IM/services/memory-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type QueryMemoryEventsExecutor interface {
	Execute(context.Context, types.QueryMemoryEventsCommand) (types.QueryMemoryEventsResult, error)
}

type GetMemoryEventExecutor interface {
	Execute(context.Context, types.GetMemoryEventCommand) (types.GetMemoryEventResult, error)
}

type ListProfileAggregatesExecutor interface {
	Execute(context.Context, types.ListProfileAggregatesCommand) (types.ListProfileAggregatesResult, error)
}

type RecomputeProfileAggregateExecutor interface {
	Execute(context.Context, types.RecomputeProfileAggregateCommand) (types.RecomputeProfileAggregateResult, error)
}

type SubmitMemoryCandidateExecutor interface {
	Execute(context.Context, types.SubmitMemoryCandidateCommand) (types.SubmitMemoryCandidateResult, error)
}

type ReviewMemoryCandidateExecutor interface {
	Execute(context.Context, types.ReviewMemoryCandidateCommand) (types.ReviewMemoryCandidateResult, error)
}

type Server struct {
	memoryv1.UnimplementedMemoryServiceServer
	queryMemoryEvents         QueryMemoryEventsExecutor
	getMemoryEvent            GetMemoryEventExecutor
	listProfileAggregates     ListProfileAggregatesExecutor
	recomputeProfileAggregate RecomputeProfileAggregateExecutor
	submitMemoryCandidate     SubmitMemoryCandidateExecutor
	reviewMemoryCandidate     ReviewMemoryCandidateExecutor
}

func NewServer(query QueryMemoryEventsExecutor, get GetMemoryEventExecutor, list ListProfileAggregatesExecutor, recompute RecomputeProfileAggregateExecutor, submit SubmitMemoryCandidateExecutor, review ReviewMemoryCandidateExecutor) *Server {
	return &Server{
		queryMemoryEvents:         query,
		getMemoryEvent:            get,
		listProfileAggregates:     list,
		recomputeProfileAggregate: recompute,
		submitMemoryCandidate:     submit,
		reviewMemoryCandidate:     review,
	}
}

func Register(server *grpcgo.Server, service *Server) {
	memoryv1.RegisterMemoryServiceServer(server, service)
}

func (server *Server) QueryMemoryEvents(ctx context.Context, request *memoryv1.QueryMemoryEventsRequest) (*memoryv1.QueryMemoryEventsResponse, error) {
	if server == nil || server.queryMemoryEvents == nil {
		return nil, publicError(types.ErrMemoryUnavailable)
	}
	command := types.QueryMemoryEventsCommand{
		AuthContext:       authContext(ctx, request.GetAuthContext()),
		Scope:             scopeFromProto(request.GetScope()),
		ScopeID:           request.GetScopeId(),
		ConversationID:    types.ConversationID(request.GetConversationId()),
		ActorUserID:       types.UserID(request.GetActorUserId()),
		Topic:             request.GetTopic(),
		Query:             request.GetQuery(),
		Statuses:          statusesFromProto(request.GetStatuses()),
		AfterValidFromSeq: request.GetAfterValidFromSeq(),
		AtConversationSeq: request.GetAtConversationSeq(),
		Limit:             int(request.GetLimit()),
	}
	result, err := server.queryMemoryEvents.Execute(ctx, command)
	if err != nil {
		return nil, publicError(err)
	}
	items := make([]*memoryv1.StructuredMemoryEvent, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, structuredMemoryEventToProto(item))
	}
	return &memoryv1.QueryMemoryEventsResponse{
		Items:             items,
		NextCursor:        result.NextCursor,
		ProjectionVersion: result.ProjectionVersion,
	}, nil
}

func (server *Server) GetMemoryEvent(ctx context.Context, request *memoryv1.GetMemoryEventRequest) (*memoryv1.GetMemoryEventResponse, error) {
	if server == nil || server.getMemoryEvent == nil {
		return nil, publicError(types.ErrMemoryUnavailable)
	}
	result, err := server.getMemoryEvent.Execute(ctx, types.GetMemoryEventCommand{
		AuthContext:   authContext(ctx, request.GetAuthContext()),
		MemoryEventID: request.GetMemoryEventId(),
	})
	if err != nil {
		return nil, publicError(err)
	}
	edges := make([]*memoryv1.MemoryGraphEdge, 0, len(result.GraphEdges))
	for _, edge := range result.GraphEdges {
		edges = append(edges, memoryGraphEdgeToProto(edge))
	}
	return &memoryv1.GetMemoryEventResponse{
		Item:       structuredMemoryEventToProto(result.Item),
		GraphEdges: edges,
	}, nil
}

func (server *Server) ListProfileAggregates(ctx context.Context, request *memoryv1.ListProfileAggregatesRequest) (*memoryv1.ListProfileAggregatesResponse, error) {
	if server == nil || server.listProfileAggregates == nil {
		return nil, publicError(types.ErrMemoryUnavailable)
	}
	result, err := server.listProfileAggregates.Execute(ctx, types.ListProfileAggregatesCommand{
		AuthContext:   authContext(ctx, request.GetAuthContext()),
		SubjectUserID: types.UserID(request.GetSubjectUserId()),
		AggregateType: request.GetAggregateType(),
		Statuses:      statusesFromProto(request.GetStatuses()),
		Limit:         int(request.GetLimit()),
	})
	if err != nil {
		return nil, publicError(err)
	}
	items := make([]*memoryv1.ProfileAggregate, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, profileAggregateToProto(item))
	}
	return &memoryv1.ListProfileAggregatesResponse{
		Items:      items,
		NextCursor: result.NextCursor,
	}, nil
}

func (server *Server) RecomputeProfileAggregate(ctx context.Context, request *memoryv1.RecomputeProfileAggregateRequest) (*memoryv1.RecomputeProfileAggregateResponse, error) {
	if server == nil || server.recomputeProfileAggregate == nil {
		return nil, publicError(types.ErrMemoryUnavailable)
	}
	result, err := server.recomputeProfileAggregate.Execute(ctx, types.RecomputeProfileAggregateCommand{
		AuthContext:     authContext(ctx, request.GetAuthContext()),
		SubjectUserID:   types.UserID(request.GetSubjectUserId()),
		AggregateType:   request.GetAggregateType(),
		AggregateKey:    request.GetAggregateKey(),
		MinSupportCount: int(request.GetMinSupportCount()),
	})
	if err != nil {
		return nil, publicError(err)
	}
	var item *memoryv1.ProfileAggregate
	if result.Item.ProfileID != "" {
		item = profileAggregateToProto(result.Item)
	}
	return &memoryv1.RecomputeProfileAggregateResponse{
		Item:         item,
		SupportCount: int32(result.SupportCount),
		Active:       result.Active,
	}, nil
}

func (server *Server) SubmitMemoryCandidate(ctx context.Context, request *memoryv1.SubmitMemoryCandidateRequest) (*memoryv1.SubmitMemoryCandidateResponse, error) {
	if server == nil || server.submitMemoryCandidate == nil {
		return nil, publicError(types.ErrMemoryUnavailable)
	}
	sourceRefs := make([]types.SourceRef, 0, len(request.GetSourceRefs()))
	for _, ref := range request.GetSourceRefs() {
		sourceRefs = append(sourceRefs, sourceRefFromProto(ref))
	}
	result, err := server.submitMemoryCandidate.Execute(ctx, types.SubmitMemoryCandidateCommand{
		AuthContext:         authContext(ctx, request.GetAuthContext()),
		CandidateID:         request.GetCandidateId(),
		Scope:               scopeFromProto(request.GetScope()),
		ScopeID:             request.GetScopeId(),
		ConversationID:      types.ConversationID(request.GetConversationId()),
		Topic:               request.GetTopic(),
		EventType:           eventTypeFromProto(request.GetEventType()),
		FactText:            request.GetFactText(),
		FactSHA256:          request.GetFactSha256(),
		ActorUserIDs:        request.GetActorUserIds(),
		AudienceUserIDs:     request.GetAudienceUserIds(),
		SourceRefs:          sourceRefs,
		ValidFromSeq:        request.GetValidFromSeq(),
		ValidToSeq:          request.GetValidToSeq(),
		SupersedesEventIDs:  request.GetSupersedesEventIds(),
		ContradictsEventIDs: request.GetContradictsEventIds(),
		Confidence:          request.GetConfidence(),
		VisibilityVersion:   request.GetVisibilityVersion(),
		ExtractionVersion:   request.GetExtractionVersion(),
	})
	if err != nil {
		return nil, publicError(err)
	}
	return &memoryv1.SubmitMemoryCandidateResponse{
		Item: structuredMemoryEventToProto(result.Item),
	}, nil
}

func (server *Server) ReviewMemoryCandidate(ctx context.Context, request *memoryv1.ReviewMemoryCandidateRequest) (*memoryv1.ReviewMemoryCandidateResponse, error) {
	if server == nil || server.reviewMemoryCandidate == nil {
		return nil, publicError(types.ErrMemoryUnavailable)
	}
	result, err := server.reviewMemoryCandidate.Execute(ctx, types.ReviewMemoryCandidateCommand{
		AuthContext:   authContext(ctx, request.GetAuthContext()),
		MemoryEventID: request.GetMemoryEventId(),
		Decision:      reviewDecisionFromProto(request.GetDecision()),
	})
	if err != nil {
		return nil, publicError(err)
	}
	return &memoryv1.ReviewMemoryCandidateResponse{
		Item: structuredMemoryEventToProto(result.Item),
	}, nil
}

func authContext(ctx context.Context, auth *memoryv1.AuthContext) types.AuthContext {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		return verified
	}
	if auth == nil {
		return types.AuthContext{}
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  auth.GetDeviceId(),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}
}

func publicError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid memory request")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrMemoryNotFound):
		return status.Error(codes.NotFound, "memory not found")
	case errors.Is(err, types.ErrProjectionStale):
		return status.Error(codes.Unavailable, "memory projection stale")
	case errors.Is(err, types.ErrMemoryUnavailable), errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "memory unavailable")
	default:
		return status.Error(codes.Internal, "memory unavailable")
	}
}

func structuredMemoryEventToProto(item types.StructuredMemoryEvent) *memoryv1.StructuredMemoryEvent {
	sourceRefs := make([]*memoryv1.SourceRef, 0, len(item.SourceRefs))
	for _, ref := range item.SourceRefs {
		sourceRefs = append(sourceRefs, sourceRefToProto(ref))
	}
	return &memoryv1.StructuredMemoryEvent{
		MemoryEventId:       item.MemoryEventID,
		Scope:               scopeToProto(item.Scope),
		ScopeId:             item.ScopeID,
		ConversationId:      string(item.ConversationID),
		Topic:               item.Topic,
		EventType:           eventTypeToProto(item.EventType),
		Status:              statusToProto(item.Status),
		ReviewState:         reviewStateToProto(item.ReviewState),
		FactText:            item.FactText,
		ActorUserIds:        item.ActorUserIDs,
		AudienceUserIds:     item.AudienceUserIDs,
		SourceRefs:          sourceRefs,
		ValidFromSeq:        item.ValidFromSeq,
		ValidToSeq:          item.ValidToSeq,
		ValidFromUnixMs:     unixMillis(item.ValidFromAt),
		ValidToUnixMs:       unixMillis(item.ValidToAt),
		SupersedesEventIds:  item.SupersedesEventIDs,
		ContradictsEventIds: item.ContradictsEventIDs,
		Confidence:          item.Confidence,
		VisibilityVersion:   item.VisibilityVersion,
		ExtractionVersion:   item.ExtractionVersion,
		UpdatedAtUnixMs:     unixMillis(item.UpdatedAt),
	}
}

func sourceRefToProto(ref types.SourceRef) *memoryv1.SourceRef {
	return &memoryv1.SourceRef{
		SourceType:       sourceTypeToProto(ref.SourceType),
		SourceId:         ref.SourceID,
		SourceEventId:    ref.SourceEventID,
		ConversationId:   string(ref.ConversationID),
		ConversationSeq:  ref.ConversationSeq,
		OccurredAtUnixMs: unixMillis(ref.OccurredAt),
	}
}

func sourceRefFromProto(ref *memoryv1.SourceRef) types.SourceRef {
	if ref == nil {
		return types.SourceRef{}
	}
	var occurredAt time.Time
	if ref.GetOccurredAtUnixMs() > 0 {
		occurredAt = time.UnixMilli(ref.GetOccurredAtUnixMs()).UTC()
	}
	return types.SourceRef{
		SourceType:      sourceTypeFromProto(ref.GetSourceType()),
		SourceID:        ref.GetSourceId(),
		SourceEventID:   ref.GetSourceEventId(),
		ConversationID:  types.ConversationID(ref.GetConversationId()),
		ConversationSeq: ref.GetConversationSeq(),
		OccurredAt:      occurredAt,
	}
}

func memoryGraphEdgeToProto(edge types.MemoryGraphEdge) *memoryv1.MemoryGraphEdge {
	sourceRefs := make([]*memoryv1.SourceRef, 0, len(edge.SourceRefs))
	for _, ref := range edge.SourceRefs {
		sourceRefs = append(sourceRefs, sourceRefToProto(ref))
	}
	return &memoryv1.MemoryGraphEdge{
		EdgeId:            edge.EdgeID,
		FromMemoryEventId: edge.FromMemoryEventID,
		ToMemoryEventId:   edge.ToMemoryEventID,
		RelationType:      edge.RelationType,
		Confidence:        edge.Confidence,
		SourceRefs:        sourceRefs,
	}
}

func profileAggregateToProto(item types.ProfileAggregate) *memoryv1.ProfileAggregate {
	return &memoryv1.ProfileAggregate{
		ProfileId:                item.ProfileID,
		SubjectUserId:            string(item.SubjectUserID),
		AggregateType:            item.AggregateType,
		AggregateKey:             item.AggregateKey,
		Status:                   statusToProto(item.Status),
		ReviewState:              reviewStateToProto(item.ReviewState),
		SummaryText:              item.SummaryText,
		SupportingMemoryEventIds: item.SupportingMemoryEventIDs,
		Confidence:               item.Confidence,
		ValidFromUnixMs:          unixMillis(item.ValidFromAt),
		ValidToUnixMs:            unixMillis(item.ValidToAt),
		UpdatedAtUnixMs:          unixMillis(item.UpdatedAt),
	}
}

func unixMillis(value interface {
	IsZero() bool
	UnixMilli() int64
}) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

func scopeFromProto(scope memoryv1.MemoryScope) string {
	switch scope {
	case memoryv1.MemoryScope_MEMORY_SCOPE_CONVERSATION:
		return types.MemoryScopeConversation
	case memoryv1.MemoryScope_MEMORY_SCOPE_PROJECT:
		return types.MemoryScopeProject
	case memoryv1.MemoryScope_MEMORY_SCOPE_PERSONAL:
		return types.MemoryScopePersonal
	case memoryv1.MemoryScope_MEMORY_SCOPE_TENANT:
		return types.MemoryScopeTenant
	default:
		return ""
	}
}

func scopeToProto(scope string) memoryv1.MemoryScope {
	switch scope {
	case types.MemoryScopeConversation:
		return memoryv1.MemoryScope_MEMORY_SCOPE_CONVERSATION
	case types.MemoryScopeProject:
		return memoryv1.MemoryScope_MEMORY_SCOPE_PROJECT
	case types.MemoryScopePersonal:
		return memoryv1.MemoryScope_MEMORY_SCOPE_PERSONAL
	case types.MemoryScopeTenant:
		return memoryv1.MemoryScope_MEMORY_SCOPE_TENANT
	default:
		return memoryv1.MemoryScope_MEMORY_SCOPE_UNSPECIFIED
	}
}

func statusesFromProto(statuses []memoryv1.MemoryEventStatus) []string {
	out := make([]string, 0, len(statuses))
	for _, status := range statuses {
		if value := statusFromProto(status); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func statusFromProto(status memoryv1.MemoryEventStatus) string {
	switch status {
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_PENDING:
		return types.MemoryStatusPending
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE:
		return types.MemoryStatusActive
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_SUPERSEDED:
		return types.MemoryStatusSuperseded
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_REJECTED:
		return types.MemoryStatusRejected
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ARCHIVED:
		return types.MemoryStatusArchived
	case memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_DELETED:
		return types.MemoryStatusDeleted
	default:
		return ""
	}
}

func statusToProto(status string) memoryv1.MemoryEventStatus {
	switch status {
	case types.MemoryStatusPending:
		return memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_PENDING
	case types.MemoryStatusActive:
		return memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE
	case types.MemoryStatusSuperseded:
		return memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_SUPERSEDED
	case types.MemoryStatusRejected:
		return memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_REJECTED
	case types.MemoryStatusArchived:
		return memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ARCHIVED
	case types.MemoryStatusDeleted:
		return memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_DELETED
	default:
		return memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_UNSPECIFIED
	}
}

func eventTypeToProto(eventType string) memoryv1.MemoryEventType {
	switch eventType {
	case types.MemoryEventTypeTask:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_TASK
	case types.MemoryEventTypeDecision:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_DECISION
	case types.MemoryEventTypeStatus:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_STATUS
	case types.MemoryEventTypeBlocker:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_BLOCKER
	case types.MemoryEventTypeFile:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_FILE
	case types.MemoryEventTypePreferenceSignal:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_PREFERENCE_SIGNAL
	case types.MemoryEventTypeRoleSignal:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_ROLE_SIGNAL
	case types.MemoryEventTypeProfileSignal:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_PROFILE_SIGNAL
	default:
		return memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_UNSPECIFIED
	}
}

func eventTypeFromProto(eventType memoryv1.MemoryEventType) string {
	switch eventType {
	case memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_TASK:
		return types.MemoryEventTypeTask
	case memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_DECISION:
		return types.MemoryEventTypeDecision
	case memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_STATUS:
		return types.MemoryEventTypeStatus
	case memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_BLOCKER:
		return types.MemoryEventTypeBlocker
	case memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_FILE:
		return types.MemoryEventTypeFile
	case memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_PREFERENCE_SIGNAL:
		return types.MemoryEventTypePreferenceSignal
	case memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_ROLE_SIGNAL:
		return types.MemoryEventTypeRoleSignal
	case memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_PROFILE_SIGNAL:
		return types.MemoryEventTypeProfileSignal
	default:
		return ""
	}
}

func reviewStateToProto(state string) memoryv1.MemoryReviewState {
	switch state {
	case types.MemoryReviewUnreviewed:
		return memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_UNREVIEWED
	case types.MemoryReviewNeedsReview:
		return memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_NEEDS_REVIEW
	case types.MemoryReviewApproved:
		return memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_APPROVED
	case types.MemoryReviewRejected:
		return memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_REJECTED
	default:
		return memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_UNSPECIFIED
	}
}

func sourceTypeToProto(sourceType string) memoryv1.MemorySourceType {
	switch sourceType {
	case types.MemorySourceTypeMessage:
		return memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_MESSAGE
	case types.MemorySourceTypeTimelineEvent:
		return memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_TIMELINE_EVENT
	case types.MemorySourceTypeProfileAggregate:
		return memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_PROFILE_AGGREGATE
	case types.MemorySourceTypeSystem:
		return memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_SYSTEM
	default:
		return memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_UNSPECIFIED
	}
}

func sourceTypeFromProto(sourceType memoryv1.MemorySourceType) string {
	switch sourceType {
	case memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_MESSAGE:
		return types.MemorySourceTypeMessage
	case memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_TIMELINE_EVENT:
		return types.MemorySourceTypeTimelineEvent
	case memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_PROFILE_AGGREGATE:
		return types.MemorySourceTypeProfileAggregate
	case memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_SYSTEM:
		return types.MemorySourceTypeSystem
	default:
		return ""
	}
}

func reviewDecisionFromProto(decision memoryv1.MemoryReviewDecision) string {
	switch decision {
	case memoryv1.MemoryReviewDecision_MEMORY_REVIEW_DECISION_APPROVE:
		return types.MemoryReviewDecisionApprove
	case memoryv1.MemoryReviewDecision_MEMORY_REVIEW_DECISION_REJECT:
		return types.MemoryReviewDecisionReject
	default:
		return ""
	}
}
