package httpbff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	mediav1 "github.com/qsyy0921/IM/api/proto/nexusim/media/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	metadataToken       = "x-nexusim-gateway-token"
	metadataTraceID     = "x-nexusim-trace-id"
	metadataRequestID   = "x-nexusim-request-id"
	metadataTraceparent = "traceparent"
	maxBodyBytes        = 1 << 20
)

type Gateway interface {
	RegisterUser(ctx context.Context, request *identityv1.RegisterUserRequest) (*identityv1.RegisterUserResponse, error)
	Login(ctx context.Context, request *identityv1.LoginRequest) (*identityv1.LoginResponse, error)
	RefreshGatewayToken(ctx context.Context, request *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error)
	IssueGatewayToken(ctx context.Context, request *identityv1.IssueGatewayTokenRequest) (*identityv1.IssueGatewayTokenResponse, error)
	RevokeSession(ctx context.Context, request *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error)
	CreateConversation(ctx context.Context, request *conversationv1.CreateConversationRequest) (*conversationv1.CreateConversationResponse, error)
	CreateMemberChange(ctx context.Context, request *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error)
	ListConversationMembers(ctx context.Context, request *conversationv1.ListConversationMembersRequest) (*conversationv1.ListConversationMembersResponse, error)
	TransferConversationOwner(ctx context.Context, request *conversationv1.TransferConversationOwnerRequest) (*conversationv1.TransferConversationOwnerResponse, error)
	GetConversationProfile(ctx context.Context, request *conversationv1.GetConversationProfileRequest) (*conversationv1.GetConversationProfileResponse, error)
	UpdateConversationProfile(ctx context.Context, request *conversationv1.UpdateConversationProfileRequest) (*conversationv1.UpdateConversationProfileResponse, error)
	SendMessage(ctx context.Context, request *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error)
	PullInbox(ctx context.Context, request *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error)
	AckDelivery(ctx context.Context, request *deliveryv1.AckDeliveryRequest) (*deliveryv1.AckDeliveryResponse, error)
	ListConversations(ctx context.Context, request *receiptv1.ListConversationsRequest) (*receiptv1.ListConversationsResponse, error)
	PinConversation(ctx context.Context, request *receiptv1.PinConversationRequest) (*receiptv1.PinConversationResponse, error)
	MuteConversation(ctx context.Context, request *receiptv1.MuteConversationRequest) (*receiptv1.MuteConversationResponse, error)
	ListReceiptStates(ctx context.Context, request *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error)
	SendContactRequest(ctx context.Context, request *contactsv1.SendContactRequestRequest) (*contactsv1.SendContactRequestResponse, error)
	RespondContactRequest(ctx context.Context, request *contactsv1.RespondContactRequestRequest) (*contactsv1.RespondContactRequestResponse, error)
	CancelContactRequest(ctx context.Context, request *contactsv1.CancelContactRequestRequest) (*contactsv1.CancelContactRequestResponse, error)
	ListContactRequests(ctx context.Context, request *contactsv1.ListContactRequestsRequest) (*contactsv1.ListContactRequestsResponse, error)
	ListContacts(ctx context.Context, request *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error)
	GetContactState(ctx context.Context, request *contactsv1.GetContactStateRequest) (*contactsv1.GetContactStateResponse, error)
	DeleteContact(ctx context.Context, request *contactsv1.DeleteContactRequest) (*contactsv1.DeleteContactResponse, error)
	BlockContact(ctx context.Context, request *contactsv1.BlockContactRequest) (*contactsv1.BlockContactResponse, error)
	UnblockContact(ctx context.Context, request *contactsv1.UnblockContactRequest) (*contactsv1.UnblockContactResponse, error)
	UpdateContactRemark(ctx context.Context, request *contactsv1.UpdateContactRemarkRequest) (*contactsv1.UpdateContactRemarkResponse, error)
	UpdateContactGroup(ctx context.Context, request *contactsv1.UpdateContactGroupRequest) (*contactsv1.UpdateContactGroupResponse, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (gatewayauth.AuthContext, error)
}

type MediaClient interface {
	CreateUploadSession(ctx context.Context, request *mediav1.CreateUploadSessionRequest, opts ...grpc.CallOption) (*mediav1.CreateUploadSessionResponse, error)
	CompleteUpload(ctx context.Context, request *mediav1.CompleteUploadRequest, opts ...grpc.CallOption) (*mediav1.CompleteUploadResponse, error)
	GetMediaDownloadURL(ctx context.Context, request *mediav1.GetMediaDownloadURLRequest, opts ...grpc.CallOption) (*mediav1.GetMediaDownloadURLResponse, error)
}

type PushTokenIssuer interface {
	IssuePushToken(context.Context, PushTokenRequest) (PushToken, error)
}

type PushTokenRequest struct {
	TenantID   string
	UserID     string
	DeviceID   string
	SessionID  string
	TraceID    string
	RequestID  string
	TTLSeconds int64
}

type PushToken struct {
	Token           string
	Audience        string
	ExpiresAtUnixMS int64
}

type HMACPushTokenIssuer struct {
	secret string
	ttl    time.Duration
	now    func() time.Time
}

func NewHMACPushTokenIssuer(secret string, ttl time.Duration) *HMACPushTokenIssuer {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &HMACPushTokenIssuer{
		secret: secret,
		ttl:    ttl,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (issuer *HMACPushTokenIssuer) IssuePushToken(_ context.Context, request PushTokenRequest) (PushToken, error) {
	if issuer == nil || strings.TrimSpace(issuer.secret) == "" {
		return PushToken{}, nil
	}
	now := time.Now().UTC()
	if issuer.now != nil {
		now = issuer.now().UTC()
	}
	expiresAt := now.Add(issuer.ttl)
	token, err := gatewayauth.SignGatewayToken(issuer.secret, map[string]string{
		"tenant_id":  request.TenantID,
		"user_id":    request.UserID,
		"device_id":  request.DeviceID,
		"session_id": request.SessionID,
		"trace_id":   request.TraceID,
		"aud":        "push-gateway",
	}, expiresAt)
	if err != nil {
		return PushToken{}, status.Error(codes.Internal, "failed to issue push token")
	}
	return PushToken{
		Token:           token,
		Audience:        "push-gateway",
		ExpiresAtUnixMS: expiresAt.UnixMilli(),
	}, nil
}

type Config struct {
	Gateway        Gateway
	Media          MediaClient
	Authenticator  Authenticator
	PushTokens     PushTokenIssuer
	Metrics        MetricsRecorder
	RateLimiter    RateLimiter
	AllowedOrigins []string
}

type Server struct {
	gateway        Gateway
	media          MediaClient
	authenticator  Authenticator
	pushTokens     PushTokenIssuer
	metrics        MetricsRecorder
	rateLimiter    RateLimiter
	allowedOrigins map[string]struct{}
	allowAnyOrigin bool
	marshal        protojson.MarshalOptions
	unmarshal      protojson.UnmarshalOptions
}

type errorPayload struct {
	Error publicError `json:"error"`
}

type publicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type openDirectConversationRequest struct {
	PeerUserID     string `json:"peer_user_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type conversationMemberChangeRequest struct {
	TargetUserID    string `json:"target_user_id"`
	TargetRole      string `json:"target_role"`
	IdempotencyKey  string `json:"idempotency_key"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expected_member_version"`
}

type transferConversationOwnerRequest struct {
	NewOwnerUserID  string `json:"new_owner_user_id"`
	IdempotencyKey  string `json:"idempotency_key"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expected_member_version"`
}

type updateConversationProfileRequest struct {
	Title                  string `json:"title"`
	AvatarURI              string `json:"avatar_uri"`
	Announcement           string `json:"announcement"`
	ExpectedProfileVersion int64  `json:"expected_profile_version"`
}

func NewServer(config Config) *Server {
	server := &Server{
		gateway:       config.Gateway,
		media:         config.Media,
		authenticator: config.Authenticator,
		pushTokens:    config.PushTokens,
		metrics:       config.Metrics,
		rateLimiter:   config.RateLimiter,
		marshal: protojson.MarshalOptions{
			UseProtoNames:   true,
			EmitUnpopulated: false,
		},
		unmarshal: protojson.UnmarshalOptions{
			DiscardUnknown: false,
		},
		allowedOrigins: make(map[string]struct{}),
	}
	for _, origin := range config.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			server.allowAnyOrigin = true
			continue
		}
		server.allowedOrigins[origin] = struct{}{}
	}
	return server
}

func (server *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	started := time.Now()
	route := RouteName(request)
	recorder := newStatusRecorder(response)
	server.serveHTTP(recorder, request, route)
	server.recordMetrics(route, request.Method, recorder.statusCode(), time.Since(started))
}

func (server *Server) serveHTTP(response http.ResponseWriter, request *http.Request, route string) {
	if server.handleCORS(response, request) {
		return
	}
	if !server.checkRateLimit(response, request, route) {
		return
	}
	path := strings.TrimRight(request.URL.Path, "/")
	if path == "" {
		path = "/"
	}
	switch {
	case request.Method == http.MethodGet && path == "/api/health":
		writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
	case request.Method == http.MethodPost && path == "/api/auth/login":
		server.handleLogin(response, request)
	case request.Method == http.MethodPost && path == "/api/auth/register":
		server.handleRegister(response, request)
	case request.Method == http.MethodPost && path == "/api/auth/refresh":
		server.handleRefresh(response, request)
	case request.Method == http.MethodPost && path == "/api/auth/logout":
		server.handleLogout(response, request)
	case request.Method == http.MethodGet && path == "/api/me":
		server.handleMe(response, request)
	case request.Method == http.MethodGet && path == "/api/conversations":
		server.handleListConversations(response, request)
	case request.Method == http.MethodPost && path == "/api/conversations/create":
		server.handleCreateConversation(response, request)
	case request.Method == http.MethodPost && path == "/api/conversations/direct":
		server.handleOpenDirectConversation(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/pin"):
		server.handlePinConversation(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/mute"):
		server.handleMuteConversation(response, request)
	case request.Method == http.MethodGet && isConversationMemberActionPath(request.URL.EscapedPath(), "/members"):
		server.handleListConversationMembers(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/members/invite"):
		server.handleInviteConversationMember(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/members/leave"):
		server.handleLeaveConversation(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/members/remove"):
		server.handleRemoveConversationMember(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/members/role"):
		server.handleUpdateConversationMemberRole(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/owner/transfer"):
		server.handleTransferConversationOwner(response, request)
	case request.Method == http.MethodGet && isConversationMemberActionPath(request.URL.EscapedPath(), "/profile"):
		server.handleGetConversationProfile(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/profile"):
		server.handleUpdateConversationProfile(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/avatar-upload-session"):
		server.handleCreateGroupAvatarUploadSession(response, request)
	case request.Method == http.MethodPost && isConversationMemberActionPath(request.URL.EscapedPath(), "/avatar-upload-complete"):
		server.handleCompleteGroupAvatarUpload(response, request)
	case request.Method == http.MethodGet && isConversationMemberActionPath(request.URL.EscapedPath(), "/avatar-download-url"):
		server.handleGetGroupAvatarDownloadURL(response, request)
	case request.Method == http.MethodGet && isConversationMessagesPath(request.URL.EscapedPath()):
		server.handleConversationMessages(response, request)
	case request.Method == http.MethodPost && path == "/api/messages/send":
		server.handleSendMessage(response, request)
	case request.Method == http.MethodPost && path == "/api/delivery/ack":
		server.handleAckDelivery(response, request)
	case request.Method == http.MethodGet && path == "/api/contact-requests":
		server.handleListContactRequests(response, request)
	case request.Method == http.MethodPost && path == "/api/contact-requests/send":
		server.handleSendContactRequest(response, request)
	case request.Method == http.MethodPost && path == "/api/contact-requests/respond":
		server.handleRespondContactRequest(response, request)
	case request.Method == http.MethodPost && path == "/api/contact-requests/cancel":
		server.handleCancelContactRequest(response, request)
	case request.Method == http.MethodGet && path == "/api/contacts":
		server.handleListContacts(response, request)
	case request.Method == http.MethodGet && path == "/api/contacts/state":
		server.handleGetContactState(response, request)
	case request.Method == http.MethodPost && path == "/api/contacts/delete":
		server.handleDeleteContact(response, request)
	case request.Method == http.MethodPost && path == "/api/contacts/block":
		server.handleBlockContact(response, request)
	case request.Method == http.MethodPost && path == "/api/contacts/unblock":
		server.handleUnblockContact(response, request)
	case request.Method == http.MethodPost && path == "/api/contacts/remark":
		server.handleUpdateContactRemark(response, request)
	case request.Method == http.MethodPost && path == "/api/contacts/group":
		server.handleUpdateContactGroup(response, request)
	case request.Method == http.MethodGet && path == "/api/receipts":
		server.handleListReceiptStates(response, request)
	default:
		writeError(response, status.Error(codes.NotFound, "endpoint not found"))
	}
}

func (server *Server) handleLogin(response http.ResponseWriter, request *http.Request) {
	var input identityv1.LoginRequest
	if !server.decode(response, request, &input) {
		return
	}
	input.Audience = "api-gateway"
	output, err := server.requireGateway().Login(contextFromRequest(request), &input)
	server.writeAuthResponseOrError(response, request, output, nil, err)
}

func (server *Server) handleRegister(response http.ResponseWriter, request *http.Request) {
	var input identityv1.RegisterUserRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().RegisterUser(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleRefresh(response http.ResponseWriter, request *http.Request) {
	var input identityv1.RefreshGatewayTokenRequest
	if !server.decode(response, request, &input) {
		return
	}
	input.Audience = "api-gateway"
	output, err := server.requireGateway().RefreshGatewayToken(contextFromRequest(request), &input)
	server.writeAuthResponseOrError(response, request, nil, output, err)
}

func (server *Server) handleLogout(response http.ResponseWriter, request *http.Request) {
	auth, err := server.authenticateRequest(request)
	if err != nil {
		writeError(response, err)
		return
	}
	input := &identityv1.RevokeSessionRequest{
		AdminContext: &identityv1.AdminContext{
			TenantId:       auth.TenantID,
			OperatorUserId: auth.UserID,
			TraceId:        auth.TraceID,
			RequestId:      auth.RequestID,
		},
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		Reason:    "client logout",
	}
	output, err := server.requireGateway().RevokeSession(contextFromRequest(request), input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleMe(response http.ResponseWriter, request *http.Request) {
	auth, err := server.authenticateRequest(request)
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{
		"tenant_id":  auth.TenantID,
		"user_id":    auth.UserID,
		"device_id":  auth.DeviceID,
		"session_id": auth.SessionID,
	})
}

func (server *Server) authenticateRequest(request *http.Request) (gatewayauth.AuthContext, error) {
	if server.authenticator == nil {
		return gatewayauth.AuthContext{}, status.Error(codes.Internal, "gateway auth is not configured")
	}
	authRequest := request.Clone(request.Context())
	if authRequest.Header.Get("Authorization") == "" {
		if token := strings.TrimSpace(authRequest.Header.Get("X-NexusIM-Gateway-Token")); token != "" {
			authRequest.Header.Set("Authorization", "Bearer "+token)
		}
	}
	auth, err := server.authenticator.Authenticate(authRequest)
	if err != nil {
		return gatewayauth.AuthContext{}, publicAuthError(err)
	}
	return auth, nil
}

func (server *Server) handleListConversations(response http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	limit, err := int32Query(query, "limit")
	if err != nil {
		writeError(response, err)
		return
	}
	sortValue, err := int32Query(query, "sort")
	if err != nil {
		writeError(response, err)
		return
	}
	input := &receiptv1.ListConversationsRequest{
		Limit:                     limit,
		PageCursor:                firstQuery(query, "page_cursor", "pageCursor"),
		Sort:                      receiptv1.ConversationListSort(sortValue),
		IncludeArchived:           boolQuery(query, "include_archived", "includeArchived"),
		UnreadOnly:                boolQuery(query, "unread_only", "unreadOnly"),
		PinnedOnly:                boolQuery(query, "pinned_only", "pinnedOnly"),
		MutedOnly:                 boolQuery(query, "muted_only", "mutedOnly"),
		TagFilter:                 firstQuery(query, "tag_filter", "tagFilter"),
		DraftOnly:                 boolQuery(query, "draft_only", "draftOnly"),
		ArchivedOnly:              boolQuery(query, "archived_only", "archivedOnly"),
		TagFilters:                query["tag_filters"],
		LastSourceEventTypeFilter: firstQuery(query, "last_source_event_type_filter", "lastSourceEventTypeFilter"),
		ExcludeMuted:              boolQuery(query, "exclude_muted", "excludeMuted"),
	}
	output, err := server.requireGateway().ListConversations(contextFromRequest(request), input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleCreateConversation(response http.ResponseWriter, request *http.Request) {
	var input conversationv1.CreateConversationRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().CreateConversation(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleOpenDirectConversation(response http.ResponseWriter, request *http.Request) {
	auth, err := server.authenticateRequest(request)
	if err != nil {
		writeError(response, err)
		return
	}
	var input openDirectConversationRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	peerUserID := strings.TrimSpace(input.PeerUserID)
	if peerUserID == "" {
		writeError(response, status.Error(codes.InvalidArgument, "peer_user_id is required"))
		return
	}
	if peerUserID == auth.UserID {
		writeError(response, status.Error(codes.InvalidArgument, "peer_user_id must differ from current user"))
		return
	}
	contact, err := server.requireGateway().GetContactState(contextFromRequest(request), &contactsv1.GetContactStateRequest{
		OtherUserId: peerUserID,
	})
	if err != nil {
		writeError(response, err)
		return
	}
	if contact.GetStatus() != contactsv1.ContactEdgeStatus_CONTACT_EDGE_STATUS_ACTIVE {
		writeError(response, status.Error(codes.PermissionDenied, "contact is not active"))
		return
	}
	conversationID := directConversationID(auth.TenantID, auth.UserID, peerUserID)
	idempotencyKey, err := requiredIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := server.requireGateway().CreateConversation(contextFromRequest(request), &conversationv1.CreateConversationRequest{
		ConversationId:   conversationID,
		ConversationType: conversationv1.ConversationType_CONVERSATION_TYPE_DIRECT,
		IdempotencyKey:   idempotencyKey,
		DirectPeerUserId: peerUserID,
	})
	server.writeProtoOrError(response, output, err)
}

type conversationPinRequest struct {
	Pinned bool `json:"pinned"`
}

type conversationMuteRequest struct {
	Muted bool `json:"muted"`
}

func (server *Server) handlePinConversation(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/pin")
	if err != nil {
		writeError(response, err)
		return
	}
	var input conversationPinRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	output, err := server.requireGateway().PinConversation(contextFromRequest(request), &receiptv1.PinConversationRequest{
		ConversationId: conversationID,
		Pinned:         input.Pinned,
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleMuteConversation(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/mute")
	if err != nil {
		writeError(response, err)
		return
	}
	var input conversationMuteRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	output, err := server.requireGateway().MuteConversation(contextFromRequest(request), &receiptv1.MuteConversationRequest{
		ConversationId: conversationID,
		Muted:          input.Muted,
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleInviteConversationMember(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/members/invite")
	if err != nil {
		writeError(response, err)
		return
	}
	var input conversationMemberChangeRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	targetUserID := strings.TrimSpace(input.TargetUserID)
	if targetUserID == "" {
		writeError(response, status.Error(codes.InvalidArgument, "target_user_id is required"))
		return
	}
	idempotencyKey, err := requiredIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := server.requireGateway().CreateMemberChange(contextFromRequest(request), &conversationv1.CreateMemberChangeRequest{
		ConversationId:        conversationID,
		TargetUserId:          targetUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: input.ExpectedVersion,
		IdempotencyKey:        idempotencyKey,
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                strings.TrimSpace(input.Reason),
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleListConversationMembers(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/members")
	if err != nil {
		writeError(response, err)
		return
	}
	query := request.URL.Query()
	pageSize, err := int32Query(query, "page_size", "pageSize")
	if err != nil {
		writeError(response, err)
		return
	}
	roleFilter, err := memberRoleFromString(firstQuery(query, "role_filter", "role"))
	if err != nil {
		writeError(response, err)
		return
	}
	input := &conversationv1.ListConversationMembersRequest{
		ConversationId: conversationID,
		PageSize:       pageSize,
		PageToken:      strings.TrimSpace(firstQuery(query, "page_token", "pageToken")),
		RoleFilter:     roleFilter,
		UserIdPrefix:   strings.TrimSpace(firstQuery(query, "user_id_prefix", "userIdPrefix")),
		Sort:           conversationv1.ConversationMemberListSort_CONVERSATION_MEMBER_LIST_SORT_ROLE_USER_ID_ASC,
	}
	output, err := server.requireGateway().ListConversationMembers(contextFromRequest(request), input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleLeaveConversation(response http.ResponseWriter, request *http.Request) {
	auth, err := server.authenticateRequest(request)
	if err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/members/leave")
	if err != nil {
		writeError(response, err)
		return
	}
	var input conversationMemberChangeRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	idempotencyKey, err := requiredIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := server.requireGateway().CreateMemberChange(contextFromRequest(request), &conversationv1.CreateMemberChangeRequest{
		ConversationId:        conversationID,
		TargetUserId:          auth.UserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_LEAVE,
		ExpectedMemberVersion: input.ExpectedVersion,
		IdempotencyKey:        idempotencyKey,
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                strings.TrimSpace(input.Reason),
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleRemoveConversationMember(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/members/remove")
	if err != nil {
		writeError(response, err)
		return
	}
	var input conversationMemberChangeRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	targetUserID := strings.TrimSpace(input.TargetUserID)
	if targetUserID == "" {
		writeError(response, status.Error(codes.InvalidArgument, "target_user_id is required"))
		return
	}
	idempotencyKey, err := requiredIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := server.requireGateway().CreateMemberChange(contextFromRequest(request), &conversationv1.CreateMemberChangeRequest{
		ConversationId:        conversationID,
		TargetUserId:          targetUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_REMOVE,
		ExpectedMemberVersion: input.ExpectedVersion,
		IdempotencyKey:        idempotencyKey,
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                strings.TrimSpace(input.Reason),
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleUpdateConversationMemberRole(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/members/role")
	if err != nil {
		writeError(response, err)
		return
	}
	var input conversationMemberChangeRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	targetUserID := strings.TrimSpace(input.TargetUserID)
	if targetUserID == "" {
		writeError(response, status.Error(codes.InvalidArgument, "target_user_id is required"))
		return
	}
	targetRole, err := memberRoleFromString(input.TargetRole)
	if err != nil {
		writeError(response, err)
		return
	}
	if targetRole != conversationv1.MemberRole_MEMBER_ROLE_ADMIN && targetRole != conversationv1.MemberRole_MEMBER_ROLE_MEMBER {
		writeError(response, status.Error(codes.InvalidArgument, "target_role must be ADMIN or MEMBER"))
		return
	}
	idempotencyKey, err := requiredIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := server.requireGateway().CreateMemberChange(contextFromRequest(request), &conversationv1.CreateMemberChangeRequest{
		ConversationId:        conversationID,
		TargetUserId:          targetUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_ROLE_CHANGED,
		TargetRole:            targetRole,
		ExpectedMemberVersion: input.ExpectedVersion,
		IdempotencyKey:        idempotencyKey,
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                strings.TrimSpace(input.Reason),
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleTransferConversationOwner(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/owner/transfer")
	if err != nil {
		writeError(response, err)
		return
	}
	var input transferConversationOwnerRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	newOwnerUserID := strings.TrimSpace(input.NewOwnerUserID)
	if newOwnerUserID == "" {
		writeError(response, status.Error(codes.InvalidArgument, "new_owner_user_id is required"))
		return
	}
	idempotencyKey, err := requiredIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := server.requireGateway().TransferConversationOwner(contextFromRequest(request), &conversationv1.TransferConversationOwnerRequest{
		ConversationId:        conversationID,
		NewOwnerUserId:        newOwnerUserID,
		ExpectedMemberVersion: input.ExpectedVersion,
		IdempotencyKey:        idempotencyKey,
		Reason:                strings.TrimSpace(input.Reason),
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleGetConversationProfile(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/profile")
	if err != nil {
		writeError(response, err)
		return
	}
	output, err := server.requireGateway().GetConversationProfile(contextFromRequest(request), &conversationv1.GetConversationProfileRequest{
		ConversationId: conversationID,
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleUpdateConversationProfile(response http.ResponseWriter, request *http.Request) {
	if _, err := server.authenticateRequest(request); err != nil {
		writeError(response, err)
		return
	}
	conversationID, err := conversationIDFromMemberActionPath(request.URL.EscapedPath(), "/profile")
	if err != nil {
		writeError(response, err)
		return
	}
	var input updateConversationProfileRequest
	if !server.decodeJSON(response, request, &input) {
		return
	}
	output, err := server.requireGateway().UpdateConversationProfile(contextFromRequest(request), &conversationv1.UpdateConversationProfileRequest{
		ConversationId:         conversationID,
		Title:                  input.Title,
		AvatarUri:              input.AvatarURI,
		Announcement:           input.Announcement,
		ExpectedProfileVersion: input.ExpectedProfileVersion,
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleConversationMessages(response http.ResponseWriter, request *http.Request) {
	conversationID, err := conversationIDFromMessagesPath(request.URL.EscapedPath())
	if err != nil {
		writeError(response, err)
		return
	}
	query := request.URL.Query()
	afterSeq, err := int64Query(query, "after_seq", "afterSeq")
	if err != nil {
		writeError(response, err)
		return
	}
	limit, err := int32Query(query, "limit")
	if err != nil {
		writeError(response, err)
		return
	}
	input := &deliveryv1.PullInboxRequest{
		ConversationId: conversationID,
		AfterSeq:       afterSeq,
		Limit:          limit,
	}
	output, err := server.requireGateway().PullInbox(contextFromRequest(request), input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleSendMessage(response http.ResponseWriter, request *http.Request) {
	var input messagev1.SendMessageRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().SendMessage(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleAckDelivery(response http.ResponseWriter, request *http.Request) {
	var input deliveryv1.AckDeliveryRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().AckDelivery(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleListContacts(response http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	pageSize, err := int32Query(query, "page_size", "pageSize")
	if err != nil {
		writeError(response, err)
		return
	}
	input := &contactsv1.ListContactsRequest{
		PageSize:  pageSize,
		PageToken: firstQuery(query, "page_token", "pageToken"),
		Query:     firstQuery(query, "query"),
		GroupName: firstQuery(query, "group_name", "groupName"),
	}
	output, err := server.requireGateway().ListContacts(contextFromRequest(request), input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleSendContactRequest(response http.ResponseWriter, request *http.Request) {
	var input contactsv1.SendContactRequestRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().SendContactRequest(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleRespondContactRequest(response http.ResponseWriter, request *http.Request) {
	var input contactsv1.RespondContactRequestRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().RespondContactRequest(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleCancelContactRequest(response http.ResponseWriter, request *http.Request) {
	var input contactsv1.CancelContactRequestRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().CancelContactRequest(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleListContactRequests(response http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	pageSize, err := int32Query(query, "page_size", "pageSize")
	if err != nil {
		writeError(response, err)
		return
	}
	direction, err := contactRequestDirectionQuery(firstQuery(query, "direction"))
	if err != nil {
		writeError(response, err)
		return
	}
	statusFilter, err := contactRequestStatusQuery(firstQuery(query, "status"))
	if err != nil {
		writeError(response, err)
		return
	}
	sourceTypeFilter, err := contactRequestSourceTypeQuery(firstQuery(query, "source_type_filter", "sourceTypeFilter"))
	if err != nil {
		writeError(response, err)
		return
	}
	riskLevelFilter, err := contactRequestRiskLevelQuery(firstQuery(query, "risk_level_filter", "riskLevelFilter"))
	if err != nil {
		writeError(response, err)
		return
	}
	input := &contactsv1.ListContactRequestsRequest{
		Direction:        direction,
		Status:           statusFilter,
		PageSize:         pageSize,
		PageToken:        firstQuery(query, "page_token", "pageToken"),
		SourceTypeFilter: sourceTypeFilter,
		RiskLevelFilter:  riskLevelFilter,
	}
	if value := firstQuery(query, "review_required_filter", "reviewRequiredFilter"); value != "" {
		enabled := boolQuery(query, "review_required_filter", "reviewRequiredFilter")
		input.ReviewRequiredFilter = &enabled
	}
	output, err := server.requireGateway().ListContactRequests(contextFromRequest(request), input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleGetContactState(response http.ResponseWriter, request *http.Request) {
	otherUserID := firstQuery(request.URL.Query(), "other_user_id", "otherUserId")
	if otherUserID == "" {
		writeError(response, status.Error(codes.InvalidArgument, "other_user_id is required"))
		return
	}
	output, err := server.requireGateway().GetContactState(contextFromRequest(request), &contactsv1.GetContactStateRequest{
		OtherUserId: otherUserID,
	})
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleDeleteContact(response http.ResponseWriter, request *http.Request) {
	var input contactsv1.DeleteContactRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().DeleteContact(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleBlockContact(response http.ResponseWriter, request *http.Request) {
	var input contactsv1.BlockContactRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().BlockContact(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleUnblockContact(response http.ResponseWriter, request *http.Request) {
	var input contactsv1.UnblockContactRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().UnblockContact(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleUpdateContactRemark(response http.ResponseWriter, request *http.Request) {
	var input contactsv1.UpdateContactRemarkRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().UpdateContactRemark(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleUpdateContactGroup(response http.ResponseWriter, request *http.Request) {
	var input contactsv1.UpdateContactGroupRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().UpdateContactGroup(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleListReceiptStates(response http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	receivedDeviceLimit, err := int32Query(query, "received_device_limit", "receivedDeviceLimit")
	if err != nil {
		writeError(response, err)
		return
	}
	conversationSeq, err := int64Query(query, "conversation_seq", "conversationSeq")
	if err != nil {
		writeError(response, err)
		return
	}
	item := receiptv1.ReceiptStateQuery{
		MessageId:       firstQuery(query, "message_id", "messageId"),
		ConversationSeq: conversationSeq,
	}
	input := &receiptv1.ListReceiptStatesRequest{
		ConversationId:         firstQuery(query, "conversation_id", "conversationId"),
		IncludeReceivedDevices: boolQuery(query, "include_received_devices", "includeReceivedDevices"),
		ReceivedDeviceLimit:    receivedDeviceLimit,
	}
	if item.MessageId != "" || item.ConversationSeq > 0 {
		input.Items = []*receiptv1.ReceiptStateQuery{&item}
	}
	output, err := server.requireGateway().ListReceiptStates(contextFromRequest(request), input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) decode(response http.ResponseWriter, request *http.Request, message proto.Message) bool {
	body, ok := readRequestBody(response, request)
	if !ok {
		return false
	}
	if err := server.unmarshal.Unmarshal(body, message); err != nil {
		writeError(response, status.Error(codes.InvalidArgument, "invalid json request"))
		return false
	}
	return true
}

func (server *Server) decodeJSON(response http.ResponseWriter, request *http.Request, target any) bool {
	body, ok := readRequestBody(response, request)
	if !ok {
		return false
	}
	if err := json.Unmarshal(body, target); err != nil {
		writeError(response, status.Error(codes.InvalidArgument, "invalid json request"))
		return false
	}
	return true
}

func readRequestBody(response http.ResponseWriter, request *http.Request) ([]byte, bool) {
	if request.Body == nil {
		writeError(response, status.Error(codes.InvalidArgument, "request body is required"))
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil {
		writeError(response, status.Error(codes.InvalidArgument, "failed to read request body"))
		return nil, false
	}
	if len(body) > maxBodyBytes {
		writeError(response, status.Error(codes.ResourceExhausted, "request body is too large"))
		return nil, false
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		writeError(response, status.Error(codes.InvalidArgument, "request body is required"))
		return nil, false
	}
	return body, true
}

func (server *Server) writeProtoOrError(response http.ResponseWriter, message proto.Message, err error) {
	if err != nil {
		writeError(response, err)
		return
	}
	if message == nil {
		writeError(response, status.Error(codes.Internal, "empty gateway response"))
		return
	}
	data, err := server.marshal.Marshal(message)
	if err != nil {
		writeError(response, status.Error(codes.Internal, "failed to encode response"))
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(data)
}

func (server *Server) writeAuthResponseOrError(
	response http.ResponseWriter,
	request *http.Request,
	login *identityv1.LoginResponse,
	refresh *identityv1.RefreshGatewayTokenResponse,
	err error,
) {
	if err != nil {
		writeError(response, err)
		return
	}
	var message proto.Message
	source := PushTokenRequest{}
	switch {
	case login != nil:
		message = login
		source = PushTokenRequest{
			TenantID:  login.GetTenantId(),
			UserID:    login.GetUserId(),
			DeviceID:  login.GetDeviceId(),
			SessionID: login.GetSessionId(),
			TraceID:   strings.TrimSpace(request.Header.Get("X-NexusIM-Trace-ID")),
			RequestID: strings.TrimSpace(request.Header.Get("X-NexusIM-Request-ID")),
		}
	case refresh != nil:
		message = refresh
		source = PushTokenRequest{
			TenantID:  refresh.GetTenantId(),
			UserID:    refresh.GetUserId(),
			DeviceID:  refresh.GetDeviceId(),
			SessionID: refresh.GetSessionId(),
			TraceID:   strings.TrimSpace(request.Header.Get("X-NexusIM-Trace-ID")),
			RequestID: strings.TrimSpace(request.Header.Get("X-NexusIM-Request-ID")),
		}
	default:
		writeError(response, status.Error(codes.Internal, "empty gateway response"))
		return
	}
	payload, err := server.protoJSONMap(message)
	if err != nil {
		writeError(response, status.Error(codes.Internal, "failed to encode response"))
		return
	}
	push, err := server.issuePushToken(request.Context(), source)
	if err != nil {
		writeError(response, err)
		return
	}
	if push.Token != "" {
		payload["push_gateway_token"] = push.Token
		payload["push_gateway_audience"] = push.Audience
		if push.ExpiresAtUnixMS > 0 {
			payload["push_gateway_expires_at_unix_ms"] = strconv.FormatInt(push.ExpiresAtUnixMS, 10)
		}
	}
	writeJSON(response, http.StatusOK, payload)
}

func (server *Server) protoJSONMap(message proto.Message) (map[string]any, error) {
	data, err := server.marshal.Marshal(message)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (server *Server) issuePushToken(ctx context.Context, request PushTokenRequest) (PushToken, error) {
	if server.pushTokens == nil {
		return PushToken{}, nil
	}
	if request.TenantID == "" || request.UserID == "" || request.DeviceID == "" || request.SessionID == "" {
		return PushToken{}, status.Error(codes.Internal, "gateway response is missing push token identity")
	}
	return server.pushTokens.IssuePushToken(ctx, request)
}

func (server *Server) requireGateway() Gateway {
	if server.gateway != nil {
		return server.gateway
	}
	return missingGateway{}
}

func (server *Server) requireMedia() (MediaClient, error) {
	if server.media == nil {
		return nil, status.Error(codes.Unavailable, "media service is not configured")
	}
	return server.media, nil
}

func (server *Server) handleCORS(response http.ResponseWriter, request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin != "" {
		if !server.originAllowed(origin) {
			writeError(response, status.Error(codes.PermissionDenied, "origin is not allowed"))
			return true
		}
		response.Header().Set("Access-Control-Allow-Origin", origin)
		response.Header().Set("Vary", "Origin")
		response.Header().Set("Access-Control-Allow-Headers", "authorization, content-type, x-nexusim-gateway-token, x-nexusim-trace-id, x-nexusim-request-id, traceparent")
		response.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func (server *Server) originAllowed(origin string) bool {
	if server.allowAnyOrigin {
		return true
	}
	_, ok := server.allowedOrigins[origin]
	return ok
}

type missingGateway struct{}

func (missingGateway) RegisterUser(context.Context, *identityv1.RegisterUserRequest) (*identityv1.RegisterUserResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) Login(context.Context, *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) RefreshGatewayToken(context.Context, *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) IssueGatewayToken(context.Context, *identityv1.IssueGatewayTokenRequest) (*identityv1.IssueGatewayTokenResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) RevokeSession(context.Context, *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) CreateConversation(context.Context, *conversationv1.CreateConversationRequest) (*conversationv1.CreateConversationResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) CreateMemberChange(context.Context, *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) ListConversationMembers(context.Context, *conversationv1.ListConversationMembersRequest) (*conversationv1.ListConversationMembersResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) TransferConversationOwner(context.Context, *conversationv1.TransferConversationOwnerRequest) (*conversationv1.TransferConversationOwnerResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) GetConversationProfile(context.Context, *conversationv1.GetConversationProfileRequest) (*conversationv1.GetConversationProfileResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) UpdateConversationProfile(context.Context, *conversationv1.UpdateConversationProfileRequest) (*conversationv1.UpdateConversationProfileResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) SendMessage(context.Context, *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) PullInbox(context.Context, *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) AckDelivery(context.Context, *deliveryv1.AckDeliveryRequest) (*deliveryv1.AckDeliveryResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) ListConversations(context.Context, *receiptv1.ListConversationsRequest) (*receiptv1.ListConversationsResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) PinConversation(context.Context, *receiptv1.PinConversationRequest) (*receiptv1.PinConversationResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) MuteConversation(context.Context, *receiptv1.MuteConversationRequest) (*receiptv1.MuteConversationResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) ListReceiptStates(context.Context, *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) SendContactRequest(context.Context, *contactsv1.SendContactRequestRequest) (*contactsv1.SendContactRequestResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) RespondContactRequest(context.Context, *contactsv1.RespondContactRequestRequest) (*contactsv1.RespondContactRequestResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) CancelContactRequest(context.Context, *contactsv1.CancelContactRequestRequest) (*contactsv1.CancelContactRequestResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) ListContactRequests(context.Context, *contactsv1.ListContactRequestsRequest) (*contactsv1.ListContactRequestsResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) ListContacts(context.Context, *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) GetContactState(context.Context, *contactsv1.GetContactStateRequest) (*contactsv1.GetContactStateResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) DeleteContact(context.Context, *contactsv1.DeleteContactRequest) (*contactsv1.DeleteContactResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) BlockContact(context.Context, *contactsv1.BlockContactRequest) (*contactsv1.BlockContactResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) UnblockContact(context.Context, *contactsv1.UnblockContactRequest) (*contactsv1.UnblockContactResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) UpdateContactRemark(context.Context, *contactsv1.UpdateContactRemarkRequest) (*contactsv1.UpdateContactRemarkResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) UpdateContactGroup(context.Context, *contactsv1.UpdateContactGroupRequest) (*contactsv1.UpdateContactGroupResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}

func contextFromRequest(request *http.Request) context.Context {
	pairs := make([]string, 0, 8)
	if value := strings.TrimSpace(request.Header.Get("Authorization")); value != "" {
		pairs = append(pairs, "authorization", value)
	}
	if value := strings.TrimSpace(request.Header.Get("X-NexusIM-Gateway-Token")); value != "" {
		pairs = append(pairs, metadataToken, value)
	}
	if value := strings.TrimSpace(request.Header.Get("X-NexusIM-Trace-ID")); value != "" {
		pairs = append(pairs, metadataTraceID, value)
	}
	if value := strings.TrimSpace(request.Header.Get("X-NexusIM-Request-ID")); value != "" {
		pairs = append(pairs, metadataRequestID, value)
	}
	if value := strings.TrimSpace(request.Header.Get("Traceparent")); value != "" {
		pairs = append(pairs, metadataTraceparent, value)
	}
	if len(pairs) == 0 {
		return request.Context()
	}
	return metadata.NewIncomingContext(request.Context(), metadata.Pairs(pairs...))
}

func isConversationMessagesPath(escapedPath string) bool {
	const prefix = "/api/conversations/"
	const suffix = "/messages"
	if !strings.HasPrefix(escapedPath, prefix) || !strings.HasSuffix(escapedPath, suffix) {
		return false
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix)
	return strings.Trim(encoded, "/") != ""
}

func conversationIDFromMessagesPath(escapedPath string) (string, error) {
	const prefix = "/api/conversations/"
	const suffix = "/messages"
	if !isConversationMessagesPath(escapedPath) {
		return "", status.Error(codes.NotFound, "endpoint not found")
	}
	encoded := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix), "/")
	decoded, err := url.PathUnescape(encoded)
	if err != nil || strings.TrimSpace(decoded) == "" {
		return "", status.Error(codes.InvalidArgument, "conversation_id is invalid")
	}
	return decoded, nil
}

func isConversationMemberActionPath(escapedPath string, suffix string) bool {
	const prefix = "/api/conversations/"
	if !strings.HasPrefix(escapedPath, prefix) || !strings.HasSuffix(escapedPath, suffix) {
		return false
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix)
	return strings.Trim(encoded, "/") != ""
}

func conversationIDFromMemberActionPath(escapedPath string, suffix string) (string, error) {
	const prefix = "/api/conversations/"
	if !isConversationMemberActionPath(escapedPath, suffix) {
		return "", status.Error(codes.NotFound, "endpoint not found")
	}
	encoded := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(escapedPath, prefix), suffix), "/")
	decoded, err := url.PathUnescape(encoded)
	if err != nil || strings.TrimSpace(decoded) == "" {
		return "", status.Error(codes.InvalidArgument, "conversation_id is invalid")
	}
	return decoded, nil
}

func memberRoleFromString(value string) (conversationv1.MemberRole, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "MEMBER_ROLE_")
	switch normalized {
	case "":
		return conversationv1.MemberRole_MEMBER_ROLE_UNSPECIFIED, nil
	case "OWNER":
		return conversationv1.MemberRole_MEMBER_ROLE_OWNER, nil
	case "ADMIN":
		return conversationv1.MemberRole_MEMBER_ROLE_ADMIN, nil
	case "MEMBER":
		return conversationv1.MemberRole_MEMBER_ROLE_MEMBER, nil
	default:
		return conversationv1.MemberRole_MEMBER_ROLE_UNSPECIFIED, status.Error(codes.InvalidArgument, "member role is invalid")
	}
}

func requiredIdempotencyKey(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	return trimmed, nil
}

func directConversationID(tenantID, currentUserID, peerUserID string) string {
	users := []string{strings.TrimSpace(currentUserID), strings.TrimSpace(peerUserID)}
	sort.Strings(users)
	sum := sha256.Sum256([]byte(strings.TrimSpace(tenantID) + "\x1f" + users[0] + "\x1f" + users[1]))
	return "direct-" + hex.EncodeToString(sum[:])[:32]
}

func int32Query(query url.Values, keys ...string) (int32, error) {
	value := firstQuery(query, keys...)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 {
		return 0, status.Error(codes.InvalidArgument, keys[0]+" must be a non-negative int32")
	}
	return int32(parsed), nil
}

func int64Query(query url.Values, keys ...string) (int64, error) {
	value := firstQuery(query, keys...)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, status.Error(codes.InvalidArgument, keys[0]+" must be a non-negative int64")
	}
	return parsed, nil
}

func boolQuery(query url.Values, keys ...string) bool {
	value := strings.ToLower(firstQuery(query, keys...))
	return value == "1" || value == "true" || value == "yes" || value == "y" || value == "on"
}

func firstQuery(query url.Values, keys ...string) string {
	for _, key := range keys {
		if values, ok := query[key]; ok && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func contactRequestDirectionQuery(value string) (contactsv1.ContactRequestListDirection, error) {
	switch normalizeContactEnumQuery(value) {
	case "":
		return contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_UNSPECIFIED, nil
	case "INCOMING", "CONTACT_REQUEST_LIST_DIRECTION_INCOMING":
		return contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING, nil
	case "OUTGOING", "CONTACT_REQUEST_LIST_DIRECTION_OUTGOING":
		return contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_OUTGOING, nil
	default:
		return contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_UNSPECIFIED,
			status.Error(codes.InvalidArgument, "direction is invalid")
	}
}

func contactRequestStatusQuery(value string) (contactsv1.ContactRequestStatus, error) {
	switch normalizeContactEnumQuery(value) {
	case "":
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_UNSPECIFIED, nil
	case "PENDING", "CONTACT_REQUEST_STATUS_PENDING":
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING, nil
	case "ACCEPTED", "CONTACT_REQUEST_STATUS_ACCEPTED":
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_ACCEPTED, nil
	case "DECLINED", "CONTACT_REQUEST_STATUS_DECLINED":
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_DECLINED, nil
	case "CANCELED", "CONTACT_REQUEST_STATUS_CANCELED":
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_CANCELED, nil
	case "EXPIRED", "CONTACT_REQUEST_STATUS_EXPIRED":
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_EXPIRED, nil
	case "REVIEW_REQUIRED", "CONTACT_REQUEST_STATUS_REVIEW_REQUIRED":
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_REVIEW_REQUIRED, nil
	default:
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_UNSPECIFIED,
			status.Error(codes.InvalidArgument, "status is invalid")
	}
}

func contactRequestSourceTypeQuery(value string) (contactsv1.ContactRequestSourceType, error) {
	switch normalizeContactEnumQuery(value) {
	case "":
		return contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_UNSPECIFIED, nil
	case "DIRECT", "CONTACT_REQUEST_SOURCE_TYPE_DIRECT":
		return contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_DIRECT, nil
	case "SEARCH", "CONTACT_REQUEST_SOURCE_TYPE_SEARCH":
		return contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_SEARCH, nil
	case "GROUP", "CONTACT_REQUEST_SOURCE_TYPE_GROUP":
		return contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_GROUP, nil
	case "INVITE_LINK", "CONTACT_REQUEST_SOURCE_TYPE_INVITE_LINK":
		return contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_INVITE_LINK, nil
	case "QR_CODE", "CONTACT_REQUEST_SOURCE_TYPE_QR_CODE":
		return contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_QR_CODE, nil
	case "IMPORT", "CONTACT_REQUEST_SOURCE_TYPE_IMPORT":
		return contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_IMPORT, nil
	default:
		return contactsv1.ContactRequestSourceType_CONTACT_REQUEST_SOURCE_TYPE_UNSPECIFIED,
			status.Error(codes.InvalidArgument, "source_type_filter is invalid")
	}
}

func contactRequestRiskLevelQuery(value string) (contactsv1.ContactRequestRiskLevel, error) {
	switch normalizeContactEnumQuery(value) {
	case "":
		return contactsv1.ContactRequestRiskLevel_CONTACT_REQUEST_RISK_LEVEL_UNSPECIFIED, nil
	case "LOW", "CONTACT_REQUEST_RISK_LEVEL_LOW":
		return contactsv1.ContactRequestRiskLevel_CONTACT_REQUEST_RISK_LEVEL_LOW, nil
	case "MEDIUM", "CONTACT_REQUEST_RISK_LEVEL_MEDIUM":
		return contactsv1.ContactRequestRiskLevel_CONTACT_REQUEST_RISK_LEVEL_MEDIUM, nil
	case "HIGH", "CONTACT_REQUEST_RISK_LEVEL_HIGH":
		return contactsv1.ContactRequestRiskLevel_CONTACT_REQUEST_RISK_LEVEL_HIGH, nil
	default:
		return contactsv1.ContactRequestRiskLevel_CONTACT_REQUEST_RISK_LEVEL_UNSPECIFIED,
			status.Error(codes.InvalidArgument, "risk_level_filter is invalid")
	}
}

func normalizeContactEnumQuery(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func publicAuthError(err error) error {
	switch {
	case errors.Is(err, gatewayauth.ErrAuthExpired):
		return status.Error(codes.Unauthenticated, "gateway token expired")
	default:
		return status.Error(codes.Unauthenticated, "gateway auth failed")
	}
}

func writeError(response http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		st = status.New(codes.Internal, "internal error")
	}
	writeJSON(response, httpStatus(st.Code()), errorPayload{
		Error: publicError{
			Code:    st.Code().String(),
			Message: st.Message(),
		},
	})
}

func writeJSON(response http.ResponseWriter, statusCode int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(statusCode)
	_ = json.NewEncoder(response).Encode(payload)
}

func httpStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists, codes.Aborted:
		return http.StatusConflict
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
