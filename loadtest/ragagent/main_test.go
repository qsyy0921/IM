package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.pgDSN != defaultPGDSN {
		t.Fatalf("unexpected pg dsn %q", cfg.pgDSN)
	}
	if cfg.memoryTarget != defaultMemoryTarget {
		t.Fatalf("unexpected memory target %q", cfg.memoryTarget)
	}
	if cfg.ragTarget != defaultRAGTarget {
		t.Fatalf("unexpected rag target %q", cfg.ragTarget)
	}
	if cfg.agentTarget != defaultAgentTarget {
		t.Fatalf("unexpected agent target %q", cfg.agentTarget)
	}
	if cfg.actionTarget != defaultActionTarget {
		t.Fatalf("unexpected action target %q", cfg.actionTarget)
	}
	if cfg.workflowTarget != defaultWorkflowTarget {
		t.Fatalf("unexpected workflow target %q", cfg.workflowTarget)
	}
	if cfg.resultRoot != defaultResultRoot {
		t.Fatalf("unexpected result root %q", cfg.resultRoot)
	}
	if cfg.tenantID == "" || cfg.conversationID == "" {
		t.Fatalf("expected generated tenant and conversation ids")
	}
	if cfg.expectExecuted {
		t.Fatalf("default demo should not require tool execution")
	}
}

func TestParseConfigRejectsMissingTargets(t *testing.T) {
	cases := [][]string{
		{"--memory-target", " "},
		{"--rag-target", " "},
		{"--agent-target", " "},
		{"--action-executor-target", " "},
		{"--workflow-target", " "},
		{"--result-root", " "},
	}
	for _, args := range cases {
		if _, err := parseConfig(args); err == nil {
			t.Fatalf("expected %v to fail", args)
		}
	}
}

func TestPathInside(t *testing.T) {
	root := filepath.Join("E:", "development", "IM")
	inside := filepath.Join(root, "loadtest", "ragagent")
	outside := filepath.Join("H:", "NexusIM", "loadtest-results")
	if !pathInside(inside, root) {
		t.Fatalf("expected %q inside %q", inside, root)
	}
	if pathInside(outside, root) {
		t.Fatalf("did not expect %q inside %q", outside, root)
	}
}

func TestChildArgsShareTenantConversation(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--tenant-id", "tenant-demo",
		"--conversation-id", "conv-demo",
		"--viewer-user-id", "viewer-demo",
		"--sender-user-id", "sender-demo",
		"--request-timeout", "3s",
		"--expect-executed",
	})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	rag := strings.Join(ragArgs(cfg, "demo-rag"), "\n")
	agent := strings.Join(agentArgs(cfg, "demo-agent"), "\n")
	for _, needle := range []string{"tenant-demo", "conv-demo", "viewer-demo", "sender-demo"} {
		if !strings.Contains(rag, needle) {
			t.Fatalf("rag args missing %q: %s", needle, rag)
		}
		if !strings.Contains(agent, needle) {
			t.Fatalf("agent args missing %q: %s", needle, agent)
		}
	}
	if !strings.Contains(agent, "nexusim.local.echo") {
		t.Fatalf("expected safe local tool args when --expect-executed is set: %s", agent)
	}
}

func TestVerifyCombinedSummary(t *testing.T) {
	startedAt := time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC)
	cfg := config{runName: "demo", expectExecuted: true}
	rag := validRAGPartial()
	agent := validAgentPartial()
	summary, err := verifyCombined(cfg, `H:\NexusIM\loadtest-results\demo`, "rag.json", "agent.json", rag, agent, validPublicCandidateReviewSummary(), validProfileRepairApprovalSummary(), startedAt)
	if err != nil {
		t.Fatalf("verifyCombined returned error: %v", err)
	}
	if !summary.RAGAnswered || !summary.AgentProposalCreated || !summary.AgentApprovalRecorded || !summary.ActionExecutionRecorded {
		t.Fatalf("missing combined success flags: %+v", summary)
	}
	if !summary.ActionExecuted || !summary.ActionResultRecorded {
		t.Fatalf("expected executed action and result record: %+v", summary)
	}
	if summary.RAGAnswerTextSHA256 == "" || summary.AgentProposalTextSHA256 == "" {
		t.Fatalf("expected hashed text fields: %+v", summary)
	}
	if !summary.PublicCandidateReviewApproved ||
		!summary.PublicCandidateEvidenceInRAG ||
		!summary.PublicCandidateEvidenceInAgent ||
		!summary.PublicCandidateTemporalUpdatePreserved ||
		summary.PublicCandidateSupersededMemoryEventID == "" {
		t.Fatalf("expected public candidate temporal update evidence flags: %+v", summary)
	}
	if !summary.ProfileRepairApprovalRequested ||
		!summary.ProfileRepairWorkflowApproved ||
		!summary.ProfileRepairApprovalVerified ||
		!summary.ProfileRepairExecuted ||
		!summary.ProfileRepairProfileActive ||
		summary.ProfileRepairSupportCount < 2 ||
		summary.ProfileRepairSupportingMemoryCount < 2 ||
		!summary.ProfileRepairRAGEvidence ||
		!summary.ProfileRepairAgentEvidence ||
		summary.ProfileRepairWorkflowID == "" {
		t.Fatalf("expected profile repair approval and evidence flags: %+v", summary)
	}
	if strings.Contains(summary.RAGAnswerTextSHA256, rag.AnswerText) ||
		strings.Contains(summary.AgentProposalTextSHA256, agent.ProposalText) {
		t.Fatalf("summary hash fields must not include raw answer or proposal")
	}
	if len(summary.Verified) < 4 {
		t.Fatalf("expected verification details: %+v", summary.Verified)
	}
}

