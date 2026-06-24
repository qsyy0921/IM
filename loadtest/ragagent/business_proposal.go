package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	actionexecutorv1 "github.com/qsyy0921/IM/api/proto/nexusim/actionexecutor/v1"
	agentv1 "github.com/qsyy0921/IM/api/proto/nexusim/agent/v1"
	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const businessProposalObjective = "phoenix launch business proposal source chain"

type businessProposalScenarioSummary struct {
	ProposalVerified       bool     `json:"business_proposal_verified"`
	ApprovalRecorded       bool     `json:"business_proposal_approval_recorded"`
	ActionAuditRecorded    bool     `json:"business_action_audit_recorded"`
	ActionExecuted         bool     `json:"business_action_executed"`
	ProposalID             string   `json:"business_proposal_id,omitempty"`
	ApprovalID             string   `json:"business_approval_id,omitempty"`
	ExecutionID            string   `json:"business_execution_id,omitempty"`
	ExecutionStatus        string   `json:"business_execution_status,omitempty"`
	ProposalTextSHA256     string   `json:"business_proposal_text_sha256,omitempty"`
	ActionInputSHA256      string   `json:"business_action_input_sha256,omitempty"`
	MemoryEventCount       int      `json:"business_proposal_memory_event_count"`
	EvidenceMemoryCount    int      `json:"business_proposal_evidence_memory_count"`
	SourceRefCount         int      `json:"business_proposal_source_ref_count"`
	CrossGroupSourceRefs   int      `json:"business_proposal_cross_group_source_ref_count"`
	EventTypes             []string `json:"business_proposal_event_types,omitempty"`
	FactSHA256             []string `json:"business_proposal_fact_sha256,omitempty"`
	ResourceType           string   `json:"business_resource_type,omitempty"`
	ResourceID             string   `json:"business_resource_id,omitempty"`
	ToolName               string   `json:"business_tool_name,omitempty"`
	SkillID                string   `json:"business_skill_id,omitempty"`
	RequiresApproval       bool     `json:"business_requires_approval"`
	PolicyAllowed          bool     `json:"business_policy_allowed"`
	PolicyRequiresApproval bool     `json:"business_policy_requires_approval"`
}

