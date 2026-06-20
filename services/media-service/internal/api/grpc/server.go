package grpc

import (
	"context"
	"errors"
	"time"

	mediav1 "github.com/qsyy0921/IM/api/proto/nexusim/media/v1"
	"github.com/qsyy0921/IM/services/media-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CreateUploadSessionExecutor interface {
	Execute(context.Context, types.CreateUploadSessionCommand) (types.CreateUploadSessionResult, error)
}

type CompleteUploadExecutor interface {
	Execute(context.Context, types.CompleteUploadCommand) (types.MediaAsset, error)
}

type GetMediaAssetExecutor interface {
	Execute(context.Context, types.GetMediaAssetCommand) (types.MediaAsset, error)
}

type GetMediaDownloadURLExecutor interface {
	Execute(context.Context, types.GetMediaDownloadURLCommand) (types.GetMediaDownloadURLResult, error)
}

type DeleteMediaAssetExecutor interface {
	Execute(context.Context, types.DeleteMediaAssetCommand) (types.MediaAsset, error)
}

type Server struct {
	mediav1.UnimplementedMediaServiceServer
	createUploadSession CreateUploadSessionExecutor
	completeUpload      CompleteUploadExecutor
	getMediaAsset       GetMediaAssetExecutor
	getDownloadURL      GetMediaDownloadURLExecutor
	deleteMediaAsset    DeleteMediaAssetExecutor
}

