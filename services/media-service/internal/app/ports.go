package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

type Repository interface {
	CreateUploadSession(ctx context.Context, command types.CreateUploadSessionCommand, allocation UploadAllocation) (types.MediaAsset, types.UploadSession, error)
	GetAsset(ctx context.Context, tenantID types.TenantID, assetID string) (types.MediaAsset, error)
	CompleteUpload(ctx context.Context, command types.CompleteUploadCommand, metadata types.ObjectMetadata) (types.MediaAsset, error)
	RecordAccessAudit(ctx context.Context, audit types.AccessAudit) error
	DeleteMediaAsset(ctx context.Context, command types.DeleteMediaAssetCommand) (types.MediaAsset, error)
}

type ObjectStore interface {
	PresignPut(ctx context.Context, objectKey string, metadata types.ObjectMetadata, expiresAt time.Time) (types.PresignedURL, error)
	VerifyUploadedObject(ctx context.Context, objectKey string, expected types.ObjectMetadata) (types.ObjectMetadata, error)
	PresignGet(ctx context.Context, objectKey string, variant string, expiresAt time.Time) (types.PresignedURL, error)
}

type VisibilityChecker interface {
	CanDownload(ctx context.Context, auth types.AuthContext, asset types.MediaAsset, messageID string) (VisibilityDecision, error)
}

type UploadAllocation struct {
	AssetID         string
	UploadSessionID string
	ObjectKey       string
	ExpiresAt       time.Time
}

type VisibilityDecision struct {
	Allowed bool
	Source  string
}