func verifyBusinessProposalScenario(
	ctx context.Context,
	cfg config,
	seed seedSummary,
) (businessProposalScenarioSummary, error) {
	conversationSeq := seed.CurrentMemoryAtSeq
	if conversationSeq <= 0 {
		conversationSeq = seed.ConversationSeq
	}
	if conversationSeq <= 0 {
		return businessProposalScenarioSummary{}, errors.New("business proposal scenario requires a positive conversation seq")
	}
	if seed.CrossGroupConversationID == "" {
		return businessProposalScenarioSummary{}, errors.New("business proposal scenario requires a cross-group conversation id")
	}
	suffix, err := randomSuffix()
	if err != nil {
		return businessProposalScenarioSummary{}, err
	}
	candidates := buildBusinessProposalCandidates(suffix, conversationSeq)

	memoryConn, err := grpc.NewClient("passthrough:///"+cfg.memoryTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return businessProposalScenarioSummary{}, err
	}
	defer memoryConn.Close()
	memoryClient := memoryv1.NewMemoryServiceClient(memoryConn)
	for _, candidate := range candidates {
		if err := submitAndApproveReviewedMemoryCandidate(
			ctx,
			cfg,
			memoryClient,
			seed,
			candidate,
			"rag-agent-business-proposal-source-chain",
			"rag-agent-business-proposal-v1",
		); err != nil {
			return businessProposalScenarioSummary{}, err
		}
	}

	querySeq := candidates[len(candidates)-1].ValidFromSeq + 1
	proposal, err := createBusinessProposal(ctx, cfg, seed, querySeq)
	if err != nil {
		return businessProposalScenarioSummary{}, err
	}
	memoryCount, sourceRefCount, crossRefCount, err := verifyGroupMemoryEvidencePack(proposal.GetEvidencePack(), seed, candidates)
	if err != nil {
		return businessProposalScenarioSummary{}, fmt.Errorf("business proposal EvidencePack: %w", err)
	}
	approval, err := approveBusinessProposal(ctx, cfg, proposal.GetProposalId())
	if err != nil {
		return businessProposalScenarioSummary{}, err
	}
	inputJSON, inputHash, err := businessActionInput(candidates)
	if err != nil {
		return businessProposalScenarioSummary{}, err
	}
	execution, err := executeBusinessProposalAction(ctx, cfg, seed, proposal, approval, inputJSON)
	if err != nil {
		return businessProposalScenarioSummary{}, err
	}
	if execution.GetStatus() != actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_RECORDED {
		return businessProposalScenarioSummary{}, fmt.Errorf("business action execution status %v, want RECORDED", execution.GetStatus())
	}
	if !execution.GetAllowed() || !execution.GetRequiresApproval() {
		return businessProposalScenarioSummary{}, fmt.Errorf("business action policy mismatch allowed=%v requires_approval=%v", execution.GetAllowed(), execution.GetRequiresApproval())
	}
	if execution.GetProposalId() != proposal.GetProposalId() ||
		execution.GetApprovalId() != approval.GetApprovalId() ||
		execution.GetPreparedAuditId() != proposal.GetPreparedAuditId() ||
		execution.GetSkillId() != defaultAgentSkillID ||
		execution.GetToolName() != defaultAgentToolName ||
		execution.GetResourceType() != defaultAgentResourceType ||
		execution.GetResourceId() != seed.ConversationID {
		return businessProposalScenarioSummary{}, fmt.Errorf("business action binding mismatch: %+v", execution)
	}
	if execution.GetExecuted() {
		return businessProposalScenarioSummary{}, errors.New("business proposal scenario must record audit without executing an unconfigured business mutation")
	}

	return businessProposalScenarioSummary{
		ProposalVerified:       true,
		ApprovalRecorded:       true,
		ActionAuditRecorded:    true,
		ActionExecuted:         execution.GetExecuted(),
		ProposalID:             proposal.GetProposalId(),
		ApprovalID:             approval.GetApprovalId(),
		ExecutionID:            execution.GetExecutionId(),
		ExecutionStatus:        "RECORDED",
		ProposalTextSHA256:     sha256Hex(proposal.GetProposalText()),
		ActionInputSHA256:      inputHash,
		MemoryEventCount:       len(candidates),
		EvidenceMemoryCount:    memoryCount,
		SourceRefCount:         sourceRefCount,
		CrossGroupSourceRefs:   crossRefCount,
		EventTypes:             groupMemoryEventTypes(candidates),
		FactSHA256:             groupMemoryFactHashes(candidates),
		ResourceType:           defaultAgentResourceType,
		ResourceID:             seed.ConversationID,
		ToolName:               defaultAgentToolName,
		SkillID:                defaultAgentSkillID,
		RequiresApproval:       proposal.GetRequiresApproval(),
		PolicyAllowed:          proposal.GetToolPolicyDecision().GetAllowed(),
		PolicyRequiresApproval: proposal.GetToolPolicyDecision().GetRequiresApproval(),
	}, nil
}

func buildBusinessProposalCandidates(suffix string, conversationSeq int64) []groupMemoryCandidate {
	facts := []struct {
		eventType     memoryv1.MemoryEventType
		eventTypeName string
		factText      string
	}{
		{
			eventType:     memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_DECISION,
			eventTypeName: "DECISION",
			factText:      "decision: phoenix launch business proposal source chain requires an owner-facing rollout note",
		},
		{
			eventType:     memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_TASK,
			eventTypeName: "TASK",
			factText:      "task: phoenix launch business proposal source chain must assign security review and release owner follow-up",
		},
		{
			eventType:     memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_STATUS,
			eventTypeName: "STATUS",
			factText:      "status: phoenix launch business proposal source chain is ready for approved conversation-note audit only",
		},
	}
	candidates := make([]groupMemoryCandidate, 0, len(facts))
	for index, fact := range facts {
		id := fmt.Sprintf("ragagent-business-proposal-%s-%d", suffix, index+1)
		candidates = append(candidates, groupMemoryCandidate{
			EventID:            id,
			EventType:          fact.eventType,
			EventTypeName:      fact.eventTypeName,
			FactText:           fact.factText,
			FactSHA256:         normalizedFactSHA256(fact.factText),
			SourceID:           "msg-" + id,
			SourceEventID:      "evt-" + id,
			CrossSourceID:      "msg-" + id + "-cross",
			CrossSourceEventID: "evt-" + id + "-cross",
			ValidFromSeq:       conversationSeq + int64(index) + 40,
		})
	}
	return candidates
}

