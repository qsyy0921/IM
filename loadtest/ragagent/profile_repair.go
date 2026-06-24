package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	agentv1 "github.com/qsyy0921/IM/api/proto/nexusim/agent/v1"
	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	ragv1 "github.com/qsyy0921/IM/api/proto/nexusim/rag/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	profileRepairBatchSchemaVersion = "nexusim.memory.profile_repair_batch.v1"
	profileRepairWorkflowType       = "REPAIR_APPROVAL"
	profileRepairTargetService      = "memory-service"
	profileRepairTargetOperation    = "RECOMPUTE_PROFILE_AGGREGATE_BATCH"
	profileRepairAggregateType      = "SKILL"
	profileRepairAggregateKey       = "phoenix-launch"
)

type profileRepairApprovalSummary struct {
	ApprovalRequested     bool
	WorkflowApproved      bool
	ApprovalVerified      bool
	Executed              bool
	NegativeCasesVerified bool
	NegativeCases         []profileRepairNegativeCaseSummary
	ProfileActive         bool
	SupportCount          int32
	SupportingMemoryCount int
	WorkflowID            string
	PayloadRefHash        string
	TargetRefHash         string
	SupportingMemoryIDs   []string
	RAGEvidence           bool
	AgentEvidence         bool
	SummaryTextSHA256     string
	RepairBatchManifest   string
	RequestSummaryPath    string
	ExecuteSummaryPath    string
	DecisionID            string
}

type profileRepairNegativeCaseSummary struct {
	Name         string `json:"name"`
	ExpectedFail string `json:"expected_fail"`
	Passed       bool   `json:"passed"`
}

type memoryProfileOperatorSummary struct {
	Executed                   bool            `json:"executed"`
	ResultDir                  string          `json:"result_dir"`
	TenantID                   string          `json:"tenant_id"`
	UserID                     string          `json:"user_id"`
	BatchMode                  bool            `json:"batch_mode"`
	BatchPayloadRefHash        string          `json:"batch_payload_ref_hash"`
	BatchTargetRefHash         string          `json:"batch_target_ref_hash"`
	ApprovalRequired           bool            `json:"approval_required"`
	ApprovalRequested          bool            `json:"approval_requested,omitempty"`
	ApprovalVerified           bool            `json:"approval_verified,omitempty"`
	ApprovalWorkflowID         string          `json:"approval_workflow_id,omitempty"`
	ApprovalWorkflowType       string          `json:"approval_workflow_type,omitempty"`
	ApprovalWorkflowStatus     string          `json:"approval_workflow_status,omitempty"`
	ApprovalWorkflowPayloadRef string          `json:"approval_workflow_payload_ref_hash,omitempty"`
	ApprovalWorkflowTargetRef  string          `json:"approval_workflow_target_ref_hash,omitempty"`
	Targets                    []targetSummary `json:"targets,omitempty"`
	Success                    bool            `json:"success"`
	Error                      string          `json:"error,omitempty"`
	Active                     bool            `json:"active,omitempty"`
	SupportCount               int32           `json:"support_count,omitempty"`
	SupportingMemoryCount      int             `json:"supporting_memory_count,omitempty"`
	SupportingMemoryIDHashes   []string        `json:"supporting_memory_id_hashes,omitempty"`
	SummaryTextSHA256          string          `json:"summary_text_sha256,omitempty"`
}

type targetSummary struct {
	Success                  bool     `json:"success"`
	Error                    string   `json:"error,omitempty"`
	Active                   bool     `json:"active,omitempty"`
	SupportCount             int32    `json:"support_count,omitempty"`
	SupportingMemoryCount    int      `json:"supporting_memory_count,omitempty"`
	SupportingMemoryIDHashes []string `json:"supporting_memory_id_hashes,omitempty"`
	SummaryTextSHA256        string   `json:"summary_text_sha256,omitempty"`
}

