package types

import (
	"strconv"
	"strings"
	"time"
)

const (
	SourceTypeMediaObject     = "MEDIA_OBJECT"
	SourceTypeWebPage         = "WEB_PAGE"
	SourceTypeAdminUpload     = "ADMIN_UPLOAD"
	SourceTypeConnectorRecord = "CONNECTOR_RECORD"
	SourceTypeManualMarkdown  = "MANUAL_MARKDOWN"

	DataClassLowSensitive      = "LOW_SENSITIVE"
	DataClassBusinessInternal  = "BUSINESS_INTERNAL"
	DataClassUserContent       = "USER_CONTENT"
	DataClassSecuritySensitive = "SECURITY_SENSITIVE"

	SourceStatusActive     = "ACTIVE"
	SourceStatusTombstoned = "TOMBSTONED"

	JobTypeIngest            = "INGEST"
	JobTypeReingest          = "REINGEST"
	JobTypeRebuildChunks     = "REBUILD_CHUNKS"
	JobTypeRefreshMetadata   = "REFRESH_METADATA"
	JobTypeTombstone         = "TOMBSTONE"
	JobTypeDeleteProofRepair = "DELETE_PROOF_REPAIR"

	JobStatusPending = "PENDING"
	JobStatusDone    = "DONE"

	ChunkStatusReady       = "READY"
	TombstoneStatusActive  = "ACTIVE"
	EmbeddingStatusPending = "PENDING"
	VectorStatusPending    = "PENDING"

	DefaultSourceVersion = "v1"
	DefaultParserProfile = "local-manifest-v1"
	DefaultChunkProfile  = "fixed-manifest-v1"
	DefaultPolicyVersion = "policy-local-v1"
	MaxChunkPreviewBytes = 256
	MaxChunkManifestSize = 1000
	MaxListPageSize      = 200
)

