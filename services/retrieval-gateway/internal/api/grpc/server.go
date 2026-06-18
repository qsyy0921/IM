package grpc

import (
	"context"
	"errors"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RetrieveEvidenceExecutor interface {
	Execute(context.Context, types.RetrieveEvidenceCommand) (types.RetrieveEvidenceResult, error)
}

type Server struct {
	retrievalv1.UnimplementedRetrievalGatewayServer
	retrieveEvidence RetrieveEvidenceExecutor
}

func NewServer(retrieve RetrieveEvidenceExecutor) *Server {
	return &Server{retrieveEvidence: retrieve}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	retrievalv1.RegisterRetrievalGatewayServer(registrar, server)
}

func (server *Server) RetrieveEvidence(
	ctx context.Context,
	request *retrievalv1.RetrieveEvidenceRequest,
) (*retrievalv1.RetrieveEvidenceResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if server == nil || server.retrieveEvidence == nil {
		return nil, publicError(types.ErrRetrievalUnavailable)
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.retrieveEvidence.Execute(ctx, types.RetrieveEvidenceCommand{
		AuthContext:    auth,
		Query:          request.GetQuery(),
		ConversationID: types.ConversationID(request.GetConversationId()),
		AfterSeq:       request.GetAfterSeq(),
		Limit:          int(request.GetLimit()),
		IncludeSearch:  request.GetIncludeSearch(),
		IncludeMemory:  request.GetIncludeMemory(),
		MemoryStatuses: memoryStatusesFromProto(request.GetMemoryStatuses()),
	})
	if err != nil {
		return nil, publicError(err)
	}
	return &retrievalv1.RetrieveEvidenceResponse{
		Pack: evidencePackToProto(result.Pack),
	}, nil
}

func authFromProto(ctx context.Context, auth *retrievalv1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		if auth != nil {
			if verified.TraceID == "" {
				verified.TraceID = auth.GetTraceId()
			}
			if verified.RequestID == "" {
				verified.RequestID = auth.GetRequestId()
			}
		}
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  auth.GetDeviceId(),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}, true
}

func publicError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid retrieval request")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrSearchUnavailable):
		return status.Error(codes.Unavailable, "search unavailable")
	case errors.Is(err, types.ErrMemoryUnavailable):
		return status.Error(codes.Unavailable, "memory unavailable")
	case errors.Is(err, types.ErrRetrievalUnavailable):
		return status.Error(codes.Unavailable, "retrieval unavailable")
	default:
		return status.Error(codes.Internal, "retrieval unavailable")
	}
}

func evidencePackToProto(pack types.EvidencePack) *retrievalv1.EvidencePack {
	items := make([]*retrievalv1.EvidenceItem, 0, len(pack.Items))
	for _, item := range pack.Items {
		items = append(items, evidenceItemToProto(item))
	}
	counts := make([]*retrievalv1.EvidenceSourceCount, 0, len(pack.SourceCounts))
	for _, count := range pack.SourceCounts {
		counts = append(counts, &retrievalv1.EvidenceSourceCount{
			SourceType: sourceTypeToProto(count.SourceType),
			Count:      int32(count.Count),
		})
	}
	return &retrievalv1.EvidencePack{
		PackId:                  pack.PackID,
		TenantId:                string(pack.TenantID),
		Query:                   pack.Query,
		ConversationId:          string(pack.ConversationID),
		Items:                   items,
		SourceCounts:            counts,
		SearchProjectionVersion: pack.SearchProjectionVersion,
		MemoryProjectionVersion: pack.MemoryProjectionVersion,
		RetrievalVersion:        pack.RetrievalVersion,
	}
}

func evidenceItemToProto(item types.EvidenceItem) *retrievalv1.EvidenceItem {
	sourceRefs := make([]*retrievalv1.EvidenceSourceRef, 0, len(item.SourceRefs))
	for _, ref := range item.SourceRefs {
		sourceRefs = append(sourceRefs, &retrievalv1.EvidenceSourceRef{
			SourceType:       ref.SourceType,
			SourceId:         ref.SourceID,
			SourceEventId:    ref.SourceEventID,
			ConversationId:   string(ref.ConversationID),
			ConversationSeq:  ref.ConversationSeq,
			OccurredAtUnixMs: unixMillis(ref.OccurredAt),
		})
	}
	return &retrievalv1.EvidenceItem{
		EvidenceId:        item.EvidenceID,
		SourceType:        sourceTypeToProto(item.SourceType),
		SourceId:          item.SourceID,
		ConversationId:    string(item.ConversationID),
		ConversationSeq:   item.ConversationSeq,
		Text:              item.Text,
		Score:             item.Score,
		SpeakerUserId:     string(item.SpeakerUserID),
		MessageId:         item.MessageID,
		MemoryEventId:     item.MemoryEventID,
		OccurredAtUnixMs:  unixMillis(item.OccurredAt),
		ValidFromSeq:      item.ValidFromSeq,
		ValidToSeq:        item.ValidToSeq,
		VisibilityVersion: item.VisibilityVersion,
		SourceRefs:        sourceRefs,
		ActorUserIds:      item.ActorUserIDs,
		AudienceUserIds:   item.AudienceUserIDs,
		TemporalStatus:    item.TemporalStatus,
		ReviewState:       item.ReviewState,
		ExtractionVersion: item.ExtractionVersion,
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

func sourceTypeToProto(sourceType string) retrievalv1.EvidenceSourceType {
	switch sourceType {
	case types.EvidenceSourceSearchMessage:
		return retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE
	case types.EvidenceSourceMemoryEvent:
		return retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT
	default:
		return retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_UNSPECIFIED
	}
}

func memoryStatusesFromProto(statuses []retrievalv1.EvidenceMemoryStatus) []string {
	out := make([]string, 0, len(statuses))
	for _, status := range statuses {
		switch status {
		case retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_PENDING:
			out = append(out, types.MemoryStatusPending)
		case retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE:
			out = append(out, types.MemoryStatusActive)
		case retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_SUPERSEDED:
			out = append(out, types.MemoryStatusSuperseded)
		case retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ARCHIVED:
			out = append(out, types.MemoryStatusArchived)
		}
	}
	return out
}
