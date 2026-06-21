package grpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	gatewaytypes "github.com/qsyy0921/IM/services/api-gateway/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	metadataTenantID    = "x-nexusim-tenant-id"
	metadataUserID      = "x-nexusim-user-id"
	metadataDeviceID    = "x-nexusim-device-id"
	metadataSessionID   = "x-nexusim-session-id"
	metadataTraceID     = "x-nexusim-trace-id"
	metadataRequestID   = "x-nexusim-request-id"
	metadataToken       = "x-nexusim-gateway-token"
	metadataTraceparent = "traceparent"
)

type Authenticator interface {
	Authenticate(*http.Request) (gatewayauth.AuthContext, error)
}

type Server struct {
	gatewayv1.UnimplementedGatewayServiceServer
	contactsv1.UnimplementedContactsServiceServer
	conversationv1.UnimplementedConversationServiceServer
	messagev1.UnimplementedMessageServiceServer
	deliveryv1.UnimplementedDeliveryServiceServer
	receiptv1.UnimplementedReceiptServiceServer

	auth         Authenticator
	contacts     contactsv1.ContactsServiceClient
	conversation conversationv1.ConversationServiceClient
	identity     identityv1.IdentityServiceClient
	message      messagev1.MessageServiceClient
	delivery     deliveryv1.DeliveryServiceClient
	receipt      receiptv1.ReceiptServiceClient
	newTraceID   func() string
	newRequestID func() string
}

type Config struct {
	Authenticator Authenticator
	Contacts      contactsv1.ContactsServiceClient
	Conversation  conversationv1.ConversationServiceClient
	Identity      identityv1.IdentityServiceClient
	Message       messagev1.MessageServiceClient
	Delivery      deliveryv1.DeliveryServiceClient
	Receipt       receiptv1.ReceiptServiceClient
	NewTraceID    func() string
	NewRequestID  func() string
}

type RegisterConfig struct {
	RegisterLegacyDescriptors bool
}

func NewServer(config Config) *Server {
	server := &Server{
		auth:         config.Authenticator,
		contacts:     config.Contacts,
		conversation: config.Conversation,
		identity:     config.Identity,
		message:      config.Message,
		delivery:     config.Delivery,
		receipt:      config.Receipt,
	}
	if config.NewTraceID != nil {
		server.newTraceID = config.NewTraceID
	} else {
		server.newTraceID = func() string { return newCorrelationID("trace") }
	}
	if config.NewRequestID != nil {
		server.newRequestID = config.NewRequestID
	} else {
		server.newRequestID = func() string { return newCorrelationID("request") }
	}
	return server
}

func Register(server grpcgo.ServiceRegistrar, gateway *Server) {
	RegisterWithConfig(server, gateway, RegisterConfig{})
}

func RegisterWithConfig(server grpcgo.ServiceRegistrar, gateway *Server, config RegisterConfig) {
	gatewayv1.RegisterGatewayServiceServer(server, gateway)
	if !config.RegisterLegacyDescriptors {
		return
	}
	contactsv1.RegisterContactsServiceServer(server, gateway)
	conversationv1.RegisterConversationServiceServer(server, gateway)
	messagev1.RegisterMessageServiceServer(server, gateway)
	deliveryv1.RegisterDeliveryServiceServer(server, gateway)
	receiptv1.RegisterReceiptServiceServer(server, gateway)
}

func (server *Server) GetSendContext(context.Context, *conversationv1.GetSendContextRequest) (*conversationv1.GetSendContextResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetSendContext is service-internal")
}

func (server *Server) RegisterUser(ctx context.Context, request *identityv1.RegisterUserRequest) (*identityv1.RegisterUserResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.RegisterUserRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.RegisterUser(outgoing, cloned)
}

func (server *Server) Login(ctx context.Context, request *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.LoginRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.Login(outgoing, cloned)
}