type KnowledgeSource struct {
	TenantID           TenantID
	SourceID           string
	IdempotencyKey     string
	CommandHash        string
	SourceType         string
	SourceRef          string
	SourceRefHash      string
	MediaObjectRef     string
	OwnerRef           string
	VisibilityScope    string
	DataClass          string
	ContentHash        string
	MimeType           string
	SizeBytes          int64
	SourceVersion      string
	RetentionPolicyRef string
	Status             string
	CorrelationID      string
	CausationID        string
	TraceID            string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type KnowledgeDocument struct {
	TenantID           TenantID
	DocumentID         string
	SourceID           string
	SourceVersion      string
	ParserProfile      string
	MimeType           string
	SizeBytes          int64
	PageCount          int
	Language           string
	DocumentHash       string
	ParseStatus        string
	ParserFailureClass string
	CreatedAt          time.Time
}

type KnowledgeChunk struct {
	TenantID             TenantID
	ChunkID              string
	DocumentID           string
	SourceID             string
	SourceVersion        string
	ChunkIndex           int
	ChunkHash            string
	ChunkPreviewRedacted string
	VisibilityScope      string
	DataClass            string
	PolicyVersion        string
	ChunkVersion         string
	Status               string
	TombstoneStatus      string
	DeleteProofID        string
	EmbeddingStatus      string
	VectorStatus         string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type KnowledgeIngestionJob struct {
	TenantID           TenantID
	JobID              string
	IdempotencyKey     string
	CommandHash        string
	SourceID           string
	SourceVersion      string
	JobType            string
	ParserProfile      string
	ChunkProfile       string
	EmbeddingPolicyRef string
	VectorPolicyRef    string
	RequestedBy        string
	Status             string
	RetryCount         int
	FailureClass       string
	PublicError        string
	DocumentID         string
	ChunkCount         int
	CorrelationID      string
	CausationID        string
	TraceID            string
	CreatedAt          time.Time
	CompletedAt        time.Time
}

type ChunkManifestItem struct {
	ChunkHash            string
	ChunkPreviewRedacted string
	VisibilityScope      string
	DataClass            string
	PolicyVersion        string
	ChunkVersion         string
}

type CreateKnowledgeSourceCommand struct {
	AuthContext        AuthContext
	SourceType         string
	SourceRef          string
	SourceURIHash      string
	MediaObjectRef     string
	OwnerRef           string
	VisibilityScope    string
	DataClass          string
	ContentHash        string
	MimeType           string
	SizeBytes          int64
	SourceVersion      string
	RetentionPolicyRef string
	IdempotencyKey     string
	CorrelationID      string
	CausationID        string
	TraceID            string
}

func (command CreateKnowledgeSourceCommand) Validate() error {
	if err := command.AuthContext.ValidateService(); err != nil {
		return err
	}
	if !IsValidSourceType(command.SourceType) {
		return NewInvalidArgument("source_type is invalid")
	}
	if strings.TrimSpace(command.SourceRef) == "" {
		return NewInvalidArgument("source_ref is required")
	}
	if strings.TrimSpace(command.OwnerRef) == "" {
		return NewInvalidArgument("owner_ref is required")
	}
	if strings.TrimSpace(command.VisibilityScope) == "" {
		return NewInvalidArgument("visibility_scope is required")
	}
	if !IsValidDataClass(command.DataClass) {
		return NewInvalidArgument("data_class is invalid")
	}
	if strings.TrimSpace(command.ContentHash) == "" {
		return NewInvalidArgument("content_hash is required")
	}
	if command.SizeBytes < 0 {
		return NewInvalidArgument("size_bytes is invalid")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

func (command CreateKnowledgeSourceCommand) Normalized() CreateKnowledgeSourceCommand {
	command.SourceType = strings.ToUpper(strings.TrimSpace(command.SourceType))
	command.SourceRef = strings.TrimSpace(command.SourceRef)
	command.SourceURIHash = strings.TrimSpace(command.SourceURIHash)
	command.MediaObjectRef = strings.TrimSpace(command.MediaObjectRef)
	command.OwnerRef = strings.TrimSpace(command.OwnerRef)
	command.VisibilityScope = strings.TrimSpace(command.VisibilityScope)
	command.DataClass = strings.ToUpper(strings.TrimSpace(command.DataClass))
	command.ContentHash = strings.TrimSpace(command.ContentHash)
	command.MimeType = strings.ToLower(strings.TrimSpace(command.MimeType))
	command.SourceVersion = strings.TrimSpace(command.SourceVersion)
	if command.SourceVersion == "" {
		command.SourceVersion = DefaultSourceVersion
	}
	command.RetentionPolicyRef = strings.TrimSpace(command.RetentionPolicyRef)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	return command
}

type SubmitIngestionJobCommand struct {
	AuthContext        AuthContext
	SourceID           string
	SourceVersion      string
	JobType            string
	ParserProfile      string
	ChunkProfile       string
	EmbeddingPolicyRef string
	VectorPolicyRef    string
	RequestedBy        string
	IdempotencyKey     string
	DocumentHash       string
	MimeType           string
	SizeBytes          int64
	PageCount          int
	Language           string
	Chunks             []ChunkManifestItem
	CorrelationID      string
	CausationID        string
	TraceID            string
}

func (command SubmitIngestionJobCommand) Validate() error {
	if err := command.AuthContext.ValidateService(); err != nil {
		return err
	}
	if strings.TrimSpace(command.SourceID) == "" {
		return NewInvalidArgument("source_id is required")
	}
	if strings.TrimSpace(command.SourceVersion) == "" {
		return NewInvalidArgument("source_version is required")
	}
	if !IsValidJobType(command.JobType) {
		return NewInvalidArgument("job_type is invalid")
	}
	if strings.TrimSpace(command.RequestedBy) == "" {
		return NewInvalidArgument("requested_by is required")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if command.SizeBytes < 0 || command.PageCount < 0 {
		return NewInvalidArgument("document size/page count is invalid")
	}
	if len(command.Chunks) > MaxChunkManifestSize {
		return NewInvalidArgument("chunk manifest is too large")
	}
	for index, chunk := range command.Chunks {
		if strings.TrimSpace(chunk.ChunkHash) == "" {
			return NewInvalidArgument("chunk_hash is required")
		}
		if len(chunk.ChunkPreviewRedacted) > MaxChunkPreviewBytes {
			return NewInvalidArgument("chunk_preview_redacted is too large")
		}
		if !IsValidDataClass(chunk.DataClass) {
			return NewInvalidArgument("chunk data_class is invalid")
		}
		if strings.TrimSpace(chunk.VisibilityScope) == "" {
			return NewInvalidArgument("chunk visibility_scope is required")
		}
		if strings.TrimSpace(chunk.PolicyVersion) == "" {
			return NewInvalidArgument("chunk policy_version is required")
		}
		if strings.TrimSpace(chunk.ChunkVersion) == "" {
			return NewInvalidArgument("chunk_version is required at index " + strconv.Itoa(index))
		}
	}
	return nil
}

func (command SubmitIngestionJobCommand) Normalized() SubmitIngestionJobCommand {
	command.SourceID = strings.TrimSpace(command.SourceID)
	command.SourceVersion = strings.TrimSpace(command.SourceVersion)
	if command.SourceVersion == "" {
		command.SourceVersion = DefaultSourceVersion
	}
	command.JobType = strings.ToUpper(strings.TrimSpace(command.JobType))
	command.ParserProfile = strings.TrimSpace(command.ParserProfile)
	if command.ParserProfile == "" {
		command.ParserProfile = DefaultParserProfile
	}
	command.ChunkProfile = strings.TrimSpace(command.ChunkProfile)
	if command.ChunkProfile == "" {
		command.ChunkProfile = DefaultChunkProfile
	}
	command.EmbeddingPolicyRef = strings.TrimSpace(command.EmbeddingPolicyRef)
	command.VectorPolicyRef = strings.TrimSpace(command.VectorPolicyRef)
	command.RequestedBy = strings.TrimSpace(command.RequestedBy)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.DocumentHash = strings.TrimSpace(command.DocumentHash)
	command.MimeType = strings.ToLower(strings.TrimSpace(command.MimeType))
	command.Language = strings.ToLower(strings.TrimSpace(command.Language))
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	for index := range command.Chunks {
		command.Chunks[index].ChunkHash = strings.TrimSpace(command.Chunks[index].ChunkHash)
		command.Chunks[index].ChunkPreviewRedacted = strings.TrimSpace(command.Chunks[index].ChunkPreviewRedacted)
		command.Chunks[index].VisibilityScope = strings.TrimSpace(command.Chunks[index].VisibilityScope)
		command.Chunks[index].DataClass = strings.ToUpper(strings.TrimSpace(command.Chunks[index].DataClass))
		command.Chunks[index].PolicyVersion = strings.TrimSpace(command.Chunks[index].PolicyVersion)
		if command.Chunks[index].PolicyVersion == "" {
			command.Chunks[index].PolicyVersion = DefaultPolicyVersion
		}
		command.Chunks[index].ChunkVersion = strings.TrimSpace(command.Chunks[index].ChunkVersion)
		if command.Chunks[index].ChunkVersion == "" {
			command.Chunks[index].ChunkVersion = command.SourceVersion
		}
	}
	return command
}

type GetIngestionJobCommand struct {
	AuthContext AuthContext
	JobID       string
}

func (command GetIngestionJobCommand) Validate() error {
	if err := command.AuthContext.ValidateService(); err != nil {
		return err
	}
	if strings.TrimSpace(command.JobID) == "" {
		return NewInvalidArgument("job_id is required")
	}
	return nil
}

type ListKnowledgeChunksCommand struct {
	AuthContext AuthContext
	SourceID    string
	DocumentID  string
	PageSize    int
	PageToken   string
}

func (command ListKnowledgeChunksCommand) Validate() error {
	if err := command.AuthContext.ValidateService(); err != nil {
		return err
	}
	if strings.TrimSpace(command.SourceID) == "" && strings.TrimSpace(command.DocumentID) == "" {
		return NewInvalidArgument("source_id or document_id is required")
	}
	if command.PageSize < 0 || command.PageSize > MaxListPageSize {
		return NewInvalidArgument("page_size is invalid")
	}
	return nil
}

func IsValidSourceType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case SourceTypeMediaObject, SourceTypeWebPage, SourceTypeAdminUpload, SourceTypeConnectorRecord, SourceTypeManualMarkdown:
		return true
	default:
		return false
	}
}

func IsValidDataClass(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case DataClassLowSensitive, DataClassBusinessInternal, DataClassUserContent, DataClassSecuritySensitive:
		return true
	default:
		return false
	}
}

func IsValidJobType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case JobTypeIngest, JobTypeReingest, JobTypeRebuildChunks, JobTypeRefreshMetadata, JobTypeTombstone, JobTypeDeleteProofRepair:
		return true
	default:
		return false
	}
}
