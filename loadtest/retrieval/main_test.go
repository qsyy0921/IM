package main

import (
	"path/filepath"
	"testing"

	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
)

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.pgDSN != defaultPGDSN {
		t.Fatalf("unexpected pg dsn %q", cfg.pgDSN)
	}
	if cfg.retrievalTarget != defaultRetrievalTarget {
		t.Fatalf("unexpected retrieval target %q", cfg.retrievalTarget)
	}
	if cfg.resultRoot != defaultResultRoot {
		t.Fatalf("unexpected result root %q", cfg.resultRoot)
	}
	if cfg.tenantID == "" || cfg.conversationID == "" {
		t.Fatalf("expected generated tenant and conversation ids")
	}
}

func TestParseConfigRejectsMissingRetrievalTarget(t *testing.T) {
	if _, err := parseConfig([]string{"--retrieval-target", " "}); err == nil {
		t.Fatalf("expected missing retrieval target to fail")
	}
}

func TestParseConfigVectorBackendDefaults(t *testing.T) {
	cfg, err := parseConfig([]string{"--include-vector-backend"})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if !cfg.includeVectorBackend {
		t.Fatalf("expected vector backend enabled")
	}
	if cfg.vectorTarget != defaultVectorTarget {
		t.Fatalf("unexpected vector target %q", cfg.vectorTarget)
	}
	if cfg.vectorCollectionType != "MEMORY_EVENT" {
		t.Fatalf("unexpected vector collection type %q", cfg.vectorCollectionType)
	}
	if cfg.vectorVisibilityScope == "" || cfg.vectorPolicyVersion == "" {
		t.Fatalf("expected vector visibility and policy fields")
	}
	if cfg.queryEmbeddingRef == "" || cfg.queryEmbeddingRef[:7] != "sha256:" {
		t.Fatalf("expected low-sensitive query embedding ref, got %q", cfg.queryEmbeddingRef)
	}
}

func TestParseConfigRejectsMissingVectorTargetWhenEnabled(t *testing.T) {
	if _, err := parseConfig([]string{"--include-vector-backend", "--vector-target", " "}); err == nil {
		t.Fatalf("expected missing vector target to fail")
	}
}

func TestPathInside(t *testing.T) {
	root := filepath.Join("E:", "development", "IM")
	inside := filepath.Join(root, "loadtest", "retrieval")
	outside := filepath.Join("H:", "NexusIM", "loadtest-results")
	if !pathInside(inside, root) {
		t.Fatalf("expected %q inside %q", inside, root)
	}
	if pathInside(outside, root) {
		t.Fatalf("did not expect %q inside %q", outside, root)
	}
}

func TestVerifySourceCoverageRequiresReturnedCoreSources(t *testing.T) {
	coverage, err := verifySourceCoverage([]*retrievalv1.EvidenceSourceCoverage{
		coverageItem(retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE, true, 2, 1, retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED),
		coverageItem(retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT, true, 1, 1, retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED),
		coverageItem(retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_PROFILE_AGGREGATE, true, 1, 1, retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED),
		coverageItem(retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_VECTOR_ITEM, false, 0, 0, retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_NOT_REQUESTED),
	}, sourceCounts{SearchMessage: 1, MemoryEvent: 1, ProfileAggregate: 1}, false)
	if err != nil {
		t.Fatalf("verifySourceCoverage returned error: %v", err)
	}
	if len(coverage) != 4 {
		t.Fatalf("coverage len=%d want 4", len(coverage))
	}
	if coverage[0].SourceType != "SEARCH_MESSAGE" || coverage[0].Status != "RETURNED" {
		t.Fatalf("unexpected first coverage entry: %+v", coverage[0])
	}
}

func TestVerifySourceCoverageRejectsMissingReturnedSource(t *testing.T) {
	_, err := verifySourceCoverage([]*retrievalv1.EvidenceSourceCoverage{
		coverageItem(retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE, true, 1, 1, retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED),
		coverageItem(retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT, true, 0, 0, retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_EMPTY),
		coverageItem(retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_PROFILE_AGGREGATE, true, 1, 1, retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED),
		coverageItem(retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_VECTOR_ITEM, false, 0, 0, retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_NOT_REQUESTED),
	}, sourceCounts{SearchMessage: 1, MemoryEvent: 1, ProfileAggregate: 1}, false)
	if err == nil {
		t.Fatalf("expected missing memory returned coverage to fail")
	}
}

func coverageItem(
	sourceType retrievalv1.EvidenceSourceType,
	requested bool,
	candidateCount int32,
	returnedCount int32,
	status retrievalv1.EvidenceSourceCoverageStatus,
) *retrievalv1.EvidenceSourceCoverage {
	return &retrievalv1.EvidenceSourceCoverage{
		SourceType:     sourceType,
		Requested:      requested,
		CandidateCount: candidateCount,
		ReturnedCount:  returnedCount,
		Status:         status,
	}
}
