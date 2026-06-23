package httpbff

import (
	"context"
	"encoding/hex"
	"net/http"
	"strings"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	mediav1 "github.com/qsyy0921/IM/api/proto/nexusim/media/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const groupAvatarMediaURIPrefix = "media://asset/"

type createGroupAvatarUploadSessionRequest struct {
	FileName       string `json:"file_name"`
	ContentType    string `json:"content_type"`
	SizeBytes      int64  `json:"size_bytes"`
	SHA256         string `json:"sha256"`
	IdempotencyKey string `json:"idempotency_key"`
}

type completeGroupAvatarUploadRequest struct {
	AssetID                string `json:"asset_id"`
	UploadSessionID        string `json:"upload_session_id"`
	SHA256                 string `json:"sha256"`
	SizeBytes              int64  `json:"size_bytes"`
	ExpectedProfileVersion int64  `json:"expected_profile_version"`
}

type groupAvatarUploadSessionResponse struct {
	AssetID              string            `json:"asset_id"`
	UploadSessionID      string            `json:"upload_session_id"`
	UploadURL            string            `json:"upload_url"`
	RequiredHeaders      map[string]string `json:"required_headers"`
	ExpiresAtUnixMS      int64             `json:"expires_at_unix_ms"`
	MaxSizeBytes         int64             `json:"max_size_bytes"`
	AcceptedContentTypes []string          `json:"accepted_content_types"`
}

type groupAvatarUploadCompleteResponse struct {
	AssetID   string                     `json:"asset_id"`
	AvatarURI string                     `json:"avatar_uri"`
	Profile   conversationProfilePayload `json:"profile"`
}

type groupAvatarDownloadURLResponse struct {
	AssetID         string            `json:"asset_id"`
	Variant         string            `json:"variant"`
	DownloadURL     string            `json:"download_url"`
	RequiredHeaders map[string]string `json:"required_headers"`
	ExpiresAtUnixMS int64             `json:"expires_at_unix_ms"`
}

type conversationProfilePayload struct {
	TenantID          string `json:"tenant_id"`
	ConversationID    string `json:"conversation_id"`
	ConversationType  string `json:"conversation_type"`
	Title             string `json:"title"`
	AvatarURI         string `json:"avatar_uri"`
	Announcement      string `json:"announcement"`
	ProfileVersion    int64  `json:"profile_version"`
	MemberVersion     int64  `json:"member_version"`
	PermissionVersion int64  `json:"permission_version"`
	UpdatedAtUnixMS   int64  `json:"updated_at_unix_ms"`
}

func (server *Server) handleCreateGroupAvatarUploadSession(response http.ResponseWriter, request *http.Request) {
	auth, err := server.authenticateRequest(request)
	if err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/avatar-upload-session")
	if err != nil {
		writeError(response, err)
		return
	}
	var input createGroupAvatarUploadSessionRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	sha, err := normalizeSHA256(input.SHA256)
	if err != nil {
		writeError(response, err)
		return
	}
	contentType, err := normalizeAvatarContentType(input.ContentType)
	if err != nil {
		writeError(response, err)
		return
	}
	idempotencyKey, err := requiredIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		writeError(response, err)
		return
	}
	fileName, err := requiredFileName(input.FileName)
	if err != nil {
		writeError(response, err)
		return
	}
	media, err := server.requireMedia()
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := media.CreateUploadSession(contextFromRequest(request), &mediav1.CreateUploadSessionRequest{
		AuthContext:    mediaAuthContext(auth, request),
		ConversationId: conversationID,
		MediaKind:      mediav1.MediaKind_MEDIA_KIND_IMAGE,
		FileName:       fileName,
		ContentType:    contentType,
		SizeBytes:      input.SizeBytes,
		Sha256:         sha,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, groupAvatarUploadSessionResponse{
		AssetID:              output.GetAssetId(),
		UploadSessionID:      output.GetUploadSessionId(),
		UploadURL:            output.GetUploadUrl(),
		RequiredHeaders:      output.GetRequiredHeaders(),
		ExpiresAtUnixMS:      output.GetExpiresAtUnixMs(),
		MaxSizeBytes:         output.GetMaxSizeBytes(),
		AcceptedContentTypes: output.GetAcceptedContentTypes(),
	})
}

func (server *Server) handleCompleteGroupAvatarUpload(response http.ResponseWriter, request *http.Request) {
	auth, err := server.authenticateRequest(request)
	if err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/avatar-upload-complete")
	if err != nil {
		writeError(response, err)
		return
	}
	var input completeGroupAvatarUploadRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	sha, err := normalizeSHA256(input.SHA256)
	if err != nil {
		writeError(response, err)
		return
	}
	assetID := strings.TrimSpace(input.AssetID)
	uploadSessionID := strings.TrimSpace(input.UploadSessionID)
	if assetID == "" || uploadSessionID == "" {
		writeError(response, status.Error(codes.InvalidArgument, "asset_id and upload_session_id are required"))
		return
	}
	media, err := server.requireMedia()
	if err != nil {
		writeError(response, err)
		return
	}
	complete, err := media.CompleteUpload(contextFromRequest(request), &mediav1.CompleteUploadRequest{
		AuthContext:     mediaAuthContext(auth, request),
		AssetId:         assetID,
		UploadSessionId: uploadSessionID,
		Sha256:          sha,
		SizeBytes:       input.SizeBytes,
	})
	if err != nil {
		writeError(response, err)
		return
	}
	asset := complete.GetAsset()
	if asset == nil || asset.GetAssetId() == "" {
		writeError(response, status.Error(codes.Internal, "media upload response is missing asset"))
		return
	}
	if asset.GetConversationId() != conversationID {
		writeError(response, status.Error(codes.PermissionDenied, "media asset conversation mismatch"))
		return
	}
	profileOutput, err := server.requireGateway().GetConversationProfile(contextFromRequest(request), &conversationv1.GetConversationProfileRequest{
		ConversationId: conversationID,
	})
	if err != nil {
		writeError(response, err)
		return
	}
	currentProfile := profileOutput.GetProfile()
	if currentProfile == nil {
		writeError(response, status.Error(codes.Internal, "conversation profile is missing"))
		return
	}
	if input.ExpectedProfileVersion > 0 && input.ExpectedProfileVersion != currentProfile.GetProfileVersion() {
		writeError(response, status.Error(codes.FailedPrecondition, "conversation profile version changed"))
		return
	}
	avatarURI := groupAvatarMediaURIPrefix + asset.GetAssetId()
	updated, err := server.requireGateway().UpdateConversationProfile(contextFromRequest(request), &conversationv1.UpdateConversationProfileRequest{
		ConversationId:         conversationID,
		Title:                  currentProfile.GetTitle(),
		AvatarUri:              avatarURI,
		Announcement:           currentProfile.GetAnnouncement(),
		ExpectedProfileVersion: currentProfile.GetProfileVersion(),
	})
	if err != nil {
		writeError(response, err)
		return
	}
	updatedProfile := updated.GetProfile()
	if updatedProfile == nil {
		writeError(response, status.Error(codes.Internal, "updated conversation profile is missing"))
		return
	}
	writeJSON(response, http.StatusOK, groupAvatarUploadCompleteResponse{
		AssetID:   asset.GetAssetId(),
		AvatarURI: avatarURI,
		Profile:   conversationProfilePayloadFromProto(updatedProfile),
	})
}

func (server *Server) handleGetGroupAvatarDownloadURL(response http.ResponseWriter, request *http.Request) {
	auth, err := server.authenticateRequest(request)
	if err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/avatar-download-url")
	if err != nil {
		writeError(response, err)
		return
	}
	avatarURI := strings.TrimSpace(request.URL.Query().Get("avatar_uri"))
	assetID, err := assetIDFromAvatarURI(avatarURI)
	if err != nil {
		writeError(response, err)
		return
	}
	profileOutput, err := server.requireGateway().GetConversationProfile(contextFromRequest(request), &conversationv1.GetConversationProfileRequest{
		ConversationId: conversationID,
	})
	if err != nil {
		writeError(response, err)
		return
	}
	currentProfile := profileOutput.GetProfile()
	if currentProfile == nil {
		writeError(response, status.Error(codes.Internal, "conversation profile is missing"))
		return
	}
	if strings.TrimSpace(currentProfile.GetAvatarUri()) != avatarURI {
		writeError(response, status.Error(codes.FailedPrecondition, "conversation avatar uri changed"))
		return
	}
	media, err := server.requireMedia()
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := media.GetMediaDownloadURL(contextFromRequest(request), &mediav1.GetMediaDownloadURLRequest{
		AuthContext:      mediaAuthContext(auth, request),
		AssetId:          assetID,
		ConversationId:   conversationID,
		RequestedVariant: mediav1.MediaVariant_MEDIA_VARIANT_ORIGINAL,
	})
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, groupAvatarDownloadURLResponse{
		AssetID:         output.GetAssetId(),
		Variant:         output.GetVariant().String(),
		DownloadURL:     output.GetDownloadUrl(),
		RequiredHeaders: output.GetRequiredHeaders(),
		ExpiresAtUnixMS: output.GetExpiresAtUnixMs(),
	})
}