func TestVerifyCombinedRejectsDifferentConversation(t *testing.T) {
	cfg := config{runName: "demo"}
	rag := validRAGPartial()
	agent := validAgentPartial()
	agent.Seed.ConversationID = "other-conv"
	if _, err := verifyCombined(cfg, "out", "rag.json", "agent.json", rag, agent, validPublicCandidateReviewSummary(), validProfileRepairApprovalSummary(), time.Now().UTC()); err == nil {
		t.Fatalf("expected conversation mismatch to fail")
	}
}

func TestVerifyCombinedRejectsMissingEvidenceBoundary(t *testing.T) {
	cfg := config{runName: "demo"}
	rag := validRAGPartial()
	agent := validAgentPartial()
	agent.MemoryGraphEdgesPreserved = false
	if _, err := verifyCombined(cfg, "out", "rag.json", "agent.json", rag, agent, validPublicCandidateReviewSummary(), validProfileRepairApprovalSummary(), time.Now().UTC()); err == nil {
		t.Fatalf("expected missing graph edge preservation to fail")
	}
}

func TestVerifyCombinedRejectsMissingPublicCandidateReview(t *testing.T) {
	cfg := config{runName: "demo"}
	rag := validRAGPartial()
	agent := validAgentPartial()
	candidate := validPublicCandidateReviewSummary()
	candidate.AgentEvidence = false
	if _, err := verifyCombined(cfg, "out", "rag.json", "agent.json", rag, agent, candidate, validProfileRepairApprovalSummary(), time.Now().UTC()); err == nil {
		t.Fatalf("expected missing public candidate evidence to fail")
	}
}

func TestVerifyCombinedRejectsMissingProfileRepairApproval(t *testing.T) {
	cfg := config{runName: "demo"}
	rag := validRAGPartial()
	agent := validAgentPartial()
	profileRepair := validProfileRepairApprovalSummary()
	profileRepair.ApprovalVerified = false
	if _, err := verifyCombined(cfg, "out", "rag.json", "agent.json", rag, agent, validPublicCandidateReviewSummary(), profileRepair, time.Now().UTC()); err == nil {
		t.Fatalf("expected missing profile repair approval verification to fail")
	}
}

func validRAGPartial() ragPartialSummary {
	return ragPartialSummary{
		Seed: seedSummary{
			TenantID:       "tenant",
			ConversationID: "conv",
			ViewerUserID:   "viewer",
		},
		AnswerID:                              "answer-1",
		AnswerStatus:                          "GROUNDED",
		AnswerText:                            "grounded answer text",
		CitationCount:                         2,
		EvidenceItemCount:                     3,
		CrossGroupSourceRefsPreserved:         true,
		CrossGroupSpeakerAttributionPreserved: true,
		MemoryGraphEdgesPreserved:             true,
		ProfileAggregatePreserved:             true,
		RAGVersion:                            "rag-v",
		RetrievalVersion:                      "retrieval-v",
	}
}

func validAgentPartial() agentPartialSummary {
	return agentPartialSummary{
		Seed: seedSummary{
			TenantID:       "tenant",
			ConversationID: "conv",
			ViewerUserID:   "viewer",
		},
		ProposalID:                            "proposal-1",
		PreparedAuditID:                       "prepared-1",
		ProposalStatus:                        "APPROVED",
		ApprovalID:                            "approval-1",
		ExecutionID:                           "execution-1",
		ExecutionStatus:                       "RECORDED",
		ExecutionExecuted:                     true,
		ExecutionResultID:                     "result-1",
		ProposalText:                          "proposal text",
		RequiresApproval:                      true,
		CitationCount:                         2,
		EvidenceItemCount:                     3,
		CrossGroupSourceRefsPreserved:         true,
		CrossGroupSpeakerAttributionPreserved: true,
		MemoryGraphEdgesPreserved:             true,
		ProfileAggregatePreserved:             true,
		AgentVersion:                          "agent-v",
		RetrievalVersion:                      "retrieval-v",
	}
}

func validPublicCandidateReviewSummary() publicCandidateReviewSummary {
	return publicCandidateReviewSummary{
		Approved:                true,
		MemoryEventID:           "candidate-1-replacement",
		SupersededMemoryEventID: "candidate-1",
		FactSHA256:              strings.Repeat("a", 64),
		RAGEvidence:             true,
		AgentEvidence:           true,
		TemporalUpdatePreserved: true,
	}
}

func validProfileRepairApprovalSummary() profileRepairApprovalSummary {
	return profileRepairApprovalSummary{
		ApprovalRequested:     true,
		WorkflowApproved:      true,
		ApprovalVerified:      true,
		Executed:              true,
		ProfileActive:         true,
		SupportCount:          2,
		SupportingMemoryCount: 2,
		WorkflowID:            "wf-profile-repair",
		PayloadRefHash:        "sha256:" + strings.Repeat("b", 64),
		TargetRefHash:         "sha256:" + strings.Repeat("c", 64),
		SupportingMemoryIDs:   []string{"profile-signal-1", "profile-signal-2"},
		RAGEvidence:           true,
		AgentEvidence:         true,
		SummaryTextSHA256:     strings.Repeat("d", 64),
	}
}