func verifyProfileRepairApproval(
	ctx context.Context,
	cfg config,
	seed seedSummary,
	resultDir string,
) (profileRepairApprovalSummary, error) {
	conversationSeq := seed.CurrentMemoryAtSeq
	if conversationSeq <= 0 {
		conversationSeq = seed.ConversationSeq
	}
	if conversationSeq <= 0 {
		return profileRepairApprovalSummary{}, errors.New("profile repair approval requires a positive conversation seq")
	}
	suffix, err := randomSuffix()
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}
	signalIDs := []string{
		"ragagent-profile-repair-signal-" + suffix + "-1",
		"ragagent-profile-repair-signal-" + suffix + "-2",
	}
	signalFacts := []string{
		"profile_signal: viewer coordinates phoenix launch repair approvals across groups",
		"profile_signal: viewer resolves phoenix launch repair blockers with audited workflow approval",
	}

	memoryConn, err := grpc.NewClient("passthrough:///"+cfg.memoryTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}
	defer memoryConn.Close()
	memoryClient := memoryv1.NewMemoryServiceClient(memoryConn)
	for index, signalID := range signalIDs {
		if err := submitAndApproveProfileSignal(ctx, cfg, memoryClient, seed, signalID, signalFacts[index], conversationSeq+int64(index)+2); err != nil {
			return profileRepairApprovalSummary{}, err
		}
	}

	batchPath, err := writeProfileRepairBatchManifest(resultDir, seed.ViewerUserID)
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}

	requestRunName := cfg.runName + "-profile-repair-request"
	if err := runChild(ctx, "memory-profile-request", memoryProfileArgs(cfg, seed, requestRunName, batchPath, "", true)); err != nil {
		return profileRepairApprovalSummary{}, err
	}
	requestSummaryPath := filepath.Join(cfg.resultRoot, sanitizeRunName(requestRunName), "memory-profile-operator-summary.json")
	requestSummary, err := readJSON[memoryProfileOperatorSummary](requestSummaryPath)
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}
	if err := verifyProfileRepairRequestSummary(cfg, requestSummary); err != nil {
		return profileRepairApprovalSummary{}, err
	}

	negativeCases := make([]profileRepairNegativeCaseSummary, 0, 2)
	unapprovedRunName := cfg.runName + "-profile-repair-negative-unapproved"
	if err := runMemoryProfileExpectFailure(
		ctx,
		"memory-profile-negative-unapproved",
		memoryProfileArgs(cfg, seed, unapprovedRunName, batchPath, requestSummary.ApprovalWorkflowID, false),
		"approval workflow must be APPROVED",
	); err != nil {
		return profileRepairApprovalSummary{}, err
	}
	negativeCases = append(negativeCases, profileRepairNegativeCaseSummary{
		Name:         "unapproved_workflow_execute",
		ExpectedFail: "approval workflow must be APPROVED",
		Passed:       true,
	})

	decisionID, err := approveProfileRepairWorkflow(ctx, cfg, seed.ViewerUserID, requestSummary.ApprovalWorkflowID)
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}

	tamperedBatchPath, err := writeProfileRepairBatchManifestForTarget(
		resultDir,
		"profile-repair-batch-tampered.json",
		seed.ViewerUserID,
		profileRepairAggregateType,
		profileRepairAggregateKey+"-tampered",
		2,
	)
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}
	tamperedRunName := cfg.runName + "-profile-repair-negative-hash-mismatch"
	if err := runMemoryProfileExpectFailure(
		ctx,
		"memory-profile-negative-hash-mismatch",
		memoryProfileArgs(cfg, seed, tamperedRunName, tamperedBatchPath, requestSummary.ApprovalWorkflowID, false),
		"approval workflow payload hash mismatch",
	); err != nil {
		return profileRepairApprovalSummary{}, err
	}
	negativeCases = append(negativeCases, profileRepairNegativeCaseSummary{
		Name:         "approval_payload_hash_mismatch",
		ExpectedFail: "approval workflow payload hash mismatch",
		Passed:       true,
	})

	executeRunName := cfg.runName + "-profile-repair-execute"
	if err := runChild(ctx, "memory-profile-execute", memoryProfileArgs(cfg, seed, executeRunName, batchPath, requestSummary.ApprovalWorkflowID, false)); err != nil {
		return profileRepairApprovalSummary{}, err
	}
	executeSummaryPath := filepath.Join(cfg.resultRoot, sanitizeRunName(executeRunName), "memory-profile-operator-summary.json")
	executeSummary, err := readJSON[memoryProfileOperatorSummary](executeSummaryPath)
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}
	if err := verifyProfileRepairExecuteSummary(requestSummary, executeSummary); err != nil {
		return profileRepairApprovalSummary{}, err
	}

	profileSeq := conversationSeq + int64(len(signalIDs)) + 2
	ragResponse, err := answerProfileRepairQuestion(ctx, cfg, seed, profileSeq)
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}
	if err := verifyProfileRepairEvidencePack(ragResponse.GetEvidencePack(), seed.ViewerUserID, signalIDs); err != nil {
		return profileRepairApprovalSummary{}, fmt.Errorf("rag profile repair evidence: %w", err)
	}
	agentResponse, err := createProfileRepairProposal(ctx, cfg, seed, profileSeq)
	if err != nil {
		return profileRepairApprovalSummary{}, err
	}
	if err := verifyProfileRepairEvidencePack(agentResponse.GetEvidencePack(), seed.ViewerUserID, signalIDs); err != nil {
		return profileRepairApprovalSummary{}, fmt.Errorf("agent profile repair evidence: %w", err)
	}

	return profileRepairApprovalSummary{
		ApprovalRequested:     requestSummary.ApprovalRequested,
		WorkflowApproved:      true,
		ApprovalVerified:      executeSummary.ApprovalVerified,
		Executed:              executeSummary.Executed,
		NegativeCasesVerified: true,
		NegativeCases:         negativeCases,
		ProfileActive:         executeSummary.Targets[0].Active,
		SupportCount:          executeSummary.Targets[0].SupportCount,
		SupportingMemoryCount: executeSummary.Targets[0].SupportingMemoryCount,
		WorkflowID:            requestSummary.ApprovalWorkflowID,
		PayloadRefHash:        executeSummary.BatchPayloadRefHash,
		TargetRefHash:         executeSummary.BatchTargetRefHash,
		SupportingMemoryIDs:   signalIDs,
		RAGEvidence:           true,
		AgentEvidence:         true,
		SummaryTextSHA256:     executeSummary.Targets[0].SummaryTextSHA256,
		RepairBatchManifest:   batchPath,
		RequestSummaryPath:    requestSummaryPath,
		ExecuteSummaryPath:    executeSummaryPath,
		DecisionID:            decisionID,
	}, nil
}