func mediaAuthContext(auth gatewayauth.AuthContext, request *http.Request) *mediav1.AuthContext {
	return &mediav1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   firstNonEmpty(auth.TraceID, request.Header.Get("X-NexusIM-Trace-ID")),
		RequestId: firstNonEmpty(auth.RequestID, request.Header.Get("X-NexusIM-Request-ID")),
	}
}

func normalizeAvatarContentType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "image/png", "image/jpeg", "image/webp", "image/gif", "image/avif":
		return normalized, nil
	default:
		return "", status.Error(codes.InvalidArgument, "avatar content_type must be an image")
	}
}

func normalizeSHA256(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	decoded, err := hex.DecodeString(normalized)
	if err != nil || len(decoded) != 32 {
		return "", status.Error(codes.InvalidArgument, "sha256 must be 64 hex characters")
	}
	return normalized, nil
}

func requiredFileName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", status.Error(codes.InvalidArgument, "file_name is required")
	}
	return trimmed, nil
}

func assetIDFromAvatarURI(value string) (string, error) {
	if !strings.HasPrefix(value, groupAvatarMediaURIPrefix) {
		return "", status.Error(codes.InvalidArgument, "avatar_uri must reference a media asset")
	}
	assetID := strings.TrimSpace(strings.TrimPrefix(value, groupAvatarMediaURIPrefix))
	if assetID == "" || strings.ContainsAny(assetID, "/?#") {
		return "", status.Error(codes.InvalidArgument, "avatar_uri has invalid asset id")
	}
	return assetID, nil
}