func (server *Server) RefreshGatewayToken(ctx context.Context, request *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.RefreshGatewayTokenRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.RefreshGatewayToken(outgoing, cloned)
}

func (server *Server) IssueGatewayToken(ctx context.Context, request *identityv1.IssueGatewayTokenRequest) (*identityv1.IssueGatewayTokenResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.IssueGatewayTokenRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.IssueGatewayToken(outgoing, cloned)
}

func (server *Server) RevokeSession(ctx context.Context, request *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error) {
	admin := request.GetAdminContext()
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, admin.GetTraceId(), admin.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.RevokeSessionRequest)
	if cloned.AdminContext == nil {
		cloned.AdminContext = &identityv1.AdminContext{}
	}
	cloned.AdminContext.TraceId = traceID
	cloned.AdminContext.RequestId = requestID
	return server.identity.RevokeSession(outgoing, cloned)
}

func (server *Server) RequestVerificationChallenge(ctx context.Context, request *identityv1.RequestVerificationChallengeRequest) (*identityv1.RequestVerificationChallengeResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.RequestVerificationChallengeRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.RequestVerificationChallenge(outgoing, cloned)
}

func (server *Server) ConfirmVerificationChallenge(ctx context.Context, request *identityv1.ConfirmVerificationChallengeRequest) (*identityv1.ConfirmVerificationChallengeResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.ConfirmVerificationChallengeRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.ConfirmVerificationChallenge(outgoing, cloned)
}

func (server *Server) RequestPasswordReset(ctx context.Context, request *identityv1.RequestPasswordResetRequest) (*identityv1.RequestPasswordResetResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.RequestPasswordResetRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.RequestPasswordReset(outgoing, cloned)
}

func (server *Server) ConfirmPasswordReset(ctx context.Context, request *identityv1.ConfirmPasswordResetRequest) (*identityv1.ConfirmPasswordResetResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.ConfirmPasswordResetRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.ConfirmPasswordReset(outgoing, cloned)
}

func (server *Server) BeginMFAEnrollment(ctx context.Context, request *identityv1.BeginMFAEnrollmentRequest) (*identityv1.BeginMFAEnrollmentResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.BeginMFAEnrollmentRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.BeginMFAEnrollment(outgoing, cloned)
}

func (server *Server) ConfirmMFAEnrollment(ctx context.Context, request *identityv1.ConfirmMFAEnrollmentRequest) (*identityv1.ConfirmMFAEnrollmentResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.ConfirmMFAEnrollmentRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.ConfirmMFAEnrollment(outgoing, cloned)
}

func (server *Server) DisableMFAFactor(ctx context.Context, request *identityv1.DisableMFAFactorRequest) (*identityv1.DisableMFAFactorResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.DisableMFAFactorRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.DisableMFAFactor(outgoing, cloned)
}

func (server *Server) RegenerateMFARecoveryCodes(ctx context.Context, request *identityv1.RegenerateMFARecoveryCodesRequest) (*identityv1.RegenerateMFARecoveryCodesResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.RegenerateMFARecoveryCodesRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.RegenerateMFARecoveryCodes(outgoing, cloned)
}

func (server *Server) RevokeMFARecoveryCodes(ctx context.Context, request *identityv1.RevokeMFARecoveryCodesRequest) (*identityv1.RevokeMFARecoveryCodesResponse, error) {
	outgoing, traceID, requestID := server.publicIdentityContext(ctx, request.GetTraceId(), request.GetRequestId())
	cloned := proto.Clone(request).(*identityv1.RevokeMFARecoveryCodesRequest)
	cloned.TraceId = traceID
	cloned.RequestId = requestID
	return server.identity.RevokeMFARecoveryCodes(outgoing, cloned)
}

func (server *Server) CreateMemberChange(ctx context.Context, request *conversationv1.CreateMemberChangeRequest) (*conversationv1.CreateMemberChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*conversationv1.CreateMemberChangeRequest)
	cloned.AuthContext = conversationAuth(auth)
	return server.conversation.CreateMemberChange(outgoing, cloned)
}