func submitAndApproveProfileSignal(
	ctx context.Context,
	cfg config,
	client memoryv1.MemoryServiceClient,
	seed seedSummary,
	candidateID string,
	factText string,
	conversationSeq int64,
) error {
	sourceID := "msg-" + candidateID
	sourceEventID := "evt-" + candidateID
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	submitted, err := client.SubmitMemoryCandidate(requestCtx, &memoryv1.SubmitMemoryCandidateRequest{
		AuthContext:    memoryAuth(cfg, seed.ViewerUserID),
		CandidateId:    candidateID,
		Scope:          memoryv1.MemoryScope_MEMORY_SCOPE_CONVERSATION,
		ScopeId:        seed.ConversationID,
		ConversationId: seed.ConversationID,
		Topic:          profileRepairAggregateKey,
		EventType:      memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_PROFILE_SIGNAL,
		FactText:       factText,
		FactSha256:     normalizedFactSHA256(factText),
		ActorUserIds:   []string{seed.ViewerUserID},
		SourceRefs: []*memoryv1.SourceRef{{
			SourceType:       memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_MESSAGE,
			SourceId:         sourceID,
			SourceEventId:    sourceEventID,
			ConversationId:   seed.ConversationID,
			ConversationSeq:  conversationSeq,
			OccurredAtUnixMs: time.Now().UTC().UnixMilli(),
		}},
		ValidFromSeq:      conversationSeq,
		Confidence:        0.97,
		VisibilityVersion: 1,
		ExtractionVersion: "memory-extraction-candidate-v1",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("submit profile signal candidate %s: %w", candidateID, err)
	}
	if submitted.GetItem().GetStatus() != memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_PENDING ||
		submitted.GetItem().GetReviewState() != memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_NEEDS_REVIEW {
		return fmt.Errorf("profile signal candidate should require review: %+v", submitted.GetItem())
	}
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	approved, err := client.ReviewMemoryCandidate(requestCtx, &memoryv1.ReviewMemoryCandidateRequest{
		AuthContext:   memoryAuth(cfg, seed.ViewerUserID),
		MemoryEventId: candidateID,
		Decision:      memoryv1.MemoryReviewDecision_MEMORY_REVIEW_DECISION_APPROVE,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("approve profile signal candidate %s: %w", candidateID, err)
	}
	if approved.GetItem().GetStatus() != memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE ||
		approved.GetItem().GetReviewState() != memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_APPROVED ||
		approved.GetItem().GetEventType() != memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_PROFILE_SIGNAL {
		return fmt.Errorf("approved profile signal should be active profile evidence: %+v", approved.GetItem())
	}
	return nil
}

func writeProfileRepairBatchManifest(resultDir string, subjectUserID string) (string, error) {
	return writeProfileRepairBatchManifestForTarget(
		resultDir,
		"profile-repair-batch.json",
		subjectUserID,
		profileRepairAggregateType,
		profileRepairAggregateKey,
		2,
	)
}

func writeProfileRepairBatchManifestForTarget(
	resultDir string,
	fileName string,
	subjectUserID string,
	aggregateType string,
	aggregateKey string,
	minSupportCount int,
) (string, error) {
	path := filepath.Join(resultDir, fileName)
	manifest := struct {
		SchemaVersion string `json:"schema_version"`
		Targets       []struct {
			SubjectUserID   string `json:"subject_user_id"`
			AggregateType   string `json:"aggregate_type"`
			AggregateKey    string `json:"aggregate_key"`
			MinSupportCount int    `json:"min_support_count"`
		} `json:"targets"`
	}{
		SchemaVersion: profileRepairBatchSchemaVersion,
		Targets: []struct {
			SubjectUserID   string `json:"subject_user_id"`
			AggregateType   string `json:"aggregate_type"`
			AggregateKey    string `json:"aggregate_key"`
			MinSupportCount int    `json:"min_support_count"`
		}{{
			SubjectUserID:   subjectUserID,
			AggregateType:   aggregateType,
			AggregateKey:    aggregateKey,
			MinSupportCount: minSupportCount,
		}},
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func memoryProfileArgs(cfg config, seed seedSummary, runName string, batchPath string, workflowID string, requestApproval bool) []string {
	args := []string{
		"run", "./loadtest/memoryprofile",
		"--memory-target", cfg.memoryTarget,
		"--workflow-target", cfg.workflowTarget,
		"--result-root", cfg.resultRoot,
		"--run-name", sanitizeRunName(runName),
		"--tenant-id", cfg.tenantID,
		"--user-id", seed.ViewerUserID,
		"--device-id", cfg.deviceID,
		"--batch-file", batchPath,
		"--request-timeout", cfg.requestTimeout.String(),
	}
	args = appendTLSArgs(args, "workflow", cfg.workflowTLS)
	if requestApproval {
		args = append(args,
			"--request-approval",
			"--approval-reason-ref", "reason:rag-agent-profile-repair",
			"--approval-evidence-refs", "evidence:rag-agent-profile-repair",
			"--approval-requester-ref", "operator:rag-agent-demo",
		)
		return args
	}
	return append(args,
		"--execute",
		"--approval-workflow-id", workflowID,
	)
}

func verifyProfileRepairRequestSummary(cfg config, summary memoryProfileOperatorSummary) error {
	if !summary.Success || summary.Error != "" {
		return fmt.Errorf("profile repair approval request failed: %s", summary.Error)
	}
	if summary.Executed {
		return errors.New("profile repair approval request must not execute repair")
	}
	if !summary.BatchMode || !summary.ApprovalRequested || summary.ApprovalWorkflowID == "" {
		return fmt.Errorf("profile repair approval request did not create workflow: %+v", summary)
	}
	if summary.TenantID != cfg.tenantID || summary.UserID == "" {
		return fmt.Errorf("profile repair approval request identity mismatch: %+v", summary)
	}
	if summary.ApprovalWorkflowType != profileRepairWorkflowType ||
		summary.ApprovalWorkflowPayloadRef == "" ||
		summary.ApprovalWorkflowTargetRef == "" ||
		summary.BatchPayloadRefHash != summary.ApprovalWorkflowPayloadRef ||
		summary.BatchTargetRefHash != summary.ApprovalWorkflowTargetRef {
		return fmt.Errorf("profile repair approval request hash/type mismatch: %+v", summary)
	}
	return nil
}

func verifyProfileRepairExecuteSummary(request memoryProfileOperatorSummary, summary memoryProfileOperatorSummary) error {
	if !summary.Success || summary.Error != "" {
		return fmt.Errorf("profile repair execute failed: %s", summary.Error)
	}
	if !summary.Executed || !summary.ApprovalVerified {
		return fmt.Errorf("profile repair execute did not verify approval: %+v", summary)
	}
	if summary.ApprovalWorkflowID != request.ApprovalWorkflowID ||
		summary.ApprovalWorkflowStatus != "APPROVED" ||
		summary.ApprovalWorkflowType != profileRepairWorkflowType {
		return fmt.Errorf("profile repair execute workflow mismatch: %+v", summary)
	}
	if summary.BatchPayloadRefHash != request.BatchPayloadRefHash ||
		summary.BatchTargetRefHash != request.BatchTargetRefHash ||
		summary.ApprovalWorkflowPayloadRef != request.BatchPayloadRefHash ||
		summary.ApprovalWorkflowTargetRef != request.BatchTargetRefHash {
		return fmt.Errorf("profile repair execute hash mismatch: request=%+v execute=%+v", request, summary)
	}
	if len(summary.Targets) != 1 || !summary.Targets[0].Success || !summary.Targets[0].Active {
		return fmt.Errorf("profile repair execute target summary mismatch: %+v", summary.Targets)
	}
	target := summary.Targets[0]
	if target.SupportCount < 2 || target.SupportingMemoryCount < 2 || target.SummaryTextSHA256 == "" {
		return fmt.Errorf("profile repair execute did not produce active profile aggregate: %+v", target)
	}
	return nil
}

func approveProfileRepairWorkflow(ctx context.Context, cfg config, userID string, workflowID string) (string, error) {
	dialOption, err := dialOptionFromTLSFlags(cfg.workflowTLS, "workflow-tls")
	if err != nil {
		return "", err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.workflowTarget, dialOption)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	client := workflowv1.NewWorkflowServiceClient(conn)
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	workflow, err := client.GetWorkflow(requestCtx, &workflowv1.GetWorkflowRequest{
		AuthContext: workflowAuth(cfg, userID),
		WorkflowId:  workflowID,
	})
	cancel()
	if err != nil {
		return "", fmt.Errorf("get profile repair workflow: %w", err)
	}
	if workflow.GetWorkflow() == nil || workflow.GetWorkflow().GetCurrentStepId() == "" {
		return "", errors.New("profile repair workflow missing current step")
	}
	if workflow.GetWorkflow().GetStatus() != "WAITING_DECISION" ||
		workflow.GetWorkflow().GetWorkflowType() != profileRepairWorkflowType ||
		workflow.GetWorkflow().GetTargetService() != profileRepairTargetService ||
		workflow.GetWorkflow().GetTargetOperation() != profileRepairTargetOperation {
		return "", fmt.Errorf("profile repair workflow not waiting for expected decision: %+v", workflow.GetWorkflow())
	}
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	decision, err := client.RecordWorkflowDecision(requestCtx, &workflowv1.RecordWorkflowDecisionRequest{
		AuthContext:       workflowAuth(cfg, userID),
		WorkflowId:        workflowID,
		StepId:            workflow.GetWorkflow().GetCurrentStepId(),
		DecisionType:      "APPROVE",
		DeciderRef:        "operator:rag-agent-demo-profile-repair",
		DecisionPolicyRef: "memory.profile.repair.batch.approval.demo.v1",
		ReasonRef:         "reason:rag-agent-profile-repair",
		EvidenceRefs:      []string{"evidence:rag-agent-profile-repair"},
		IdempotencyKey:    "rag-agent-profile-repair-approval:" + workflowID,
		CorrelationId:     "rag-agent-profile-repair",
		CausationId:       workflowID,
		TraceId:           "rag-agent-profile-repair",
	})
	cancel()
	if err != nil {
		return "", fmt.Errorf("approve profile repair workflow: %w", err)
	}
	if decision.GetWorkflow().GetStatus() != "APPROVED" || decision.GetDecision() == nil ||
		decision.GetDecision().GetDecisionType() != "APPROVE" {
		return "", fmt.Errorf("profile repair approval decision mismatch: %+v", decision)
	}
	return decision.GetDecision().GetDecisionId(), nil
}

func workflowAuth(cfg config, userID string) *workflowv1.AuthContext {
	return &workflowv1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      userID,
		ServiceName: "rag-agent-demo-profile-repair",
		InstanceRef: "rag-agent-demo",
		TraceId:     "rag-agent-profile-repair",
		RequestId:   "rag-agent-profile-repair",
	}
}

func answerProfileRepairQuestion(
	ctx context.Context,
	cfg config,
	seed seedSummary,
	conversationSeq int64,
) (*ragv1.AnswerQuestionResponse, error) {
	dialOption, err := dialOptionFromTLSFlags(cfg.ragTLS, "rag-tls")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.ragTarget, dialOption)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := ragv1.NewRagServiceClient(conn).AnswerQuestion(requestCtx, &ragv1.AnswerQuestionRequest{
		AuthContext:       retrievalAuth(cfg, seed.ViewerUserID),
		Question:          "profile repair approval",
		ConversationId:    seed.ConversationID,
		Limit:             10,
		IncludeSearch:     false,
		IncludeMemory:     true,
		MemoryStatuses:    []retrievalv1.EvidenceMemoryStatus{retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE},
		AtConversationSeq: conversationSeq,
	})
	if err != nil {
		return nil, fmt.Errorf("answer profile repair question: %w", err)
	}
	if response.GetStatus() != ragv1.AnswerStatus_ANSWER_STATUS_GROUNDED {
		return nil, fmt.Errorf("profile repair RAG answer status %v, want GROUNDED", response.GetStatus())
	}
	return response, nil
}

func createProfileRepairProposal(
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
		Objective:         "profile repair approval",
		ToolName:          defaultAgentToolName,
		ToolAction:        policyv1.ToolAction_TOOL_ACTION_CALL,
		ResourceType:      defaultAgentResourceType,
		ResourceId:        seed.ConversationID,
		RiskLevel:         "LOW",
		Intent:            "profile repair approval",
		Limit:             10,
		IncludeSearch:     false,
		IncludeMemory:     true,
		MemoryStatuses:    []retrievalv1.EvidenceMemoryStatus{retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE},
		SkillId:           defaultAgentSkillID,
		AtConversationSeq: conversationSeq,
	})
	if err != nil {
		return nil, fmt.Errorf("create profile repair proposal: %w", err)
	}
	if response.GetStatus() != agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_PROPOSED {
		return nil, fmt.Errorf("profile repair agent proposal status %v, want PROPOSED", response.GetStatus())
	}
	if !response.GetRequiresApproval() {
		return nil, errors.New("profile repair agent proposal should require approval")
	}
	return response, nil
}

