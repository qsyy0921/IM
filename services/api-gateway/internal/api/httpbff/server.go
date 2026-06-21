package httpbff

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
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
	Login(ctx context.Context, request *identityv1.LoginRequest) (*identityv1.LoginResponse, error)
	RefreshGatewayToken(ctx context.Context, request *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error)
	SendMessage(ctx context.Context, request *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error)
	PullInbox(ctx context.Context, request *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error)
	AckDelivery(ctx context.Context, request *deliveryv1.AckDeliveryRequest) (*deliveryv1.AckDeliveryResponse, error)
	ListConversations(ctx context.Context, request *receiptv1.ListConversationsRequest) (*receiptv1.ListConversationsResponse, error)
	ListReceiptStates(ctx context.Context, request *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error)
	ListContacts(ctx context.Context, request *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (gatewayauth.AuthContext, error)
}

type Config struct {
	Gateway        Gateway
	Authenticator  Authenticator
	AllowedOrigins []string
}

type Server struct {
	gateway        Gateway
	authenticator  Authenticator
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

func NewServer(config Config) *Server {
	server := &Server{
		gateway:       config.Gateway,
		authenticator: config.Authenticator,
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
	if server.handleCORS(response, request) {
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
	case request.Method == http.MethodPost && path == "/api/auth/refresh":
		server.handleRefresh(response, request)
	case request.Method == http.MethodPost && path == "/api/auth/logout":
		writeError(response, status.Error(codes.Unimplemented, "logout is not implemented"))
	case request.Method == http.MethodGet && path == "/api/me":
		server.handleMe(response, request)
	case request.Method == http.MethodGet && path == "/api/conversations":
		server.handleListConversations(response, request)
	case request.Method == http.MethodGet && isConversationMessagesPath(request.URL.EscapedPath()):
		server.handleConversationMessages(response, request)
	case request.Method == http.MethodPost && path == "/api/messages/send":
		server.handleSendMessage(response, request)
	case request.Method == http.MethodPost && path == "/api/delivery/ack":
		server.handleAckDelivery(response, request)
	case request.Method == http.MethodGet && path == "/api/contacts":
		server.handleListContacts(response, request)
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
	output, err := server.requireGateway().Login(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleRefresh(response http.ResponseWriter, request *http.Request) {
	var input identityv1.RefreshGatewayTokenRequest
	if !server.decode(response, request, &input) {
		return
	}
	output, err := server.requireGateway().RefreshGatewayToken(contextFromRequest(request), &input)
	server.writeProtoOrError(response, output, err)
}

func (server *Server) handleMe(response http.ResponseWriter, request *http.Request) {
	if server.authenticator == nil {
		writeError(response, status.Error(codes.Internal, "gateway auth is not configured"))
		return
	}
	authRequest := request.Clone(request.Context())
	if authRequest.Header.Get("Authorization") == "" {
		if token := strings.TrimSpace(authRequest.Header.Get("X-NexusIM-Gateway-Token")); token != "" {
			authRequest.Header.Set("Authorization", "Bearer "+token)
		}
	}
	auth, err := server.authenticator.Authenticate(authRequest)
	if err != nil {
		writeError(response, publicAuthError(err))
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{
		"tenant_id":  auth.TenantID,
		"user_id":    auth.UserID,
		"device_id":  auth.DeviceID,
		"session_id": auth.SessionID,
	})
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
	if request.Body == nil {
		writeError(response, status.Error(codes.InvalidArgument, "request body is required"))
		return false
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil {
		writeError(response, status.Error(codes.InvalidArgument, "failed to read request body"))
		return false
	}
	if len(body) > maxBodyBytes {
		writeError(response, status.Error(codes.ResourceExhausted, "request body is too large"))
		return false
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		writeError(response, status.Error(codes.InvalidArgument, "request body is required"))
		return false
	}
	if err := server.unmarshal.Unmarshal(body, message); err != nil {
		writeError(response, status.Error(codes.InvalidArgument, "invalid json request"))
		return false
	}
	return true
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

func (server *Server) requireGateway() Gateway {
	if server.gateway != nil {
		return server.gateway
	}
	return missingGateway{}
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

func (missingGateway) Login(context.Context, *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) RefreshGatewayToken(context.Context, *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error) {
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
func (missingGateway) ListReceiptStates(context.Context, *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error) {
	return nil, status.Error(codes.Internal, "gateway is not configured")
}
func (missingGateway) ListContacts(context.Context, *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
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