func (server *Server) GetMemberChange(ctx context.Context, request *conversationv1.GetMemberChangeRequest) (*conversationv1.GetMemberChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*conversationv1.GetMemberChangeRequest)
	cloned.AuthContext = conversationAuth(auth)
	return server.conversation.GetMemberChange(outgoing, cloned)
}

func (server *Server) ListConversationMembers(ctx context.Context, request *conversationv1.ListConversationMembersRequest) (*conversationv1.ListConversationMembersResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*conversationv1.ListConversationMembersRequest)
	cloned.AuthContext = conversationAuth(auth)
	return server.conversation.ListConversationMembers(outgoing, cloned)
}

func (server *Server) TransferConversationOwner(ctx context.Context, request *conversationv1.TransferConversationOwnerRequest) (*conversationv1.TransferConversationOwnerResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*conversationv1.TransferConversationOwnerRequest)
	cloned.AuthContext = conversationAuth(auth)
	return server.conversation.TransferConversationOwner(outgoing, cloned)
}

func (server *Server) SendMessage(ctx context.Context, request *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*messagev1.SendMessageRequest)
	cloned.AuthContext = messageAuth(auth)
	return server.message.SendMessage(outgoing, cloned)
}

func (server *Server) EditMessage(ctx context.Context, request *messagev1.EditMessageRequest) (*messagev1.MessageChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*messagev1.EditMessageRequest)
	cloned.AuthContext = messageAuth(auth)
	return server.message.EditMessage(outgoing, cloned)
}

func (server *Server) RevokeMessage(ctx context.Context, request *messagev1.RevokeMessageRequest) (*messagev1.MessageChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*messagev1.RevokeMessageRequest)
	cloned.AuthContext = messageAuth(auth)
	return server.message.RevokeMessage(outgoing, cloned)
}

func (server *Server) DeleteMessage(ctx context.Context, request *messagev1.DeleteMessageRequest) (*messagev1.MessageChangeResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*messagev1.DeleteMessageRequest)
	cloned.AuthContext = messageAuth(auth)
	return server.message.DeleteMessage(outgoing, cloned)
}

func (server *Server) PullInbox(ctx context.Context, request *deliveryv1.PullInboxRequest) (*deliveryv1.PullInboxResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*deliveryv1.PullInboxRequest)
	cloned.AuthContext = deliveryAuth(auth)
	return server.delivery.PullInbox(outgoing, cloned)
}

func (server *Server) AckDelivery(ctx context.Context, request *deliveryv1.AckDeliveryRequest) (*deliveryv1.AckDeliveryResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*deliveryv1.AckDeliveryRequest)
	cloned.AuthContext = deliveryAuth(auth)
	return server.delivery.AckDelivery(outgoing, cloned)
}

func (server *Server) HideInboxItem(ctx context.Context, request *deliveryv1.HideInboxItemRequest) (*deliveryv1.HideInboxItemResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*deliveryv1.HideInboxItemRequest)
	cloned.AuthContext = deliveryAuth(auth)
	return server.delivery.HideInboxItem(outgoing, cloned)
}

func (server *Server) MarkRead(ctx context.Context, request *receiptv1.MarkReadRequest) (*receiptv1.MarkReadResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.MarkReadRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.MarkRead(outgoing, cloned)
}

func (server *Server) GetReceiptState(ctx context.Context, request *receiptv1.GetReceiptStateRequest) (*receiptv1.GetReceiptStateResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.GetReceiptStateRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.GetReceiptState(outgoing, cloned)
}

func (server *Server) ListReceiptStates(ctx context.Context, request *receiptv1.ListReceiptStatesRequest) (*receiptv1.ListReceiptStatesResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.ListReceiptStatesRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.ListReceiptStates(outgoing, cloned)
}

