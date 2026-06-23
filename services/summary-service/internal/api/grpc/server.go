package grpc

import (
	"context"
	"errors"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	summaryv1 "github.com/qsyy0921/IM/api/proto/nexusim/summary/v1"
	"github.com/qsyy0921/IM/services/summary-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GenerateConversationSummaryExecutor interface {
	Execute(context.Context, types.GenerateConversationSummaryCommand) (types.GenerateConversationSummaryResult, error)
}

type Server struct {
	summaryv1.UnimplementedSummaryServiceServer
	generateSummary GenerateConversationSummaryExecutor
}

func NewServer(generate GenerateConversationSummaryExecutor) *Server {
	return &Server{generateSummary: generate}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	summaryv1.RegisterSummaryServiceServer(registrar, server)
}

func (server *Server) GenerateConversationSummary(
	ctx context.Context,
	request *summaryv1.GenerateConversationSummaryRequest,
) (*summaryv1.GenerateConversationSummaryResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if server == nil || server.generateSummary == nil {
		return nil, publicError(types.ErrSummaryUnavailable)
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.generateSummary.Execute(ctx, types.GenerateConversationSummaryCommand{
		AuthContext:       auth,
		ConversationID:    types.ConversationID(request.GetConversationId()),
		Focus:             request.GetFocus(),
		AfterSeq:          request.GetAfterSeq(),
		AtConversationSeq: request.GetAtConversationSeq(),
		Limit:             int(request.GetLimit()),
		IncludeSearch:     request.GetIncludeSearch(),
		IncludeMemory:     request.GetIncludeMemory(),
		MemoryStatuses:    memoryStatusesFromProto(request.GetMemoryStatuses()),
	})
	if err != nil {
		return nil, publicError(err)
	}
	return generateConversationSummaryResultToProto(result), nil
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
		return status.Error(codes.InvalidArgument, "invalid summary request")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrRetrievalUnavailable):
		return status.Error(codes.Unavailable, "retrieval unavailable")
	case errors.Is(err, types.ErrCitationVerification):
		return status.Error(codes.Internal, "summary unavailable")
	case errors.Is(err, types.ErrSummaryUnavailable):
		return status.Error(codes.Unavailable, "summary unavailable")
	default:
		return status.Error(codes.Internal, "summary unavailable")
	}
}

func generateConversationSummaryResultToProto(
	result types.GenerateConversationSummaryResult,
) *summaryv1.GenerateConversationSummaryResponse {
	citations := make([]*summaryv1.Citation, 0, len(result.Citations))
	for _, citation := range result.Citations {
		citations = append(citations, &summaryv1.Citation{
			EvidenceId:       citation.EvidenceID,
			SourceType:       sourceTypeToProto(citation.SourceType),
			SourceId:         citation.SourceID,
			SourceEventId:    citation.SourceEventID,
			ConversationId:   string(citation.ConversationID),
			ConversationSeq:  citation.ConversationSeq,
			OccurredAtUnixMs: unixMillis(citation.OccurredAt),
		})
	}
	return &summaryv1.GenerateConversationSummaryResponse{
		SummaryId:      result.SummaryID,
		Status:         summaryStatusToProto(result.Status),
		SummaryText:    result.SummaryText,
		Confidence:     result.Confidence,
		Citations:      citations,
		EvidencePack:   evidencePackToProto(result.EvidencePack),
		SummaryVersion: result.SummaryVersion,
		GeneratedByLlm: result.GeneratedByLLM,
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
	coverage := make([]*retrievalv1.EvidenceSourceCoverage, 0, len(pack.SourceCoverage))
	for _, item := range pack.SourceCoverage {
		coverage = append(coverage, &retrievalv1.EvidenceSourceCoverage{
			SourceType:     sourceTypeToProto(item.SourceType),
			Requested:      item.Requested,
			CandidateCount: int32(item.CandidateCount),
			ReturnedCount:  int32(item.ReturnedCount),
			DedupedCount:   int32(item.DedupedCount),
			Status:         coverageStatusToProto(item.Status),
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
		SourceCoverage:          coverage,
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
	graphEdges := make([]*retrievalv1.EvidenceMemoryGraphEdge, 0, len(item.MemoryGraphEdges))
	for _, edge := range item.MemoryGraphEdges {
		graphEdges = append(graphEdges, memoryGraphEdgeToProto(edge))
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
		RerankScore:       item.RerankScore,
		DedupeReason:      item.DedupeReason,
		MemoryGraphEdges:  graphEdges,
	}
}

func memoryGraphEdgeToProto(edge types.MemoryGraphEdge) *retrievalv1.EvidenceMemoryGraphEdge {
	sourceRefs := make([]*retrievalv1.EvidenceSourceRef, 0, len(edge.SourceRefs))
	for _, ref := range edge.SourceRefs {
		sourceRefs = append(sourceRefs, &retrievalv1.EvidenceSourceRef{
			SourceType:       ref.SourceType,
			SourceId:         ref.SourceID,
			SourceEventId:    ref.SourceEventID,
			ConversationId:   string(ref.ConversationID),
			ConversationSeq:  ref.ConversationSeq,
			OccurredAtUnixMs: unixMillis(ref.OccurredAt),
		})
	}
	return &retrievalv1.EvidenceMemoryGraphEdge{
		EdgeId:            edge.EdgeID,
		FromMemoryEventId: edge.FromMemoryEventID,
		ToMemoryEventId:   edge.ToMemoryEventID,
		RelationType:      edge.RelationType,
		Confidence:        edge.Confidence,
		SourceRefs:        sourceRefs,
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

func summaryStatusToProto(status string) summaryv1.SummaryStatus {
	switch status {
	case types.SummaryStatusGrounded:
		return summaryv1.SummaryStatus_SUMMARY_STATUS_GROUNDED
	case types.SummaryStatusInsufficientEvidence:
		return summaryv1.SummaryStatus_SUMMARY_STATUS_INSUFFICIENT_EVIDENCE
	default:
		return summaryv1.SummaryStatus_SUMMARY_STATUS_UNSPECIFIED
	}
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

func coverageStatusToProto(status string) retrievalv1.EvidenceSourceCoverageStatus {
	switch status {
	case "NOT_REQUESTED":
		return retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_NOT_REQUESTED
	case "EMPTY":
		return retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_EMPTY
	case "RETURNED":
		return retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED
	case "FILTERED":
		return retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_FILTERED
	default:
		return retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_UNSPECIFIED
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
