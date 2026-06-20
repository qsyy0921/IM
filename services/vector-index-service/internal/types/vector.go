package types

import (
	"strings"
	"time"
)

const (
	CollectionTypeKnowledgeChunk    = "KNOWLEDGE_CHUNK"
	CollectionTypeMemoryEvent       = "MEMORY_EVENT"
	CollectionTypeSearchDocument    = "SEARCH_DOCUMENT"
	CollectionTypeProfileAggregate  = "PROFILE_AGGREGATE"
	CollectionTypeEvalFixture       = "EVAL_FIXTURE"
	CollectionStatusActive          = "ACTIVE"
	BackendTypePostgresTest         = "POSTGRES_TEST"
	VectorItemStatusIndexed         = "INDEXED"
	VectorItemStatusTombstoned      = "TOMBSTONED"
	TombstoneStatusNone             = "NONE"
	TombstoneStatusTombstoned       = "TOMBSTONED"
	JobTypeUpsert                   = "UPSERT"
	JobTypeTombstone                = "TOMBSTONE"
	JobTypeRebuild                  = "REBUILD"
	JobStatusIndexed                = "INDEXED"
	JobStatusTombstoned             = "TOMBSTONED"
	AllowedCallerKnowledgeIngestion = "knowledge-ingestion-service"
	AllowedCallerMemory             = "memory-service"
	AllowedCallerSearch             = "search-service"
	AllowedCallerVectorIndex        = "vector-index-service"
	AllowedCallerRetrieval          = "retrieval-gateway"
)

type UpsertVectorItemCommand struct {
	AuthContext         AuthContext
	SourceService       string
	CollectionType      string
	SourceRefHash       string
	SourceID            string
	SourceVersion       int64
	SourceHash          string
	ChunkHash           string
	EmbeddingModelRef   string
	EmbeddingVectorHash string
	Dimension           int
	VisibilityScope     string
	VisibilityVersion   int64
	PolicyVersion       string
	DataClass           string
	DeleteProofID       string
	RetentionPolicyRef  string
	IdempotencyKey      string
	CorrelationID       string
	CausationID         string
	TraceID             string
}

type TombstoneVectorItemCommand struct {
	AuthContext    AuthContext
	VectorItemID   string
	DeleteProofID  string
	ReasonClass    string
	IdempotencyKey string
	CorrelationID  string
	CausationID    string
	TraceID        string
}

type SearchVectorsCommand struct {
	AuthContext        AuthContext
	RequesterRef       string
	RetrievalRequestID string
	CollectionTypes    []string
	QueryEmbeddingRef  string
	TopK               int
	MinScore           float64
	VisibilityScope    string
	PolicyVersion      string
	At                 time.Time
}

type GetVectorIndexJobCommand struct {
	AuthContext AuthContext
	JobID       string
}

type VectorCollection struct {
	TenantID              TenantID
	CollectionID          string
	CollectionType        string
	BackendType           string
	Dimension             int
	EmbeddingModelRef     string
	RoutePolicyRef        string
	Status                string
	MetadataSchemaVersion int
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type VectorItem struct {
	TenantID            TenantID
	VectorItemID        string
	CollectionID        string
	CollectionType      string
	SourceService       string
	SourceRefHash       string
	SourceID            string
	SourceVersion       int64
	SourceHash          string
	ChunkHash           string
	EmbeddingModelRef   string
	EmbeddingVectorHash string
	Dimension           int
	VisibilityScope     string
	VisibilityVersion   int64
	PolicyVersion       string
	DataClass           string
	TombstoneStatus     string
	DeleteProofID       string
	RetentionPolicyRef  string
	Status              string
	IdempotencyKey      string
	CommandHash         string
	CorrelationID       string
	CausationID         string
	TraceID             string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type VectorIndexJob struct {
	TenantID       TenantID
	JobID          string
	CollectionID   string
	VectorItemID   string
	JobType        string
	Status         string
	RetryCount     int
	FailureClass   string
	PublicError    string
	IdempotencyKey string
	CommandHash    string
	CorrelationID  string
	CausationID    string
	TraceID        string
	CreatedAt      time.Time
	CompletedAt    time.Time
}

type VectorTombstone struct {
	TenantID            TenantID
	TombstoneID         string
	VectorItemID        string
	SourceRefHash       string
	DeleteProofID       string
	ReasonClass         string
	BackendDeleteStatus string
	IdempotencyKey      string
	CommandHash         string
	CreatedAt           time.Time
}

type VectorSearchResult struct {
	VectorItemRef     string
	SourceRefHash     string
	SourceService     string
	CollectionType    string
	Score             float64
	VisibilityVersion int64
	TombstoneStatus   string
}

func (command UpsertVectorItemCommand) Normalized() UpsertVectorItemCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.SourceService = strings.TrimSpace(command.SourceService)
	command.CollectionType = strings.ToUpper(strings.TrimSpace(command.CollectionType))
	command.SourceRefHash = strings.TrimSpace(command.SourceRefHash)
	command.SourceID = strings.TrimSpace(command.SourceID)
	command.SourceHash = strings.TrimSpace(command.SourceHash)
	command.ChunkHash = strings.TrimSpace(command.ChunkHash)
	command.EmbeddingModelRef = strings.TrimSpace(command.EmbeddingModelRef)
	command.EmbeddingVectorHash = strings.TrimSpace(command.EmbeddingVectorHash)
	command.VisibilityScope = strings.TrimSpace(command.VisibilityScope)
	command.PolicyVersion = strings.TrimSpace(command.PolicyVersion)
	command.DataClass = strings.ToUpper(strings.TrimSpace(command.DataClass))
	command.DeleteProofID = strings.TrimSpace(command.DeleteProofID)
	command.RetentionPolicyRef = strings.TrimSpace(command.RetentionPolicyRef)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	return command
}

func (command UpsertVectorItemCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if !isAllowedIndexingCaller(command.AuthContext.ServiceName) {
		return NewPermissionDenied("caller is not allowed to mutate vector index")
	}
	if !isAllowedCollectionType(command.CollectionType) {
		return NewInvalidArgument("collection_type is unsupported")
	}
	if command.SourceService == "" || command.SourceRefHash == "" || command.SourceID == "" {
		return NewInvalidArgument("source refs are required")
	}
	if command.SourceVersion <= 0 {
		return NewInvalidArgument("source_version must be positive")
	}
	if !looksHash(command.SourceRefHash) || !looksHash(command.SourceHash) || !looksHash(command.ChunkHash) || !looksHash(command.EmbeddingVectorHash) {
		return NewInvalidArgument("source and embedding refs must be hashes")
	}
	if command.EmbeddingModelRef == "" || command.Dimension <= 0 {
		return NewInvalidArgument("embedding model and dimension are required")
	}
	if command.VisibilityScope == "" || command.VisibilityVersion <= 0 || command.PolicyVersion == "" {
		return NewInvalidArgument("visibility and policy metadata are required")
	}
	if command.IdempotencyKey == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if containsSensitiveValue(command.SourceService, command.SourceID, command.EmbeddingModelRef, command.VisibilityScope, command.PolicyVersion, command.DataClass, command.DeleteProofID, command.RetentionPolicyRef) {
		return NewInvalidArgument("vector metadata must use low-sensitive refs")
	}
	return nil
}

func (command TombstoneVectorItemCommand) Normalized() TombstoneVectorItemCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.VectorItemID = strings.TrimSpace(command.VectorItemID)
	command.DeleteProofID = strings.TrimSpace(command.DeleteProofID)
	command.ReasonClass = strings.ToUpper(strings.TrimSpace(command.ReasonClass))
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	return command
}