func conversationProfilePayloadFromProto(profile *conversationv1.ConversationProfile) conversationProfilePayload {
	return conversationProfilePayload{
		TenantID:          profile.GetTenantId(),
		ConversationID:    profile.GetConversationId(),
		ConversationType:  profile.GetConversationType().String(),
		Title:             profile.GetTitle(),
		AvatarURI:         profile.GetAvatarUri(),
		Announcement:      profile.GetAnnouncement(),
		ProfileVersion:    profile.GetProfileVersion(),
		MemberVersion:     profile.GetMemberVersion(),
		PermissionVersion: profile.GetPermissionVersion(),
		UpdatedAtUnixMS:   profile.GetUpdatedAtUnixMs(),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var _ MediaClient = (*missingMedia)(nil)

type missingMedia struct{}

func (missingMedia) CreateUploadSession(context.Context, *mediav1.CreateUploadSessionRequest, ...grpc.CallOption) (*mediav1.CreateUploadSessionResponse, error) {
	return nil, status.Error(codes.Unavailable, "media service is not configured")
}

func (missingMedia) CompleteUpload(context.Context, *mediav1.CompleteUploadRequest, ...grpc.CallOption) (*mediav1.CompleteUploadResponse, error) {
	return nil, status.Error(codes.Unavailable, "media service is not configured")
}

func (missingMedia) GetMediaDownloadURL(context.Context, *mediav1.GetMediaDownloadURLRequest, ...grpc.CallOption) (*mediav1.GetMediaDownloadURLResponse, error) {
	return nil, status.Error(codes.Unavailable, "media service is not configured")
}
