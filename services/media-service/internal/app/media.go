package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

type CreateUploadSessionUseCase struct {
	repository  Repository
	objectStore ObjectStore
	now         func() time.Time
}

func NewCreateUploadSessionUseCase(repository Repository, objectStore ObjectStore) *CreateUploadSessionUseCase {
	return &CreateUploadSessionUseCase{repository: repository, objectStore: objectStore, now: time.Now}
}

func (useCase *CreateUploadSessionUseCase) Execute(
	ctx context.Context,
	command types.CreateUploadSessionCommand,
) (types.CreateUploadSessionResult, error) {
	if err := command.Validate(); err != nil {
		return types.CreateUploadSessionResult{}, err
	}
	command = command.Normalized()
	expiresAt := useCase.now().Add(types.DefaultUploadSessionTTL)
	allocation := UploadAllocation{
		AssetID:         randomID("asset"),
		UploadSessionID: randomID("upload"),
		ObjectKey:       objectKey(command),
		ExpiresAt:       expiresAt,
	}
	asset, session, err := useCase.repository.CreateUploadSession(ctx, command, allocation)
	if err != nil {
		return types.CreateUploadSessionResult{}, err
	}
	presign, err := useCase.objectStore.PresignPut(ctx, asset.ObjectKey, types.ObjectMetadata{
		SizeBytes: asset.SizeBytes,
		SHA256:    asset.SHA256,
	}, session.ExpiresAt)
	if err != nil {
		return types.CreateUploadSessionResult{}, err
	}
	return types.CreateUploadSessionResult{
		Asset:           asset,
		Session:         session,
		UploadURL:       presign.URL,
		RequiredHeaders: presign.RequiredHeaders,
		MaxSizeBytes:    types.DefaultMaxSizeBytes,
		AcceptedTypes:   types.AcceptedContentTypes(),
	}, nil
}

type CompleteUploadUseCase struct {
	repository  Repository
	objectStore ObjectStore
}

func NewCompleteUploadUseCase(repository Repository, objectStore ObjectStore) *CompleteUploadUseCase {
	return &CompleteUploadUseCase{repository: repository, objectStore: objectStore}
}

func (useCase *CompleteUploadUseCase) Execute(
	ctx context.Context,
	command types.CompleteUploadCommand,
) (types.MediaAsset, error) {
	if err := command.Validate(); err != nil {
		return types.MediaAsset{}, err
	}
	command = command.Normalized()
	asset, err := useCase.repository.GetAsset(ctx, command.AuthContext.TenantID, command.AssetID)
	if err != nil {
		return types.MediaAsset{}, err
	}
	metadata, err := useCase.objectStore.VerifyUploadedObject(ctx, asset.ObjectKey, types.ObjectMetadata{
		SizeBytes: command.SizeBytes,
		SHA256:    command.SHA256,
	})
	if err != nil {
		return types.MediaAsset{}, err
	}
	return useCase.repository.CompleteUpload(ctx, command, metadata)
}

type GetMediaAssetUseCase struct {
	repository Repository
}

func NewGetMediaAssetUseCase(repository Repository) *GetMediaAssetUseCase {
	return &GetMediaAssetUseCase{repository: repository}
}

func (useCase *GetMediaAssetUseCase) Execute(
	ctx context.Context,
	command types.GetMediaAssetCommand,
) (types.MediaAsset, error) {
	if err := command.Validate(); err != nil {
		return types.MediaAsset{}, err
	}
	return useCase.repository.GetAsset(ctx, command.AuthContext.TenantID, strings.TrimSpace(command.AssetID))
}

type GetMediaDownloadURLUseCase struct {
	repository        Repository
	objectStore       ObjectStore
	visibilityChecker VisibilityChecker
	now               func() time.Time
}

func NewGetMediaDownloadURLUseCase(repository Repository, objectStore ObjectStore, visibilityChecker VisibilityChecker) *GetMediaDownloadURLUseCase {
	return &GetMediaDownloadURLUseCase{
		repository:        repository,
		objectStore:       objectStore,
		visibilityChecker: visibilityChecker,
		now:               time.Now,
	}
}

