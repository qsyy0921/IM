package embedding

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestFileTaskSourceClaimsAndCompletesTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "embedding-tasks.jsonl")
	content := `{"tenant_id":"tenant-vector","source_service":"knowledge-ingestion-service","collection_type":"KNOWLEDGE_CHUNK","source_ref_hash":"sha256:sourceref","source_id":"ksrc_1:kchunk_1","source_version":1,"source_hash":"sha256:sourcehash","chunk_hash":"sha256:chunkhash","input_text":"redacted local input","input_hash":"sha256:inputhash","input_schema_version":1,"embedding_model_ref":"deterministic-embedding-v1","dimension":8,"visibility_scope":"tenant:tenant-vector","visibility_version":1,"policy_version":"policy-v1","data_class":"BUSINESS_INTERNAL","idempotency_key":"idem-embed","trace_id":"trace-embed"}`
	if err := os.WriteFile(path, []byte(content+"\n"), 0o600); err != nil {
		t.Fatalf("write task file: %v", err)
	}
	source, err := NewFileTaskSource(path)
	if err != nil {
		t.Fatalf("new source: %v", err)
	}

	tasks, err := source.ClaimEmbeddingTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("claim tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.AuthContext.ServiceName != types.AllowedCallerVectorIndex || task.SourceService != types.AllowedCallerKnowledgeIngestion {
		t.Fatalf("unexpected service refs: %+v", task)
	}
	if task.InputText != "redacted local input" {
		t.Fatalf("expected input text to be available only to worker source: %+v", task)
	}
	if err := source.CompleteEmbeddingTask(context.Background(), task); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	tasks, err = source.ClaimEmbeddingTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("claim after complete: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("completed task should not be claimed again: %+v", tasks)
	}
}
