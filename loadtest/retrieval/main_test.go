package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestVerifyProviderCoverageReportsReadinessWithoutVectorBackend(t *testing.T) {
	path := writeProviderReadinessSummary(t, `{
  "phase": "preflight-provider-readiness",
  "provider_readiness": [
    {
      "provider": "opensearch-vector",
      "requested": true,
      "configured": true,
      "available": false,
      "status": "FAILED",
      "error": "opensearch vector index does not exist"
    }
  ]
}`)
	cfg := config{providerReadinessFile: path}
	coverage, err := verifyProviderCoverage(cfg, []sourceCoverageSummary{
		{SourceType: "VECTOR_ITEM", Requested: false, ReturnedCount: 0, Status: "NOT_REQUESTED"},
	})
	if err != nil {
		t.Fatalf("verifyProviderCoverage returned error: %v", err)
	}
	if len(coverage) != 1 {
		t.Fatalf("coverage len=%d want 1", len(coverage))
	}
	entry := coverage[0]
	if entry.Provider != "opensearch-vector" || entry.ReadinessStatus != "FAILED" || entry.ErrorClass != "INDEX_MISSING" {
		t.Fatalf("unexpected provider coverage entry: %+v", entry)
	}
	if entry.VectorLaneRequested || entry.VectorLaneStatus != "NOT_REQUESTED" || entry.VectorReturnedCount != 0 {
		t.Fatalf("unexpected vector lane linkage: %+v", entry)
	}
}

func TestVerifyProviderCoverageRejectsFailedRequestedProviderWithVectorBackend(t *testing.T) {
	path := writeProviderReadinessSummary(t, `{
  "phase": "preflight-provider-readiness",
  "provider_readiness": [
    {
      "provider": "pgvector",
      "requested": true,
      "configured": true,
      "available": false,
      "status": "FAILED",
      "error": "pgvector extension is unavailable"
    }
  ]
}`)
	cfg := config{includeVectorBackend: true, providerReadinessFile: path}
	_, err := verifyProviderCoverage(cfg, []sourceCoverageSummary{
		{SourceType: "VECTOR_ITEM", Requested: true, ReturnedCount: 1, Status: "RETURNED"},
	})
	if err == nil || !strings.Contains(err.Error(), "pgvector") {
		t.Fatalf("expected failed requested provider to reject vector backend, got %v", err)
	}
}

func TestVerifyProviderCoverageAcceptsReadyProviderWithVectorBackend(t *testing.T) {
	path := writeProviderReadinessSummary(t, `{
  "phase": "preflight-provider-readiness",
  "provider_readiness": [
    {
      "provider": "pgvector",
      "requested": true,
      "configured": true,
      "available": true,
      "status": "READY"
    }
  ]
}`)
	cfg := config{includeVectorBackend: true, providerReadinessFile: path}
	coverage, err := verifyProviderCoverage(cfg, []sourceCoverageSummary{
		{SourceType: "VECTOR_ITEM", Requested: true, ReturnedCount: 1, Status: "RETURNED"},
	})
	if err != nil {
		t.Fatalf("verifyProviderCoverage returned error: %v", err)
	}
	if len(coverage) != 1 || coverage[0].ReadinessStatus != "READY" || coverage[0].VectorLaneStatus != "RETURNED" {
		t.Fatalf("unexpected provider coverage: %+v", coverage)
	}
}

func TestVerifyProviderCoverageReportsMilvusReadiness(t *testing.T) {
	path := writeProviderReadinessSummary(t, `{
  "phase": "preflight-provider-readiness",
  "provider_readiness": [
    {
      "provider": "milvus",
      "requested": true,
      "configured": true,
      "available": false,
      "status": "FAILED",
      "error": "milvus collection nexusim_vector_items does not exist"
    }
  ]
}`)
	cfg := config{providerReadinessFile: path}
	coverage, err := verifyProviderCoverage(cfg, []sourceCoverageSummary{
		{SourceType: "VECTOR_ITEM", Requested: false, ReturnedCount: 0, Status: "NOT_REQUESTED"},
	})
	if err != nil {
		t.Fatalf("verifyProviderCoverage returned error: %v", err)
	}
	if len(coverage) != 1 {
		t.Fatalf("coverage len=%d want 1", len(coverage))
	}
	entry := coverage[0]
	if entry.Provider != "milvus" || entry.ReadinessStatus != "FAILED" || entry.ErrorClass != "INDEX_MISSING" {
		t.Fatalf("unexpected provider coverage entry: %+v", entry)
	}
}

func TestLoadProviderReadinessFileRejectsWrongPhase(t *testing.T) {
	path := writeProviderReadinessSummary(t, `{
  "phase": "verify",
  "provider_readiness": [
    {"provider": "pgvector", "requested": true, "configured": true, "available": true, "status": "READY"}
  ]
}`)
	if _, err := loadProviderReadinessFile(path); err == nil || !strings.Contains(err.Error(), "preflight-provider-readiness") {
		t.Fatalf("expected wrong phase to fail, got %v", err)
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

func writeProviderReadinessSummary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vector-embedding-producer-summary.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write provider readiness summary: %v", err)
	}
	return path
}