func createBusinessProposal(
	ctx context.Context,
	cfg config,
	seed seedSummary,
	conversationSeq int64,
) (*agentv1.CreateAgentProposalResponse, error) {
	dialOption, err := dialOptionFromTLSFlags(cfg.agentTLS, "agent-tls")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.agentTarget, dialOption)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := agentv1.NewAgentServiceClient(conn).CreateAgentProposal(requestCtx, &agentv1.CreateAgentProposalRequest{
		AuthContext:       retrievalAuth(cfg, seed.ViewerUserID),
		ConversationId:    seed.ConversationID,
		Objective:         businessProposalObjective,
		ToolName:          defaultAgentToolName,
		ToolAction:        policyv1.ToolAction_TOOL_ACTION_CALL,
		ResourceType:      defaultAgentResourceType,
		ResourceId:        seed.ConversationID,
		RiskLevel:         "LOW",
		Intent:            businessProposalObjective,
		Limit:             10,
		IncludeSearch:     false,
		IncludeMemory:     true,
		MemoryStatuses:    []retrievalv1.EvidenceMemoryStatus{retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE},
		SkillId:           defaultAgentSkillID,
		AtConversationSeq: conversationSeq,
	})
	if err != nil {
		return nil, fmt.Errorf("create business proposal: %w", err)
	}
	if response.GetStatus() != agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_PROPOSED {
		return nil, fmt.Errorf("business proposal status %v, want PROPOSED", response.GetStatus())
	}
	if !response.GetRequiresApproval() || response.GetToolPolicyDecision() == nil ||
		!response.GetToolPolicyDecision().GetAllowed() ||
		!response.GetToolPolicyDecision().GetRequiresApproval() {
		return nil, fmt.Errorf("business proposal should be allowed and require approval: %+v", response.GetToolPolicyDecision())
	}
	return response, nil
}

func approveBusinessProposal(
	ctx context.Context,
	cfg config,
	proposalID string,
) (*agentv1.ApproveAgentProposalResponse, error) {
	dialOption, err := dialOptionFromTLSFlags(cfg.agentTLS, "agent-tls")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.agentTarget, dialOption)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	approval, err := agentv1.NewAgentServiceClient(conn).ApproveAgentProposal(requestCtx, &agentv1.ApproveAgentProposalRequest{
		AuthContext: retrievalAuth(cfg, cfg.viewerUserID),
		ProposalId:  proposalID,
		Reason:      "approved business proposal source-chain demo",
	})
	if err != nil {
		return nil, fmt.Errorf("approve business proposal: %w", err)
	}
	if approval.GetStatus() != agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_APPROVED ||
		approval.GetApprovalId() == "" {
		return nil, fmt.Errorf("business proposal approval mismatch: %+v", approval)
	}
	return approval, nil
}

func executeBusinessProposalAction(
	ctx context.Context,
	cfg config,
	seed seedSummary,
	proposal *agentv1.CreateAgentProposalResponse,
	approval *agentv1.ApproveAgentProposalResponse,
	inputJSON string,
) (*actionexecutorv1.ExecuteApprovedActionResponse, error) {
	conn, err := grpc.NewClient(
		"passthrough:///"+cfg.actionTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := actionexecutorv1.NewActionExecutorServiceClient(conn).ExecuteApprovedAction(
		requestCtx,
		&actionexecutorv1.ExecuteApprovedActionRequest{
			AuthContext: &actionexecutorv1.AuthContext{
				TenantId:  cfg.tenantID,
				UserId:    seed.ViewerUserID,
				DeviceId:  cfg.deviceID,
				SessionId: "rag-agent-demo-business-action-session",
				TraceId:   "rag-agent-demo-business-action-trace",
				RequestId: "rag-agent-demo-business-action-request",
			},
			ProposalId:      proposal.GetProposalId(),
			ApprovalId:      approval.GetApprovalId(),
			PreparedAuditId: proposal.GetPreparedAuditId(),
			SkillId:         defaultAgentSkillID,
			ToolName:        defaultAgentToolName,
			Action:          policyv1.ToolAction_TOOL_ACTION_EXECUTE,
			ResourceType:    defaultAgentResourceType,
			ResourceId:      seed.ConversationID,
			RiskLevel:       "LOW",
			Intent:          businessProposalObjective,
			InputJson:       inputJSON,
			IdempotencyKey:  proposal.GetProposalId() + "-business-execute",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("execute business proposal action: %w", err)
	}
	return response, nil
}

func businessActionInput(candidates []groupMemoryCandidate) (string, string, error) {
	eventHashes := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		eventHashes = append(eventHashes, sha256Hex(candidate.EventID))
	}
	encoded, err := json.Marshal(map[string]any{
		"action":                "conversation_note_audit",
		"source":                "loadtest/ragagent",
		"evidence_event_count":  len(candidates),
		"evidence_event_hashes": eventHashes,
	})
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(encoded)
	return string(encoded), hex.EncodeToString(sum[:]), nil
}