func (server *Server) ListConversations(ctx context.Context, request *receiptv1.ListConversationsRequest) (*receiptv1.ListConversationsResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.ListConversationsRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.ListConversations(outgoing, cloned)
}

func (server *Server) ArchiveConversation(ctx context.Context, request *receiptv1.ArchiveConversationRequest) (*receiptv1.ArchiveConversationResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.ArchiveConversationRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.ArchiveConversation(outgoing, cloned)
}

func (server *Server) PinConversation(ctx context.Context, request *receiptv1.PinConversationRequest) (*receiptv1.PinConversationResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.PinConversationRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.PinConversation(outgoing, cloned)
}

func (server *Server) MuteConversation(ctx context.Context, request *receiptv1.MuteConversationRequest) (*receiptv1.MuteConversationResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.MuteConversationRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.MuteConversation(outgoing, cloned)
}

func (server *Server) SetConversationTags(ctx context.Context, request *receiptv1.SetConversationTagsRequest) (*receiptv1.SetConversationTagsResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.SetConversationTagsRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.SetConversationTags(outgoing, cloned)
}

func (server *Server) SetConversationDraft(ctx context.Context, request *receiptv1.SetConversationDraftRequest) (*receiptv1.SetConversationDraftResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*receiptv1.SetConversationDraftRequest)
	cloned.AuthContext = receiptAuth(auth)
	return server.receipt.SetConversationDraft(outgoing, cloned)
}

func (server *Server) SendContactRequest(ctx context.Context, request *contactsv1.SendContactRequestRequest) (*contactsv1.SendContactRequestResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.SendContactRequestRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.SendContactRequest(outgoing, cloned)
}

func (server *Server) GetContactPrivacy(ctx context.Context, request *contactsv1.GetContactPrivacyRequest) (*contactsv1.GetContactPrivacyResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.GetContactPrivacyRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.GetContactPrivacy(outgoing, cloned)
}

func (server *Server) SetContactPrivacy(ctx context.Context, request *contactsv1.SetContactPrivacyRequest) (*contactsv1.SetContactPrivacyResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.SetContactPrivacyRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.SetContactPrivacy(outgoing, cloned)
}

func (server *Server) RespondContactRequest(ctx context.Context, request *contactsv1.RespondContactRequestRequest) (*contactsv1.RespondContactRequestResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.RespondContactRequestRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.RespondContactRequest(outgoing, cloned)
}

func (server *Server) CancelContactRequest(ctx context.Context, request *contactsv1.CancelContactRequestRequest) (*contactsv1.CancelContactRequestResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.CancelContactRequestRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.CancelContactRequest(outgoing, cloned)
}

func (server *Server) ListContactRequests(ctx context.Context, request *contactsv1.ListContactRequestsRequest) (*contactsv1.ListContactRequestsResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.ListContactRequestsRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.ListContactRequests(outgoing, cloned)
}

func (server *Server) ListContacts(ctx context.Context, request *contactsv1.ListContactsRequest) (*contactsv1.ListContactsResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.ListContactsRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.ListContacts(outgoing, cloned)
}

func (server *Server) GetContactState(ctx context.Context, request *contactsv1.GetContactStateRequest) (*contactsv1.GetContactStateResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.GetContactStateRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.GetContactState(outgoing, cloned)
}

func (server *Server) DeleteContact(ctx context.Context, request *contactsv1.DeleteContactRequest) (*contactsv1.DeleteContactResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.DeleteContactRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.DeleteContact(outgoing, cloned)
}

func (server *Server) BlockContact(ctx context.Context, request *contactsv1.BlockContactRequest) (*contactsv1.BlockContactResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.BlockContactRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.BlockContact(outgoing, cloned)
}

func (server *Server) UnblockContact(ctx context.Context, request *contactsv1.UnblockContactRequest) (*contactsv1.UnblockContactResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.UnblockContactRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.UnblockContact(outgoing, cloned)
}