func (command TombstoneVectorItemCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if !isAllowedIndexingCaller(command.AuthContext.ServiceName) {
		return NewPermissionDenied("caller is not allowed to mutate vector index")
	}
	if command.VectorItemID == "" || command.DeleteProofID == "" || command.ReasonClass == "" {
		return NewInvalidArgument("vector_item_id, delete_proof_id and reason_class are required")
	}
	if command.IdempotencyKey == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if containsSensitiveValue(command.VectorItemID, command.DeleteProofID, command.ReasonClass) {
		return NewInvalidArgument("tombstone refs must be low-sensitive")
	}
	return nil
}

func (command SearchVectorsCommand) Normalized() SearchVectorsCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.RequesterRef = strings.TrimSpace(command.RequesterRef)
	command.RetrievalRequestID = strings.TrimSpace(command.RetrievalRequestID)
	command.CollectionTypes = normalizeCollectionTypes(command.CollectionTypes)
	command.QueryEmbeddingRef = strings.TrimSpace(command.QueryEmbeddingRef)
	command.VisibilityScope = strings.TrimSpace(command.VisibilityScope)
	command.PolicyVersion = strings.TrimSpace(command.PolicyVersion)
	return command
}

func (command SearchVectorsCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.AuthContext.ServiceName != AllowedCallerRetrieval && command.AuthContext.ServiceName != AllowedCallerVectorIndex {
		return NewPermissionDenied("caller is not allowed to search vectors")
	}
	if command.RequesterRef == "" || command.RetrievalRequestID == "" {
		return NewInvalidArgument("requester_ref and retrieval_request_id are required")
	}
	if command.QueryEmbeddingRef == "" || containsSensitiveValue(command.QueryEmbeddingRef) {
		return NewInvalidArgument("query_embedding_ref is required and must be low-sensitive")
	}
	if command.TopK <= 0 || command.TopK > 100 {
		return NewInvalidArgument("top_k must be between 1 and 100")
	}
	if command.MinScore < 0 || command.MinScore > 1 {
		return NewInvalidArgument("min_score must be between 0 and 1")
	}
	if command.VisibilityScope == "" || command.PolicyVersion == "" {
		return NewInvalidArgument("visibility_scope and policy_version are required")
	}
	for _, collectionType := range command.CollectionTypes {
		if !isAllowedCollectionType(collectionType) {
			return NewInvalidArgument("collection_type is unsupported")
		}
	}
	return nil
}

func (command GetVectorIndexJobCommand) Normalized() GetVectorIndexJobCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.JobID = strings.TrimSpace(command.JobID)
	return command
}

func (command GetVectorIndexJobCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.JobID == "" {
		return NewInvalidArgument("job_id is required")
	}
	return nil
}

func isAllowedIndexingCaller(service string) bool {
	switch strings.TrimSpace(service) {
	case AllowedCallerKnowledgeIngestion, AllowedCallerMemory, AllowedCallerSearch, AllowedCallerVectorIndex:
		return true
	default:
		return false
	}
}

func isAllowedCollectionType(value string) bool {
	switch value {
	case CollectionTypeKnowledgeChunk, CollectionTypeMemoryEvent, CollectionTypeSearchDocument, CollectionTypeProfileAggregate, CollectionTypeEvalFixture:
		return true
	default:
		return false
	}
}

func normalizeCollectionTypes(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToUpper(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func looksHash(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "sha256:")
}

func containsSensitiveValue(values ...string) bool {
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		for _, marker := range []string{"secret", "token", "api_key", "apikey", "password", "private://", "raw:", "dsn=", "postgres://", "http://", "https://", "s3://"} {
			if strings.Contains(normalized, marker) {
				return true
			}
		}
	}
	return false
}