func (useCase *GetMediaDownloadURLUseCase) Execute(
	ctx context.Context,
	command types.GetMediaDownloadURLCommand,
) (types.GetMediaDownloadURLResult, error) {
	if err := command.Validate(); err != nil {
		return types.GetMediaDownloadURLResult{}, err
	}
	asset, err := useCase.repository.GetAsset(ctx, command.AuthContext.TenantID, strings.TrimSpace(command.AssetID))
	if err != nil {
		return types.GetMediaDownloadURLResult{}, err
	}
	if asset.ConversationID != strings.TrimSpace(command.ConversationID) {
		return types.GetMediaDownloadURLResult{}, types.NewPermissionDenied("conversation mismatch")
	}
	if asset.Status != types.AssetStatusReady {
		return types.GetMediaDownloadURLResult{}, types.NewFailedPrecondition("asset is not ready")
	}
	decision, err := useCase.visibilityChecker.CanDownload(ctx, command.AuthContext, asset, strings.TrimSpace(command.MessageID))
	if err != nil {
		return types.GetMediaDownloadURLResult{}, err
	}
	audit := types.AccessAudit{
		TenantID:       command.AuthContext.TenantID,
		AuditID:        randomID("audit"),
		AssetID:        asset.AssetID,
		UserID:         command.AuthContext.UserID,
		ConversationID: asset.ConversationID,
		MessageID:      strings.TrimSpace(command.MessageID),
		Variant:        command.EffectiveVariant(),
		DecisionSource: decision.Source,
		RequestID:      command.AuthContext.RequestID,
	}
	if !decision.Allowed {
		audit.Decision = "DENY"
		_ = useCase.repository.RecordAccessAudit(ctx, audit)
		return types.GetMediaDownloadURLResult{}, types.NewPermissionDenied("media download denied")
	}
	audit.Decision = "ALLOW"
	if err := useCase.repository.RecordAccessAudit(ctx, audit); err != nil {
		return types.GetMediaDownloadURLResult{}, err
	}
	expiresAt := useCase.now().Add(types.DefaultDownloadURLTTL)
	presign, err := useCase.objectStore.PresignGet(ctx, asset.ObjectKey, command.EffectiveVariant(), expiresAt)
	if err != nil {
		return types.GetMediaDownloadURLResult{}, err
	}
	return types.GetMediaDownloadURLResult{
		AssetID:         asset.AssetID,
		Variant:         command.EffectiveVariant(),
		DownloadURL:     presign.URL,
		RequiredHeaders: presign.RequiredHeaders,
		ExpiresAt:       presign.ExpiresAt,
	}, nil
}

type DeleteMediaAssetUseCase struct {
	repository Repository
}

func NewDeleteMediaAssetUseCase(repository Repository) *DeleteMediaAssetUseCase {
	return &DeleteMediaAssetUseCase{repository: repository}
}

func (useCase *DeleteMediaAssetUseCase) Execute(
	ctx context.Context,
	command types.DeleteMediaAssetCommand,
) (types.MediaAsset, error) {
	if err := command.Validate(); err != nil {
		return types.MediaAsset{}, err
	}
	return useCase.repository.DeleteMediaAsset(ctx, command)
}

type AllowAllVisibilityChecker struct{}

func NewAllowAllVisibilityChecker() AllowAllVisibilityChecker {
	return AllowAllVisibilityChecker{}
}

func (AllowAllVisibilityChecker) CanDownload(_ context.Context, _ types.AuthContext, _ types.MediaAsset, _ string) (VisibilityDecision, error) {
	return VisibilityDecision{Allowed: true, Source: "allow-all-local"}, nil
}

func objectKey(command types.CreateUploadSessionCommand) string {
	return fmt.Sprintf("%s/%s/%s", command.AuthContext.TenantID, command.ConversationID, randomID("object"))
}

func randomID(prefix string) string {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(buffer[:])
}
