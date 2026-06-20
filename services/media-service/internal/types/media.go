package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	MediaKindImage = "IMAGE"
	MediaKindFile  = "FILE"
	MediaKindVoice = "VOICE"
	MediaKindVideo = "VIDEO"

	AssetStatusUploadPending = "UPLOAD_PENDING"
	AssetStatusUploaded      = "UPLOADED"
	AssetStatusProcessing    = "PROCESSING"
	AssetStatusReady         = "READY"
	AssetStatusQuarantined   = "QUARANTINED"
	AssetStatusFailed        = "FAILED"
	AssetStatusDeleted       = "DELETED"
	AssetStatusExpired       = "EXPIRED"

	ProcessingStatusPending = "PENDING"
	ProcessingStatusPassed  = "PASSED"
	ProcessingStatusSkipped = "SKIPPED"
	ProcessingStatusFailed  = "FAILED"

	VariantOriginal   = "ORIGINAL"
	VariantThumbnail  = "THUMBNAIL"
	VariantTranscoded = "TRANSCODED"

	DefaultUploadSessionTTL = 15 * time.Minute
	DefaultDownloadURLTTL   = 5 * time.Minute
	DefaultMaxSizeBytes     = 2 * 1024 * 1024 * 1024
)

var sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type MediaAsset struct {
	TenantID        TenantID
	AssetID         string
	OwnerUserID     UserID
	ConversationID  string
	MediaKind       string
	ContentType     string
	FileName        string
	SizeBytes       int64
	SHA256          string
	ObjectKey       string
	Status          string
	ScanStatus      string
	ThumbnailStatus string
	TranscodeStatus string
	CreatedAt       time.Time
	UploadedAt      time.Time
	ReadyAt         time.Time
	DeletedAt       time.Time
}

type UploadSession struct {
	TenantID        TenantID
	UploadSessionID string
	AssetID         string
	OwnerUserID     UserID
	IdempotencyKey  string
	CommandHash     string
	Status          string
	ExpiresAt       time.Time
	CompletedAt     time.Time
	CreatedAt       time.Time
}

type CreateUploadSessionResult struct {
	Asset           MediaAsset
	Session         UploadSession
	UploadURL       string
	RequiredHeaders map[string]string
	MaxSizeBytes    int64
	AcceptedTypes   []string
}

type MessageAttachmentRef struct {
	AssetID     string
	MediaKind   string
	ContentType string
	FileName    string
	SizeBytes   int64
	SHA256      string
}

type ObjectMetadata struct {
	SizeBytes int64
	SHA256    string
}

type PresignedURL struct {
	URL             string
	RequiredHeaders map[string]string
	ExpiresAt       time.Time
}

type AccessAudit struct {
	TenantID       TenantID
	AuditID        string
	AssetID        string
	UserID         UserID
	ConversationID string
	MessageID      string
	Variant        string
	Decision       string
	DecisionSource string
	RequestID      string
}

type CreateUploadSessionCommand struct {
	AuthContext    AuthContext
	ConversationID string
	MediaKind      string
	FileName       string
	ContentType    string
	SizeBytes      int64
	SHA256         string
	IdempotencyKey string
}

func (command CreateUploadSessionCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.ConversationID) == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if !IsValidMediaKind(command.MediaKind) {
		return NewInvalidArgument("invalid media_kind")
	}
	if strings.TrimSpace(command.ContentType) == "" {
		return NewInvalidArgument("content_type is required")
	}
	if command.SizeBytes <= 0 || command.SizeBytes > DefaultMaxSizeBytes {
		return NewInvalidArgument("size_bytes is out of range")
	}
	if !sha256Pattern.MatchString(strings.TrimSpace(command.SHA256)) {
		return NewInvalidArgument("sha256 must be 64 hex characters")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	return nil
}

func (command CreateUploadSessionCommand) Normalized() CreateUploadSessionCommand {
	command.ConversationID = strings.TrimSpace(command.ConversationID)
	command.MediaKind = strings.TrimSpace(command.MediaKind)
	command.FileName = strings.TrimSpace(command.FileName)
	command.ContentType = strings.TrimSpace(command.ContentType)
	command.SHA256 = strings.ToLower(strings.TrimSpace(command.SHA256))
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	return command
}