func NewServer(
	createUploadSession CreateUploadSessionExecutor,
	completeUpload CompleteUploadExecutor,
	getMediaAsset GetMediaAssetExecutor,
	getDownloadURL GetMediaDownloadURLExecutor,
	deleteMediaAsset DeleteMediaAssetExecutor,
) *Server {
	return &Server{
		createUploadSession: createUploadSession,
		completeUpload:      completeUpload,
		getMediaAsset:       getMediaAsset,
		getDownloadURL:      getDownloadURL,
		deleteMediaAsset:    deleteMediaAsset,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	mediav1.RegisterMediaServiceServer(registrar, server)
}

func (server *Server) CreateUploadSession(
	ctx context.Context,
	request *mediav1.CreateUploadSessionRequest,
) (*mediav1.CreateUploadSessionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.createUploadSession.Execute(ctx, types.CreateUploadSessionCommand{
		AuthContext:    auth,
		ConversationID: request.GetConversationId(),
		MediaKind:      mediaKindFromProto(request.GetMediaKind()),
		FileName:       request.GetFileName(),
		ContentType:    request.GetContentType(),
		SizeBytes:      request.GetSizeBytes(),
		SHA256:         request.GetSha256(),
		IdempotencyKey: request.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &mediav1.CreateUploadSessionResponse{
		AssetId:              result.Asset.AssetID,
		UploadSessionId:      result.Session.UploadSessionID,
		UploadUrl:            result.UploadURL,
		RequiredHeaders:      result.RequiredHeaders,
		ExpiresAtUnixMs:      unixMillis(result.Session.ExpiresAt),
		MaxSizeBytes:         result.MaxSizeBytes,
		AcceptedContentTypes: result.AcceptedTypes,
	}, nil
}

func (server *Server) CompleteUpload(
	ctx context.Context,
	request *mediav1.CompleteUploadRequest,
) (*mediav1.CompleteUploadResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	asset, err := server.completeUpload.Execute(ctx, types.CompleteUploadCommand{
		AuthContext:     auth,
		AssetID:         request.GetAssetId(),
		UploadSessionID: request.GetUploadSessionId(),
		SHA256:          request.GetSha256(),
		SizeBytes:       request.GetSizeBytes(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &mediav1.CompleteUploadResponse{
		Asset:                assetToProto(asset),
		MessageAttachmentRef: attachmentRefToProto(types.ToAttachmentRef(asset)),
	}, nil
}

func (server *Server) GetMediaAsset(
	ctx context.Context,
	request *mediav1.GetMediaAssetRequest,
) (*mediav1.GetMediaAssetResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	asset, err := server.getMediaAsset.Execute(ctx, types.GetMediaAssetCommand{
		AuthContext: auth,
		AssetID:     request.GetAssetId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &mediav1.GetMediaAssetResponse{Asset: assetToProto(asset)}, nil
}

func (server *Server) GetMediaDownloadURL(
	ctx context.Context,
	request *mediav1.GetMediaDownloadURLRequest,
) (*mediav1.GetMediaDownloadURLResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.getDownloadURL.Execute(ctx, types.GetMediaDownloadURLCommand{
		AuthContext:    auth,
		AssetID:        request.GetAssetId(),
		ConversationID: request.GetConversationId(),
		MessageID:      request.GetMessageId(),
		Variant:        variantFromProto(request.GetRequestedVariant()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &mediav1.GetMediaDownloadURLResponse{
		AssetId:         result.AssetID,
		Variant:         variantToProto(result.Variant),
		DownloadUrl:     result.DownloadURL,
		ExpiresAtUnixMs: unixMillis(result.ExpiresAt),
		RequiredHeaders: result.RequiredHeaders,
	}, nil
}

func (server *Server) DeleteMediaAsset(
	ctx context.Context,
	request *mediav1.DeleteMediaAssetRequest,
) (*mediav1.DeleteMediaAssetResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	asset, err := server.deleteMediaAsset.Execute(ctx, types.DeleteMediaAssetCommand{
		AuthContext:     auth,
		AssetID:         request.GetAssetId(),
		DeleteRequestID: request.GetDeleteRequestId(),
		Reason:          request.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &mediav1.DeleteMediaAssetResponse{Asset: assetToProto(asset)}, nil
}

func authFromProto(ctx context.Context, auth *mediav1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		if auth != nil {
			if verified.TraceID == "" {
				verified.TraceID = auth.GetTraceId()
			}
			if verified.RequestID == "" {
				verified.RequestID = auth.GetRequestId()
			}
		}
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  auth.GetDeviceId(),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}, true
}

func assetToProto(asset types.MediaAsset) *mediav1.MediaAsset {
	return &mediav1.MediaAsset{
		TenantId:         string(asset.TenantID),
		AssetId:          asset.AssetID,
		OwnerUserId:      string(asset.OwnerUserID),
		ConversationId:   asset.ConversationID,
		MediaKind:        mediaKindToProto(asset.MediaKind),
		ContentType:      asset.ContentType,
		FileName:         asset.FileName,
		SizeBytes:        asset.SizeBytes,
		Sha256:           asset.SHA256,
		Status:           assetStatusToProto(asset.Status),
		ScanStatus:       processingStatusToProto(asset.ScanStatus),
		ThumbnailStatus:  processingStatusToProto(asset.ThumbnailStatus),
		TranscodeStatus:  processingStatusToProto(asset.TranscodeStatus),
		CreatedAtUnixMs:  unixMillis(asset.CreatedAt),
		UploadedAtUnixMs: unixMillis(asset.UploadedAt),
		ReadyAtUnixMs:    unixMillis(asset.ReadyAt),
		DeletedAtUnixMs:  unixMillis(asset.DeletedAt),
	}
}

func attachmentRefToProto(ref types.MessageAttachmentRef) *mediav1.MessageAttachmentRef {
	return &mediav1.MessageAttachmentRef{
		AssetId:     ref.AssetID,
		MediaKind:   mediaKindToProto(ref.MediaKind),
		ContentType: ref.ContentType,
		FileName:    ref.FileName,
		SizeBytes:   ref.SizeBytes,
		Sha256:      ref.SHA256,
	}
}

func mediaKindFromProto(kind mediav1.MediaKind) string {
	switch kind {
	case mediav1.MediaKind_MEDIA_KIND_IMAGE:
		return types.MediaKindImage
	case mediav1.MediaKind_MEDIA_KIND_FILE:
		return types.MediaKindFile
	case mediav1.MediaKind_MEDIA_KIND_VOICE:
		return types.MediaKindVoice
	case mediav1.MediaKind_MEDIA_KIND_VIDEO:
		return types.MediaKindVideo
	default:
		return ""
	}
}

func mediaKindToProto(kind string) mediav1.MediaKind {
	switch kind {
	case types.MediaKindImage:
		return mediav1.MediaKind_MEDIA_KIND_IMAGE
	case types.MediaKindFile:
		return mediav1.MediaKind_MEDIA_KIND_FILE
	case types.MediaKindVoice:
		return mediav1.MediaKind_MEDIA_KIND_VOICE
	case types.MediaKindVideo:
		return mediav1.MediaKind_MEDIA_KIND_VIDEO
	default:
		return mediav1.MediaKind_MEDIA_KIND_UNSPECIFIED
	}
}

func assetStatusToProto(status string) mediav1.MediaAssetStatus {
	switch status {
	case types.AssetStatusUploadPending:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_UPLOAD_PENDING
	case types.AssetStatusUploaded:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_UPLOADED
	case types.AssetStatusProcessing:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_PROCESSING
	case types.AssetStatusReady:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_READY
	case types.AssetStatusQuarantined:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_QUARANTINED
	case types.AssetStatusFailed:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_FAILED
	case types.AssetStatusDeleted:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_DELETED
	case types.AssetStatusExpired:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_EXPIRED
	default:
		return mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_UNSPECIFIED
	}
}

func processingStatusToProto(status string) mediav1.MediaProcessingStatus {
	switch status {
	case types.ProcessingStatusPending:
		return mediav1.MediaProcessingStatus_MEDIA_PROCESSING_STATUS_PENDING
	case types.ProcessingStatusPassed:
		return mediav1.MediaProcessingStatus_MEDIA_PROCESSING_STATUS_PASSED
	case types.ProcessingStatusSkipped:
		return mediav1.MediaProcessingStatus_MEDIA_PROCESSING_STATUS_SKIPPED
	case types.ProcessingStatusFailed:
		return mediav1.MediaProcessingStatus_MEDIA_PROCESSING_STATUS_FAILED
	default:
		return mediav1.MediaProcessingStatus_MEDIA_PROCESSING_STATUS_UNSPECIFIED
	}
}

func variantFromProto(variant mediav1.MediaVariant) string {
	switch variant {
	case mediav1.MediaVariant_MEDIA_VARIANT_ORIGINAL:
		return types.VariantOriginal
	case mediav1.MediaVariant_MEDIA_VARIANT_THUMBNAIL:
		return types.VariantThumbnail
	case mediav1.MediaVariant_MEDIA_VARIANT_TRANSCODED:
		return types.VariantTranscoded
	default:
		return ""
	}
}

func variantToProto(variant string) mediav1.MediaVariant {
	switch variant {
	case types.VariantOriginal:
		return mediav1.MediaVariant_MEDIA_VARIANT_ORIGINAL
	case types.VariantThumbnail:
		return mediav1.MediaVariant_MEDIA_VARIANT_THUMBNAIL
	case types.VariantTranscoded:
		return mediav1.MediaVariant_MEDIA_VARIANT_TRANSCODED
	default:
		return mediav1.MediaVariant_MEDIA_VARIANT_UNSPECIFIED
	}
}

func unixMillis(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "failed precondition")
	case errors.Is(err, types.ErrMediaAssetNotFound), errors.Is(err, types.ErrUploadSessionNotFound):
		return status.Error(codes.NotFound, "media resource not found")
	case errors.Is(err, types.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "idempotency conflict")
	case errors.Is(err, types.ErrProviderUnavailable), errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "media service unavailable")
	default:
		return status.Error(codes.Internal, "media service internal error")
	}
}
