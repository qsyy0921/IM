package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/rag-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type RetrievalClient struct {
	client  retrievalv1.RetrievalGatewayClient
	timeout time.Duration
}

func NewRetrievalClient(client retrievalv1.RetrievalGatewayClient, timeout time.Duration) RetrievalClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return RetrievalClient{client: client, timeout: timeout}
}

func DialRetrievalClient(_ context.Context, addr string, timeout time.Duration) (RetrievalClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return RetrievalClient{}, nil, errors.New("retrieval-gateway address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return RetrievalClient{}, nil, err
	}
	return NewRetrievalClient(retrievalv1.NewRetrievalGatewayClient(conn), timeout), conn.Close, nil
}

func (client RetrievalClient) RetrieveEvidence(
	ctx context.Context,
	query types.RetrieveEvidenceQuery,
) (types.RetrieveEvidenceResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, query.AuthContext)
	response, err := client.client.RetrieveEvidence(callCtx, &retrievalv1.RetrieveEvidenceRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId:  string(query.AuthContext.TenantID),
			UserId:    string(query.AuthContext.UserID),
			DeviceId:  query.AuthContext.DeviceID,
			SessionId: query.AuthContext.SessionID,
			TraceId:   query.AuthContext.TraceID,
			RequestId: query.AuthContext.RequestID,
		},
		Query:             query.Query,
		ConversationId:    string(query.ConversationID),
		AfterSeq:          query.AfterSeq,
		AtConversationSeq: query.AtConversationSeq,
		Limit:             int32(query.Limit),
		IncludeSearch:     query.IncludeSearch,
		IncludeMemory:     query.IncludeMemory,
		MemoryStatuses:    memoryStatusesToProto(query.MemoryStatuses),
	})
	if err != nil {
		return types.RetrieveEvidenceResult{}, mapRetrievalError(err)
	}
	return types.RetrieveEvidenceResult{Pack: evidencePackFromProto(response.GetPack())}, nil
}

func evidencePackFromProto(pack *retrievalv1.EvidencePack) types.EvidencePack {
	if pack == nil {
		return types.EvidencePack{}
	}
	items := make([]types.EvidenceItem, 0, len(pack.GetItems()))
	for _, item := range pack.GetItems() {
		items = append(items, evidenceItemFromProto(item))
	}
	counts := make([]types.EvidenceSourceCount, 0, len(pack.GetSourceCounts()))
	for _, count := range pack.GetSourceCounts() {
		counts = append(counts, types.EvidenceSourceCount{
			SourceType: sourceTypeFromProto(count.GetSourceType()),
			Count:      int(count.GetCount()),
		})
	}
	coverage := make([]types.EvidenceSourceCoverage, 0, len(pack.GetSourceCoverage()))
	for _, item := range pack.GetSourceCoverage() {
		coverage = append(coverage, types.EvidenceSourceCoverage{
			SourceType:     sourceTypeFromProto(item.GetSourceType()),
			Requested:      item.GetRequested(),
			CandidateCount: int(item.GetCandidateCount()),
			ReturnedCount:  int(item.GetReturnedCount()),
			DedupedCount:   int(item.GetDedupedCount()),
			Status:         coverageStatusFromProto(item.GetStatus()),
		})
	}
	return types.EvidencePack{
		PackID:                  pack.GetPackId(),
		TenantID:                types.TenantID(pack.GetTenantId()),
		Query:                   pack.GetQuery(),
		ConversationID:          types.ConversationID(pack.GetConversationId()),
		Items:                   items,
		SourceCounts:            counts,
		SearchProjectionVersion: pack.GetSearchProjectionVersion(),
		MemoryProjectionVersion: pack.GetMemoryProjectionVersion(),
		RetrievalVersion:        pack.GetRetrievalVersion(),
		SourceCoverage:          coverage,
	}
}