func verifyProfileRepairEvidencePack(pack *retrievalv1.EvidencePack, subjectUserID string, supportingMemoryIDs []string) error {
	if pack == nil {
		return errors.New("missing EvidencePack")
	}
	seenMatchingProfile := false
	for _, item := range pack.GetItems() {
		if item.GetSourceType() != retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_PROFILE_AGGREGATE {
			continue
		}
		if item.GetProfileSubjectUserId() != subjectUserID ||
			item.GetProfileAggregateType() != profileRepairAggregateType ||
			item.GetProfileAggregateKey() != profileRepairAggregateKey {
			continue
		}
		if item.GetProfileId() == "" || item.GetText() == "" || item.GetProfileUpdatedAtUnixMs() <= 0 {
			return fmt.Errorf("profile repair EvidencePack item is incomplete: %+v", item)
		}
		seenMatchingProfile = true
		required := make(map[string]bool, len(supportingMemoryIDs))
		for _, id := range supportingMemoryIDs {
			required[id] = false
		}
		for _, id := range item.GetSupportingMemoryEventIds() {
			if _, ok := required[id]; ok {
				required[id] = true
			}
		}
		allFound := true
		for id, found := range required {
			if !found {
				allFound = false
				_ = id
				break
			}
		}
		if allFound {
			return nil
		}
	}
	if seenMatchingProfile {
		return fmt.Errorf("profile repair profile aggregate evidence found, but missing required supporting memories %v", supportingMemoryIDs)
	}
	return fmt.Errorf("profile repair profile aggregate evidence missing for subject %q", subjectUserID)
}
