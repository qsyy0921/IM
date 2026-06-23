package grpc

import (
	"context"
	"errors"

	agentv1 "github.com/qsyy0921/IM/api/proto/nexusim/agent/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CreateAgentProposalExecutor interface {
	Execute(context.Context, types.CreateAgentProposalCommand) (types.CreateAgentProposalResult, error)
}

type ApproveAgentProposalExecutor interface {
	Execute(context.Context, types.ApproveAgentProposalCommand) (types.ApproveAgentProposalResult, error)
}

type VerifyApprovedAgentProposalExecutor interface {
	Execute(context.Context, types.VerifyApprovedAgentProposalCommand) (types.VerifyApprovedAgentProposalResult, error)
}

type Server struct {
	agentv1.UnimplementedAgentServiceServer
	createProposal  CreateAgentProposalExecutor
	approveProposal ApproveAgentProposalExecutor
	verifyProposal  VerifyApprovedAgentProposalExecutor
}

func NewServer(createProposal CreateAgentProposalExecutor) *Server {
	return &Server{createProposal: createProposal}
}

func NewServerWithWorkflows(
	createProposal CreateAgentProposalExecutor,
	approveProposal ApproveAgentProposalExecutor,
	verifyProposal VerifyApprovedAgentProposalExecutor,
) *Server {
	return &Server{
		createProposal:  createProposal,
		approveProposal: approveProposal,
		verifyProposal:  verifyProposal,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	agentv1.RegisterAgentServiceServer(registrar, server)
}

func (server *Server) ApproveAgentProposal(
	ctx context.Context,
	request *agentv1.ApproveAgentProposalRequest,
) (*agentv1.ApproveAgentProposalResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if server == nil || server.approveProposal == nil {
		return nil, publicError(types.ErrProposalStoreUnavailable)
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.approveProposal.Execute(ctx, types.ApproveAgentProposalCommand{
		AuthContext: auth,
		ProposalID:  request.GetProposalId(),
		Reason:      request.GetReason(),
	})
	if err != nil {
		return nil, publicError(err)
	}
	return &agentv1.ApproveAgentProposalResponse{
		ProposalId:       result.ProposalID,
		ApprovalId:       result.ApprovalID,
		Status:           proposalStatusToProto(result.Status),
		ApprovedByUserId: string(result.ApprovedByUserID),
		ApprovedAtUnixMs: unixMillis(result.ApprovedAt),
	}, nil
}

func (server *Server) VerifyApprovedAgentProposal(
	ctx context.Context,
	request *agentv1.VerifyApprovedAgentProposalRequest,
) (*agentv1.VerifyApprovedAgentProposalResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if server == nil || server.verifyProposal == nil {
		return nil, publicError(types.ErrProposalStoreUnavailable)
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.verifyProposal.Execute(ctx, types.VerifyApprovedAgentProposalCommand{
		AuthContext:     auth,
		ProposalID:      request.GetProposalId(),
		ApprovalID:      request.GetApprovalId(),
		PreparedAuditID: request.GetPreparedAuditId(),
		SkillID:         request.GetSkillId(),
		ToolName:        request.GetToolName(),
		ResourceType:    request.GetResourceType(),
		ResourceID:      request.GetResourceId(),
	})
	if err != nil {
		return nil, publicError(err)
	}
	return &agentv1.VerifyApprovedAgentProposalResponse{
		ProposalId:       result.ProposalID,
		ApprovalId:       result.ApprovalID,
		Status:           proposalStatusToProto(result.Status),
		UserId:           string(result.UserID),
		ConversationId:   string(result.ConversationID),
		SkillId:          result.SkillID,
		PreparedAuditId:  result.PreparedAuditID,
		ToolName:         result.ToolName,
		ResourceType:     result.ResourceType,
		ResourceId:       result.ResourceID,
		RiskLevel:        result.RiskLevel,
		ApprovedAtUnixMs: unixMillis(result.ApprovedAt),
	}, nil
}

func (server *Server) CreateAgentProposal(
	ctx context.Context,
	request *agentv1.CreateAgentProposalRequest,
) (*agentv1.CreateAgentProposalResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if server == nil || server.createProposal == nil {
		return nil, publicError(types.ErrAgentUnavailable)
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.createProposal.Execute(ctx, types.CreateAgentProposalCommand{
		AuthContext:       auth,
		ConversationID:    types.ConversationID(request.GetConversationId()),
		Objective:         request.GetObjective(),
		SkillID:           request.GetSkillId(),
		ToolName:          request.GetToolName(),
		ToolAction:        toolActionFromProto(request.GetToolAction()),
		ResourceType:      request.GetResourceType(),
		ResourceID:        request.GetResourceId(),
		RiskLevel:         request.GetRiskLevel(),
		Intent:            request.GetIntent(),
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
	return createAgentProposalResultToProto(result), nil
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
		return status.Error(codes.InvalidArgument, "invalid agent request")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrRetrievalUnavailable):
		return status.Error(codes.Unavailable, "retrieval unavailable")
	case errors.Is(err, types.ErrToolPolicyUnavailable):
		return status.Error(codes.Unavailable, "tool policy unavailable")
	case errors.Is(err, types.ErrToolPrepareUnavailable):
		return status.Error(codes.Unavailable, "tool prepare unavailable")
	case errors.Is(err, types.ErrCitationVerification):
		return status.Error(codes.Internal, "agent unavailable")
	case errors.Is(err, types.ErrProposalStoreUnavailable):
		return status.Error(codes.Unavailable, "proposal store unavailable")
	case errors.Is(err, types.ErrProposalNotFound):
		return status.Error(codes.NotFound, "proposal not found")
	case errors.Is(err, types.ErrProposalNotApprovable):
		return status.Error(codes.FailedPrecondition, "proposal not approvable")
	case errors.Is(err, types.ErrProposalNotApproved):
		return status.Error(codes.FailedPrecondition, "proposal not approved")
	case errors.Is(err, types.ErrProposalMismatch):
		return status.Error(codes.FailedPrecondition, "proposal mismatch")
	case errors.Is(err, types.ErrAgentUnavailable):
		return status.Error(codes.Unavailable, "agent unavailable")
	default:
		return status.Error(codes.Internal, "agent unavailable")
	}
}

func createAgentProposalResultToProto(
	result types.CreateAgentProposalResult,
) *agentv1.CreateAgentProposalResponse {
	citations := make([]*agentv1.AgentCitation, 0, len(result.Citations))
	for _, citation := range result.Citations {
		citations = append(citations, &agentv1.AgentCitation{
			EvidenceId:       citation.EvidenceID,
			SourceType:       sourceTypeToProto(citation.SourceType),
			SourceId:         citation.SourceID,
			SourceEventId:    citation.SourceEventID,
			ConversationId:   string(citation.ConversationID),
			ConversationSeq:  citation.ConversationSeq,
			OccurredAtUnixMs: unixMillis(citation.OccurredAt),
		})
	}
	return &agentv1.CreateAgentProposalResponse{
		ProposalId:         result.ProposalID,
		Status:             proposalStatusToProto(result.Status),
		ProposalText:       result.ProposalText,
		RequiresApproval:   result.RequiresApproval,
		ToolPolicyDecision: toolPolicyDecisionToProto(result.ToolPolicyDecision),
		Citations:          citations,
		EvidencePack:       evidencePackToProto(result.EvidencePack),
		AgentVersion:       result.AgentVersion,
		GeneratedByLlm:     result.GeneratedByLLM,
		SkillId:            result.SkillID,
		PreparedAuditId:    result.PreparedAuditID,
	}
}

func toolPolicyDecisionToProto(decision types.ToolPolicyDecision) *agentv1.ToolPolicyDecision {
	return &agentv1.ToolPolicyDecision{
		TenantId:          string(decision.TenantID),
		UserId:            string(decision.UserID),
		ToolName:          decision.ToolName,
		Action:            toolActionToProto(decision.Action),
		ResourceType:      decision.ResourceType,
		ResourceId:        decision.ResourceID,
		RiskLevel:         decision.RiskLevel,
		Allowed:           decision.Allowed,
		RequiresApproval:  decision.RequiresApproval,
		PermissionVersion: decision.PermissionVersion,
		Classification:    decision.Classification,
		Reason:            decision.Reason,
		DecisionSource:    decision.DecisionSource,
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

func proposalStatusToProto(status string) agentv1.AgentProposalStatus {
	switch status {
	case types.AgentProposalStatusProposed:
		return agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_PROPOSED
	case types.AgentProposalStatusBlocked:
		return agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_BLOCKED
	case types.AgentProposalStatusInsufficientEvidence:
		return agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_INSUFFICIENT_EVIDENCE
	case types.AgentProposalStatusApproved:
		return agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_APPROVED
	default:
		return agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_UNSPECIFIED
	}
}

func toolActionFromProto(action policyv1.ToolAction) string {
	switch action {
	case policyv1.ToolAction_TOOL_ACTION_CALL:
		return types.ToolActionCall
	case policyv1.ToolAction_TOOL_ACTION_APPROVE:
		return types.ToolActionApprove
	case policyv1.ToolAction_TOOL_ACTION_EXECUTE:
		return types.ToolActionExecute
	default:
		return ""
	}
}

func toolActionToProto(action string) policyv1.ToolAction {
	switch action {
	case types.ToolActionCall:
		return policyv1.ToolAction_TOOL_ACTION_CALL
	case types.ToolActionApprove:
		return policyv1.ToolAction_TOOL_ACTION_APPROVE
	case types.ToolActionExecute:
		return policyv1.ToolAction_TOOL_ACTION_EXECUTE
	default:
		return policyv1.ToolAction_TOOL_ACTION_UNSPECIFIED
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
