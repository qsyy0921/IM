package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentv1 "github.com/qsyy0921/IM/api/proto/nexusim/agent/v1"
	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	ragv1 "github.com/qsyy0921/IM/api/proto/nexusim/rag/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const groupMemoryScenarioQuery = "phoenix launch group memory chain"

type groupMemoryScenarioSummary struct {
	AnswerVerified        bool
	ProposalVerified      bool
	MemoryEventCount      int
	RAGMemoryEventCount   int
	AgentMemoryEventCount int
	EventTypes            []string
	FactSHA256            []string
	SourceRefCount        int
	CrossGroupSourceRefs  int
}

type groupMemoryCandidate struct {
	EventID            string
	EventType          memoryv1.MemoryEventType
	EventTypeName      string
	FactText           string
	FactSHA256         string
	SourceID           string
	SourceEventID      string
	CrossSourceID      string
	CrossSourceEventID string
	ValidFromSeq       int64
}

func verifyGroupMemoryAnswerProposalScenario(
	ctx context.Context,
	cfg config,
	seed seedSummary,
) (groupMemoryScenarioSummary, error) {
	conversationSeq := seed.CurrentMemoryAtSeq
	if conversationSeq <= 0 {
		conversationSeq = seed.ConversationSeq
	}
	if conversationSeq <= 0 {
		return groupMemoryScenarioSummary{}, errors.New("group memory scenario requires a positive conversation seq")
	}
	if seed.CrossGroupConversationID == "" {
		return groupMemoryScenarioSummary{}, errors.New("group memory scenario requires a cross-group conversation id")
	}
	suffix, err := randomSuffix()
	if err != nil {
		return groupMemoryScenarioSummary{}, err
	}
	candidates := buildGroupMemoryCandidates(suffix, conversationSeq)

	memoryConn, err := grpc.NewClient("passthrough:///"+cfg.memoryTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return groupMemoryScenarioSummary{}, err
	}
	defer memoryConn.Close()
	memoryClient := memoryv1.NewMemoryServiceClient(memoryConn)
	for _, candidate := range candidates {
		if err := submitAndApproveGroupMemoryCandidate(ctx, cfg, memoryClient, seed, candidate); err != nil {
			return groupMemoryScenarioSummary{}, err
		}
	}

	querySeq := candidates[len(candidates)-1].ValidFromSeq + 1
	ragResponse, err := answerGroupMemoryQuestion(ctx, cfg, seed, querySeq)
	if err != nil {
		return groupMemoryScenarioSummary{}, err
	}
	ragCount, sourceRefCount, crossRefCount, err := verifyGroupMemoryEvidencePack(ragResponse.GetEvidencePack(), seed, candidates)
	if err != nil {
		return groupMemoryScenarioSummary{}, fmt.Errorf("rag group memory evidence: %w", err)
	}
	agentResponse, err := createGroupMemoryProposal(ctx, cfg, seed, querySeq)
	if err != nil {
		return groupMemoryScenarioSummary{}, err
	}
	agentCount, agentSourceRefCount, agentCrossRefCount, err := verifyGroupMemoryEvidencePack(agentResponse.GetEvidencePack(), seed, candidates)
	if err != nil {
		return groupMemoryScenarioSummary{}, fmt.Errorf("agent group memory evidence: %w", err)
	}
	if agentSourceRefCount < sourceRefCount {
		sourceRefCount = agentSourceRefCount
	}
	if agentCrossRefCount < crossRefCount {
		crossRefCount = agentCrossRefCount
	}

	return groupMemoryScenarioSummary{
		AnswerVerified:        true,
		ProposalVerified:      true,
		MemoryEventCount:      len(candidates),
		RAGMemoryEventCount:   ragCount,
		AgentMemoryEventCount: agentCount,
		EventTypes:            groupMemoryEventTypes(candidates),
		FactSHA256:            groupMemoryFactHashes(candidates),
		SourceRefCount:        sourceRefCount,
		CrossGroupSourceRefs:  crossRefCount,
	}, nil
}

func buildGroupMemoryCandidates(suffix string, conversationSeq int64) []groupMemoryCandidate {
	facts := []struct {
		eventType     memoryv1.MemoryEventType
		eventTypeName string
		factText      string
	}{
		{
			eventType:     memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_DECISION,
			eventTypeName: "DECISION",
			factText:      "decision: phoenix launch group memory chain requires audited rollout ownership before release",
		},
		{
			eventType:     memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_BLOCKER,
			eventTypeName: "BLOCKER",
			factText:      "blocker: phoenix launch group memory chain waits for security review before production rollout",
		},
		{
			eventType:     memoryv1.MemoryEventType_MEMORY_EVENT_TYPE_FILE,
			eventTypeName: "FILE",
			factText:      "file: phoenix launch group memory chain evidence is tracked in launch-readiness.md",
		},
	}
	candidates := make([]groupMemoryCandidate, 0, len(facts))
	for index, fact := range facts {
		id := fmt.Sprintf("ragagent-group-memory-%s-%d", suffix, index+1)
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
			ValidFromSeq:       conversationSeq + int64(index) + 20,
		})
	}
	return candidates
}