func (server *Server) UpdateContactRemark(ctx context.Context, request *contactsv1.UpdateContactRemarkRequest) (*contactsv1.UpdateContactRemarkResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.UpdateContactRemarkRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.UpdateContactRemark(outgoing, cloned)
}

func (server *Server) UpdateContactGroup(ctx context.Context, request *contactsv1.UpdateContactGroupRequest) (*contactsv1.UpdateContactGroupResponse, error) {
	auth, outgoing, err := server.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	cloned := proto.Clone(request).(*contactsv1.UpdateContactGroupRequest)
	cloned.AuthContext = contactsAuth(auth)
	return server.contacts.UpdateContactGroup(outgoing, cloned)
}

func (server *Server) authenticate(ctx context.Context) (gatewayauth.AuthContext, context.Context, error) {
	if server.auth == nil {
		return gatewayauth.AuthContext{}, nil, status.Error(codes.Internal, "gateway auth is not configured")
	}
	request, requestID := authRequestFromMetadata(ctx)
	auth, err := server.auth.Authenticate(request)
	if err != nil {
		return gatewayauth.AuthContext{}, nil, publicAuthError(err)
	}
	if auth.TenantID == "" || auth.UserID == "" || auth.DeviceID == "" {
		return gatewayauth.AuthContext{}, nil, status.Error(codes.Unauthenticated, "gateway auth metadata is required")
	}
	if auth.RequestID == "" {
		auth.RequestID = requestID
	}
	if auth.TraceID == "" {
		auth.TraceID = firstIncomingMetadata(ctx, metadataTraceID)
	}
	if auth.TraceID == "" {
		auth.TraceID = traceIDFromTraceparent(firstIncomingMetadata(ctx, metadataTraceparent))
	}
	if auth.TraceID == "" && server.newTraceID != nil {
		auth.TraceID = strings.TrimSpace(server.newTraceID())
	}
	if auth.RequestID == "" && server.newRequestID != nil {
		auth.RequestID = strings.TrimSpace(server.newRequestID())
	}
	if auth.TraceID != "" || auth.RequestID != "" {
		gatewaytypes.PublishCorrelation(ctx, auth.TraceID, auth.RequestID)
		_ = grpcgo.SetHeader(ctx, responseCorrelationMetadata(auth))
	}
	return auth, outgoingVerifiedContext(ctx, auth), nil
}

func (server *Server) publicIdentityContext(ctx context.Context, traceID string, requestID string) (context.Context, string, string) {
	traceID = strings.TrimSpace(traceID)
	requestID = strings.TrimSpace(requestID)
	if traceID == "" {
		traceID = firstIncomingMetadata(ctx, metadataTraceID)
	}
	if traceID == "" {
		traceID = traceIDFromTraceparent(firstIncomingMetadata(ctx, metadataTraceparent))
	}
	if traceID == "" && server.newTraceID != nil {
		traceID = strings.TrimSpace(server.newTraceID())
	}
	if requestID == "" {
		requestID = firstIncomingMetadata(ctx, metadataRequestID)
	}
	if requestID == "" && server.newRequestID != nil {
		requestID = strings.TrimSpace(server.newRequestID())
	}
	if traceID != "" || requestID != "" {
		gatewaytypes.PublishCorrelation(ctx, traceID, requestID)
		_ = grpcgo.SetHeader(ctx, responseCorrelationMetadata(gatewayauth.AuthContext{
			TraceID:   traceID,
			RequestID: requestID,
		}))
	}
	return outgoingCorrelationContext(ctx, traceID, requestID), traceID, requestID
}

func responseCorrelationMetadata(auth gatewayauth.AuthContext) metadata.MD {
	pairs := make([]string, 0, 4)
	if auth.TraceID != "" {
		pairs = append(pairs, metadataTraceID, auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, metadataRequestID, auth.RequestID)
	}
	return metadata.Pairs(pairs...)
}

