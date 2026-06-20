package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
)

func TestPrepareKnowledgeSourceNormalizesAndHashesSourceRef(t *testing.T) {
	prepared, err := PrepareKnowledgeSource(types.CreateKnowledgeSourceCommand{
		AuthContext:     types.AuthContext{TenantID: "tenant-knowledge-test", ServiceName: "admin-service"},
		SourceType:      "manual_markdown",
		SourceRef:       " manual://project/private-source ",
		OwnerRef:        "group:docs",
		VisibilityScope: "tenant:tenant-knowledge-test",
		DataClass:       "business_internal",
		ContentHash:     "sha256:content",
		SourceVersion:   "",
		IdempotencyKey:  "source-idem",
	}, "ksrc_test", time.Unix(100, 0))
	if err != nil {
		t.Fatalf("prepare source: %v", err)
	}
	source := SourceFromPrepared(prepared)
	if source.SourceType != types.SourceTypeManualMarkdown || source.DataClass != types.DataClassBusinessInternal {
		t.Fatalf("unexpected normalized source: %+v", source)
	}
	if source.SourceVersion != types.DefaultSourceVersion {
		t.Fatalf("unexpected source version: %s", source.SourceVersion)
	}
	if source.SourceRefHash == "" || strings.Contains(source.SourceRefHash, "manual://") {
		t.Fatalf("source ref hash should be low-sensitive: %q", source.SourceRefHash)
	}
}

func TestPrepareIngestionJobRejectsOversizedPreview(t *testing.T) {
	_, err := PrepareIngestionJob(types.SubmitIngestionJobCommand{
		AuthContext:    types.AuthContext{TenantID: "tenant-knowledge-test", ServiceName: "admin-service"},
		SourceID:       "ksrc_test",
		SourceVersion:  "v1",
		JobType:        types.JobTypeIngest,
		RequestedBy:    "operator:test",
		IdempotencyKey: "job-idem",
		DocumentHash:   "sha256:doc",
		Chunks: []types.ChunkManifestItem{{
			ChunkHash:            "sha256:chunk",
			ChunkPreviewRedacted: strings.Repeat("x", types.MaxChunkPreviewBytes+1),
			VisibilityScope:      "tenant:tenant-knowledge-test",
			DataClass:            types.DataClassBusinessInternal,
			PolicyVersion:        types.DefaultPolicyVersion,
			ChunkVersion:         "v1",
		}},
	}, "kjob_test", "kdoc_test", []string{"kchk_test"}, time.Now())
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