func submitAndApproveGroupMemoryCandidate(
	ctx context.Context,
	cfg config,
	client memoryv1.MemoryServiceClient,
	seed seedSummary,
	candidate groupMemoryCandidate,
) error {
	return submitAndApproveReviewedMemoryCandidate(
		ctx,
		cfg,
		client,
		seed,
		candidate,
		"rag-agent-group-memory-chain",
		"rag-agent-group-memory-v1",
	)
}

func submitAndApproveReviewedMemoryCandidate(
	ctx context.Context,
	cfg config,
	client memoryv1.MemoryServiceClient,
	seed seedSummary,
	candidate groupMemoryCandidate,
	topic string,
	extractionVersion string,
) error {
	now := time.Now().UTC().UnixMilli()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	submitted, err := client.SubmitMemoryCandidate(requestCtx, &memoryv1.SubmitMemoryCandidateRequest{
		AuthContext:     memoryAuth(cfg, seed.ViewerUserID),
		CandidateId:     candidate.EventID,
		Scope:           memoryv1.MemoryScope_MEMORY_SCOPE_CONVERSATION,
		ScopeId:         seed.ConversationID,
		ConversationId:  seed.ConversationID,
		Topic:           topic,
		EventType:       candidate.EventType,
		FactText:        candidate.FactText,
		FactSha256:      candidate.FactSHA256,
		ActorUserIds:    []string{seed.SenderUserID, seed.CrossGroupActorUserID},
		AudienceUserIds: []string{seed.ViewerUserID},
		SourceRefs: []*memoryv1.SourceRef{
			{
				SourceType:       memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_MESSAGE,
				SourceId:         candidate.SourceID,
				SourceEventId:    candidate.SourceEventID,
				ConversationId:   seed.ConversationID,
				ConversationSeq:  candidate.ValidFromSeq,
				OccurredAtUnixMs: now,
			},
			{
				SourceType:       memoryv1.MemorySourceType_MEMORY_SOURCE_TYPE_MESSAGE,
				SourceId:         candidate.CrossSourceID,
				SourceEventId:    candidate.CrossSourceEventID,
				ConversationId:   seed.CrossGroupConversationID,
				ConversationSeq:  candidate.ValidFromSeq + 1,
				OccurredAtUnixMs: now,
			},
		},
		ValidFromSeq:      candidate.ValidFromSeq,
		Confidence:        0.97,
		VisibilityVersion: 1,
		ExtractionVersion: extractionVersion,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("submit group memory candidate %s: %w", candidate.EventID, err)
	}
	if submitted.GetItem().GetStatus() != memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_PENDING ||
		submitted.GetItem().GetReviewState() != memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_NEEDS_REVIEW {
		return fmt.Errorf("submitted group memory candidate should require review: %+v", submitted.GetItem())
	}

	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	approved, err := client.ReviewMemoryCandidate(requestCtx, &memoryv1.ReviewMemoryCandidateRequest{
		AuthContext:   memoryAuth(cfg, seed.ViewerUserID),
		MemoryEventId: candidate.EventID,
		Decision:      memoryv1.MemoryReviewDecision_MEMORY_REVIEW_DECISION_APPROVE,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("approve group memory candidate %s: %w", candidate.EventID, err)
	}
	if approved.GetItem().GetStatus() != memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_ACTIVE ||
		approved.GetItem().GetReviewState() != memoryv1.MemoryReviewState_MEMORY_REVIEW_STATE_APPROVED ||
		approved.GetItem().GetEventType() != candidate.EventType {
		return fmt.Errorf("approved group memory candidate mismatch: %+v", approved.GetItem())
	}
	return nil
}

func answerGroupMemoryQuestion(
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
		Question:          groupMemoryScenarioQuery,
		ConversationId:    seed.ConversationID,
		Limit:             10,
		IncludeSearch:     false,
		IncludeMemory:     true,
		MemoryStatuses:    []retrievalv1.EvidenceMemoryStatus{retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE},
		AtConversationSeq: conversationSeq,
	})
	if err != nil {
		return nil, fmt.Errorf("answer group memory question: %w", err)
	}
	if response.GetStatus() != ragv1.AnswerStatus_ANSWER_STATUS_GROUNDED {
		return nil, fmt.Errorf("group memory RAG answer status %v, want GROUNDED", response.GetStatus())
	}
	return response, nil
}

