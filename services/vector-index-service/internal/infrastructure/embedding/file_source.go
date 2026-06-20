package embedding

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type FileTaskSource struct {
	path      string
	mu        sync.Mutex
	processed map[string]struct{}
}

type fileTask struct {
	TenantID           string `json:"tenant_id"`
	SourceService      string `json:"source_service"`
	CollectionType     string `json:"collection_type"`
	SourceRefHash      string `json:"source_ref_hash"`
	SourceID           string `json:"source_id"`
	SourceVersion      int64  `json:"source_version"`
	SourceHash         string `json:"source_hash"`
	ChunkHash          string `json:"chunk_hash"`
	InputText          string `json:"input_text"`
	InputHash          string `json:"input_hash"`
	InputSchemaVersion int    `json:"input_schema_version"`
	EmbeddingModelRef  string `json:"embedding_model_ref"`
	Dimension          int    `json:"dimension"`
	VisibilityScope    string `json:"visibility_scope"`
	VisibilityVersion  int64  `json:"visibility_version"`
	PolicyVersion      string `json:"policy_version"`
	DataClass          string `json:"data_class"`
	DeleteProofID      string `json:"delete_proof_id"`
	RetentionPolicyRef string `json:"retention_policy_ref"`
	IdempotencyKey     string `json:"idempotency_key"`
	CorrelationID      string `json:"correlation_id"`
	CausationID        string `json:"causation_id"`
	TraceID            string `json:"trace_id"`
	RequestID          string `json:"request_id"`
	TimeoutMS          int64  `json:"timeout_ms"`
}

func NewFileTaskSource(path string) (*FileTaskSource, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("vector embedding task file is required")
	}
	return &FileTaskSource{path: path, processed: map[string]struct{}{}}, nil
}

func (source *FileTaskSource) ClaimEmbeddingTasks(_ context.Context, limit int) ([]types.VectorEmbeddingTask, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	file, err := os.Open(source.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	tasks := make([]types.VectorEmbeddingTask, 0, limit)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var item fileTask
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, err
		}
		task := item.toTask()
		key := task.IdempotencyKey
		if key == "" {
			key = task.SourceRefHash + ":" + task.EmbeddingModelRef
		}
		if _, ok := source.processed[key]; ok {
			continue
		}
		tasks = append(tasks, task)
		if len(tasks) >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (source *FileTaskSource) CompleteEmbeddingTask(_ context.Context, task types.VectorEmbeddingTask) error {
	source.mu.Lock()
	defer source.mu.Unlock()
	key := strings.TrimSpace(task.IdempotencyKey)
	if key == "" {
		key = strings.TrimSpace(task.SourceRefHash) + ":" + strings.TrimSpace(task.EmbeddingModelRef)
	}
	source.processed[key] = struct{}{}
	return nil
}

func (item fileTask) toTask() types.VectorEmbeddingTask {
	timeout := time.Duration(item.TimeoutMS) * time.Millisecond
	if item.TimeoutMS <= 0 {
		timeout = 0
	}
	return types.VectorEmbeddingTask{
		AuthContext: types.AuthContext{
			TenantID:    types.TenantID(item.TenantID),
			ServiceName: types.AllowedCallerVectorIndex,
			InstanceRef: "embedding-worker",
			TraceID:     item.TraceID,
			RequestID:   item.RequestID,
		},
		SourceService:      item.SourceService,
		CollectionType:     item.CollectionType,
		SourceRefHash:      item.SourceRefHash,
		SourceID:           item.SourceID,
		SourceVersion:      item.SourceVersion,
		SourceHash:         item.SourceHash,
		ChunkHash:          item.ChunkHash,
		InputText:          item.InputText,
		InputHash:          item.InputHash,
		InputSchemaVersion: item.InputSchemaVersion,
		EmbeddingModelRef:  item.EmbeddingModelRef,
		Dimension:          item.Dimension,
		VisibilityScope:    item.VisibilityScope,
		VisibilityVersion:  item.VisibilityVersion,
		PolicyVersion:      item.PolicyVersion,
		DataClass:          item.DataClass,
		DeleteProofID:      item.DeleteProofID,
		RetentionPolicyRef: item.RetentionPolicyRef,
		IdempotencyKey:     item.IdempotencyKey,
		CorrelationID:      item.CorrelationID,
		CausationID:        item.CausationID,
		TraceID:            item.TraceID,
		Timeout:            timeout,
	}
}