func outgoingCorrelationContext(ctx context.Context, traceID string, requestID string) context.Context {
	pairs := make([]string, 0, 4)
	if traceID != "" {
		pairs = append(pairs, metadataTraceID, traceID)
	}
	if requestID != "" {
		pairs = append(pairs, metadataRequestID, requestID)
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func authRequestFromMetadata(ctx context.Context) (*http.Request, string) {
	query := url.Values{}
	for _, pair := range []struct {
		metadata string
		query    string
	}{
		{metadataToken, "token"},
		{metadataTraceID, "trace_id"},
	} {
		if value := firstIncomingMetadata(ctx, pair.metadata); value != "" {
			query.Set(pair.query, value)
		}
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://api-gateway/auth?"+query.Encode(), nil)
	if authorization := firstIncomingMetadata(ctx, "authorization"); authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	return request, firstIncomingMetadata(ctx, metadataRequestID)
}

func outgoingVerifiedContext(ctx context.Context, auth gatewayauth.AuthContext) context.Context {
	pairs := []string{
		metadataTenantID, auth.TenantID,
		metadataUserID, auth.UserID,
		metadataDeviceID, auth.DeviceID,
	}
	if auth.SessionID != "" {
		pairs = append(pairs, metadataSessionID, auth.SessionID)
	}
	if auth.TraceID != "" {
		pairs = append(pairs, metadataTraceID, auth.TraceID)
	}
	if auth.RequestID != "" {
		pairs = append(pairs, metadataRequestID, auth.RequestID)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func firstIncomingMetadata(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func traceIDFromTraceparent(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 4 {
		return ""
	}
	version, traceID, spanID, flags := parts[0], parts[1], parts[2], parts[3]
	if len(version) != 2 || strings.EqualFold(version, "ff") || !isHex(version) {
		return ""
	}
	if len(traceID) != 32 || isAllZero(traceID) || !isHex(traceID) {
		return ""
	}
	if len(spanID) != 16 || isAllZero(spanID) || !isHex(spanID) {
		return ""
	}
	if len(flags) != 2 || !isHex(flags) {
		return ""
	}
	return strings.ToLower(traceID)
}

func isHex(value string) bool {
	for _, char := range value {
		switch {
		case char >= '0' && char <= '9':
		case char >= 'a' && char <= 'f':
		case char >= 'A' && char <= 'F':
		default:
			return false
		}
	}
	return value != ""
}

func isAllZero(value string) bool {
	for _, char := range value {
		if char != '0' {
			return false
		}
	}
	return value != ""
}

func newCorrelationID(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(raw[:])
	}
	return prefix + "_" + strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", "")
}

func publicAuthError(err error) error {
	switch {
	case errors.Is(err, gatewayauth.ErrAuthExpired):
		return status.Error(codes.Unauthenticated, "gateway token expired")
	case errors.Is(err, gatewayauth.ErrPermissionDenied), errors.Is(err, gatewayauth.ErrInvalidRequest):
		return status.Error(codes.Unauthenticated, "gateway auth failed")
	default:
		return status.Error(codes.Unauthenticated, "gateway auth failed")
	}
}

func contactsAuth(auth gatewayauth.AuthContext) *contactsv1.AuthContext {
	return &contactsv1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}

func conversationAuth(auth gatewayauth.AuthContext) *conversationv1.AuthContext {
	return &conversationv1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}

func messageAuth(auth gatewayauth.AuthContext) *messagev1.AuthContext {
	return &messagev1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}

func deliveryAuth(auth gatewayauth.AuthContext) *deliveryv1.AuthContext {
	return &deliveryv1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}

func receiptAuth(auth gatewayauth.AuthContext) *receiptv1.AuthContext {
	return &receiptv1.AuthContext{
		TenantId:  auth.TenantID,
		UserId:    auth.UserID,
		DeviceId:  auth.DeviceID,
		SessionId: auth.SessionID,
		TraceId:   auth.TraceID,
		RequestId: auth.RequestID,
	}
}