func evidenceItemFromProto(item *retrievalv1.EvidenceItem) types.EvidenceItem {
	if item == nil {
		return types.EvidenceItem{}
	}
	sourceRefs := make([]types.EvidenceSourceRef, 0, len(item.GetSourceRefs()))
	for _, ref := range item.GetSourceRefs() {
		sourceRefs = append(sourceRefs, types.EvidenceSourceRef{
			SourceType:      ref.GetSourceType(),
			SourceID:        ref.GetSourceId(),
			SourceEventID:   ref.GetSourceEventId(),
			ConversationID:  types.ConversationID(ref.GetConversationId()),
			ConversationSeq: ref.GetConversationSeq(),
			OccurredAt:      unixMillisToTime(ref.GetOccurredAtUnixMs()),
		})
	}
	graphEdges := make([]types.MemoryGraphEdge, 0, len(item.GetMemoryGraphEdges()))
	for _, edge := range item.GetMemoryGraphEdges() {
		graphEdges = append(graphEdges, memoryGraphEdgeFromProto(edge))
	}
	return types.EvidenceItem{
		EvidenceID:        item.GetEvidenceId(),
		SourceType:        sourceTypeFromProto(item.GetSourceType()),
		SourceID:          item.GetSourceId(),
		ConversationID:    types.ConversationID(item.GetConversationId()),
		ConversationSeq:   item.GetConversationSeq(),
		Text:              item.GetText(),
		Score:             item.GetScore(),
		SpeakerUserID:     types.UserID(item.GetSpeakerUserId()),
		MessageID:         item.GetMessageId(),
		MemoryEventID:     item.GetMemoryEventId(),
		OccurredAt:        unixMillisToTime(item.GetOccurredAtUnixMs()),
		ValidFromSeq:      item.GetValidFromSeq(),
		ValidToSeq:        item.GetValidToSeq(),
		VisibilityVersion: item.GetVisibilityVersion(),
		SourceRefs:        sourceRefs,
		ActorUserIDs:      item.GetActorUserIds(),
		AudienceUserIDs:   item.GetAudienceUserIds(),
		TemporalStatus:    item.GetTemporalStatus(),
		ReviewState:       item.GetReviewState(),
		ExtractionVersion: item.GetExtractionVersion(),
		RerankScore:       item.GetRerankScore(),
		DedupeReason:      item.GetDedupeReason(),
		MemoryGraphEdges:  graphEdges,
	}
}

func memoryGraphEdgeFromProto(edge *retrievalv1.EvidenceMemoryGraphEdge) types.MemoryGraphEdge {
	if edge == nil {
		return types.MemoryGraphEdge{}
	}
	sourceRefs := make([]types.EvidenceSourceRef, 0, len(edge.GetSourceRefs()))
	for _, ref := range edge.GetSourceRefs() {
		sourceRefs = append(sourceRefs, types.EvidenceSourceRef{
			SourceType:      ref.GetSourceType(),
			SourceID:        ref.GetSourceId(),
			SourceEventID:   ref.GetSourceEventId(),
			ConversationID:  types.ConversationID(ref.GetConversationId()),
			ConversationSeq: ref.GetConversationSeq(),
			OccurredAt:      unixMillisToTime(ref.GetOccurredAtUnixMs()),
		})
	}
	return types.MemoryGraphEdge{
		EdgeID:            edge.GetEdgeId(),
		FromMemoryEventID: edge.GetFromMemoryEventId(),
		ToMemoryEventID:   edge.GetToMemoryEventId(),
		RelationType:      edge.GetRelationType(),
		Confidence:        edge.GetConfidence(),
		SourceRefs:        sourceRefs,
	}
}

func memoryStatusesToProto(statuses []string) []retrievalv1.EvidenceMemoryStatus {
	out := make([]retrievalv1.EvidenceMemoryStatus, 0, len(statuses))
	for _, status := range statuses {
		switch status {
		case types.MemoryStatusPending:
			out = append(out, retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_PENDING)
		case types.MemoryStatusActive:
			out = append(out, retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE)
		case types.MemoryStatusSuperseded:
			out = append(out, retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_SUPERSEDED)
		case types.MemoryStatusArchived:
			out = append(out, retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ARCHIVED)
		}
	}
	return out
}

func sourceTypeFromProto(sourceType retrievalv1.EvidenceSourceType) string {
	switch sourceType {
	case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE:
		return types.EvidenceSourceSearchMessage
	case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT:
		return types.EvidenceSourceMemoryEvent
	default:
		return ""
	}
}

func coverageStatusFromProto(status retrievalv1.EvidenceSourceCoverageStatus) string {
	switch status {
	case retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_NOT_REQUESTED:
		return "NOT_REQUESTED"
	case retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_EMPTY:
		return "EMPTY"
	case retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED:
		return "RETURNED"
	case retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_FILTERED:
		return "FILTERED"
	default:
		return ""
	}
}

func unixMillisToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}