func createGroupMemoryProposal(
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
		Objective:         groupMemoryScenarioQuery,
		ToolName:          defaultAgentToolName,
		ToolAction:        policyv1.ToolAction_TOOL_ACTION_CALL,
		ResourceType:      defaultAgentResourceType,
		ResourceId:        seed.ConversationID,
		RiskLevel:         "LOW",
		Intent:            groupMemoryScenarioQuery,
		Limit:             10,
		IncludeSearch:     false,
		IncludeMemory:     true,
		MemoryStatuses:    []retrievalv1.EvidenceMemoryStatus{retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE},
		SkillId:           defaultAgentSkillID,
		AtConversationSeq: conversationSeq,
	})
	if err != nil {
		return nil, fmt.Errorf("create group memory proposal: %w", err)
	}
	if response.GetStatus() != agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_PROPOSED {
		return nil, fmt.Errorf("group memory agent proposal status %v, want PROPOSED", response.GetStatus())
	}
	if !response.GetRequiresApproval() {
		return nil, errors.New("group memory agent proposal should require approval")
	}
	return response, nil
}

func verifyGroupMemoryEvidencePack(
	pack *retrievalv1.EvidencePack,
	seed seedSummary,
	candidates []groupMemoryCandidate,
) (int, int, int, error) {
	if pack == nil {
		return 0, 0, 0, errors.New("missing EvidencePack")
	}
	found := map[string]bool{}
	sourceRefCount := 0
	crossGroupRefCount := 0
	for _, item := range pack.GetItems() {
		if item.GetSourceType() != retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT {
			continue
		}
		expected, ok := groupMemoryCandidateByID(candidates, item.GetMemoryEventId())
		if !ok {
			continue
		}
		if item.GetTemporalStatus() != "ACTIVE" || item.GetReviewState() != "APPROVED" {
			return 0, 0, 0, fmt.Errorf("group memory item should be active and approved: %+v", item)
		}
		if item.GetValidFromSeq() != expected.ValidFromSeq {
			return 0, 0, 0, fmt.Errorf("group memory valid_from_seq %d, want %d", item.GetValidFromSeq(), expected.ValidFromSeq)
		}
		if !containsString(item.GetActorUserIds(), seed.SenderUserID) ||
			!containsString(item.GetActorUserIds(), seed.CrossGroupActorUserID) {
			return 0, 0, 0, fmt.Errorf("group memory item missing actor attribution: %+v", item.GetActorUserIds())
		}
		if !containsString(item.GetAudienceUserIds(), seed.ViewerUserID) {
			return 0, 0, 0, fmt.Errorf("group memory item missing viewer audience: %+v", item.GetAudienceUserIds())
		}
		primary, ok := findEvidenceSourceRef(item.GetSourceRefs(), expected.SourceID, expected.SourceEventID, seed.ConversationID)
		if !ok || primary.GetConversationSeq() != expected.ValidFromSeq {
			return 0, 0, 0, fmt.Errorf("group memory item missing primary source ref: %+v", item.GetSourceRefs())
		}
		cross, ok := findEvidenceSourceRef(item.GetSourceRefs(), expected.CrossSourceID, expected.CrossSourceEventID, seed.CrossGroupConversationID)
		if !ok || cross.GetConversationSeq() != expected.ValidFromSeq+1 {
			return 0, 0, 0, fmt.Errorf("group memory item missing cross-group source ref: %+v", item.GetSourceRefs())
		}
		found[expected.EventID] = true
		sourceRefCount += len(item.GetSourceRefs())
		crossGroupRefCount++
	}
	for _, expected := range candidates {
		if !found[expected.EventID] {
			return 0, 0, 0, fmt.Errorf("group memory event %q missing from EvidencePack", expected.EventID)
		}
	}
	return len(found), sourceRefCount, crossGroupRefCount, nil
}

func groupMemoryCandidateByID(candidates []groupMemoryCandidate, id string) (groupMemoryCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.EventID == id {
			return candidate, true
		}
	}
	return groupMemoryCandidate{}, false
}

func findEvidenceSourceRef(refs []*retrievalv1.EvidenceSourceRef, sourceID, sourceEventID, conversationID string) (*retrievalv1.EvidenceSourceRef, bool) {
	for _, ref := range refs {
		if ref.GetSourceId() == sourceID &&
			ref.GetSourceEventId() == sourceEventID &&
			ref.GetConversationId() == conversationID {
			return ref, true
		}
	}
	return nil, false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func groupMemoryEventTypes(candidates []groupMemoryCandidate) []string {
	values := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		values = append(values, candidate.EventTypeName)
	}
	return values
}

func groupMemoryFactHashes(candidates []groupMemoryCandidate) []string {
	values := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		values = append(values, candidate.FactSHA256)
	}
	return values
}