func (command CreateUploadSessionCommand) CommandHash() string {
	command = command.Normalized()
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s",
		command.ConversationID,
		command.MediaKind,
		command.FileName,
		command.ContentType,
		command.SizeBytes,
		command.SHA256,
		command.IdempotencyKey,
	)))
	return hex.EncodeToString(sum[:])
}

type CompleteUploadCommand struct {
	AuthContext     AuthContext
	AssetID         string
	UploadSessionID string
	SHA256          string
	SizeBytes       int64
}

func (command CompleteUploadCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.AssetID) == "" {
		return NewInvalidArgument("asset_id is required")
	}
	if strings.TrimSpace(command.UploadSessionID) == "" {
		return NewInvalidArgument("upload_session_id is required")
	}
	if !sha256Pattern.MatchString(strings.TrimSpace(command.SHA256)) {
		return NewInvalidArgument("sha256 must be 64 hex characters")
	}
	if command.SizeBytes <= 0 {
		return NewInvalidArgument("size_bytes is required")
	}
	return nil
}

func (command CompleteUploadCommand) Normalized() CompleteUploadCommand {
	command.AssetID = strings.TrimSpace(command.AssetID)
	command.UploadSessionID = strings.TrimSpace(command.UploadSessionID)
	command.SHA256 = strings.ToLower(strings.TrimSpace(command.SHA256))
	return command
}

type GetMediaAssetCommand struct {
	AuthContext AuthContext
	AssetID     string
}

func (command GetMediaAssetCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.AssetID) == "" {
		return NewInvalidArgument("asset_id is required")
	}
	return nil
}

type GetMediaDownloadURLCommand struct {
	AuthContext    AuthContext
	AssetID        string
	ConversationID string
	MessageID      string
	Variant        string
}

func (command GetMediaDownloadURLCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.AssetID) == "" {
		return NewInvalidArgument("asset_id is required")
	}
	if strings.TrimSpace(command.ConversationID) == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	if !IsValidVariant(command.EffectiveVariant()) {
		return NewInvalidArgument("invalid requested_variant")
	}
	return nil
}

func (command GetMediaDownloadURLCommand) EffectiveVariant() string {
	if strings.TrimSpace(command.Variant) == "" {
		return VariantOriginal
	}
	return strings.TrimSpace(command.Variant)
}

type GetMediaDownloadURLResult struct {
	AssetID         string
	Variant         string
	DownloadURL     string
	RequiredHeaders map[string]string
	ExpiresAt       time.Time
}

type DeleteMediaAssetCommand struct {
	AuthContext     AuthContext
	AssetID         string
	DeleteRequestID string
	Reason          string
}

func (command DeleteMediaAssetCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.AssetID) == "" {
		return NewInvalidArgument("asset_id is required")
	}
	if strings.TrimSpace(command.DeleteRequestID) == "" {
		return NewInvalidArgument("delete_request_id is required")
	}
	return nil
}

func ToAttachmentRef(asset MediaAsset) MessageAttachmentRef {
	return MessageAttachmentRef{
		AssetID:     asset.AssetID,
		MediaKind:   asset.MediaKind,
		ContentType: asset.ContentType,
		FileName:    asset.FileName,
		SizeBytes:   asset.SizeBytes,
		SHA256:      asset.SHA256,
	}
}

func IsValidMediaKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case MediaKindImage, MediaKindFile, MediaKindVoice, MediaKindVideo:
		return true
	default:
		return false
	}
}

func IsValidVariant(variant string) bool {
	switch strings.TrimSpace(variant) {
	case VariantOriginal, VariantThumbnail, VariantTranscoded:
		return true
	default:
		return false
	}
}

func AcceptedContentTypes() []string {
	return []string{
		"image/jpeg",
		"image/png",
		"image/webp",
		"audio/mpeg",
		"audio/ogg",
		"video/mp4",
		"application/octet-stream",
	}
}
