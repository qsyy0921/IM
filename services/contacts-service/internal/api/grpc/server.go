package grpc

import (
	"context"
	"errors"

	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SendContactRequestExecutor interface {
	Execute(context.Context, types.SendContactRequestCommand) (types.SendContactRequestResult, error)
}

type RespondContactRequestExecutor interface {
	Execute(context.Context, types.RespondContactRequestCommand) (types.RespondContactRequestResult, error)
}

type CancelContactRequestExecutor interface {
	Execute(context.Context, types.CancelContactRequestCommand) (types.CancelContactRequestResult, error)
}

type ListContactRequestsExecutor interface {
	Execute(context.Context, types.ListContactRequestsCommand) (types.ListContactRequestsResult, error)
}

type ListContactsExecutor interface {
	Execute(context.Context, types.ListContactsCommand) (types.ListContactsResult, error)
}

type GetContactStateExecutor interface {
	Execute(context.Context, types.GetContactStateCommand) (types.GetContactStateResult, error)
}

type DeleteContactExecutor interface {
	Execute(context.Context, types.DeleteContactCommand) (types.DeleteContactResult, error)
}

type BlockContactExecutor interface {
	Execute(context.Context, types.BlockContactCommand) (types.BlockContactResult, error)
}

type UnblockContactExecutor interface {
	Execute(context.Context, types.UnblockContactCommand) (types.UnblockContactResult, error)
}

type UpdateContactRemarkExecutor interface {
	Execute(context.Context, types.UpdateContactRemarkCommand) (types.UpdateContactRemarkResult, error)
}

type Server struct {
	contactsv1.UnimplementedContactsServiceServer
	sendContactRequest    SendContactRequestExecutor
	respondContactRequest RespondContactRequestExecutor
	cancelContactRequest  CancelContactRequestExecutor
	listContactRequests   ListContactRequestsExecutor
	listContacts          ListContactsExecutor
	getContactState       GetContactStateExecutor
	deleteContact         DeleteContactExecutor
	blockContact          BlockContactExecutor
	unblockContact        UnblockContactExecutor
	updateContactRemark   UpdateContactRemarkExecutor
}

func NewServer(
	sendContactRequest SendContactRequestExecutor,
	respondContactRequest RespondContactRequestExecutor,
	cancelContactRequest CancelContactRequestExecutor,
	listContactRequests ListContactRequestsExecutor,
	listContacts ListContactsExecutor,
	getContactState GetContactStateExecutor,
	deleteContact DeleteContactExecutor,
	blockContact BlockContactExecutor,
	unblockContact UnblockContactExecutor,
	updateContactRemark UpdateContactRemarkExecutor,
) *Server {
	return &Server{
		sendContactRequest:    sendContactRequest,
		respondContactRequest: respondContactRequest,
		cancelContactRequest:  cancelContactRequest,
		listContactRequests:   listContactRequests,
		listContacts:          listContacts,
		getContactState:       getContactState,
		deleteContact:         deleteContact,
		blockContact:          blockContact,
		unblockContact:        unblockContact,
		updateContactRemark:   updateContactRemark,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	contactsv1.RegisterContactsServiceServer(registrar, server)
}

func (s *Server) SendContactRequest(
	ctx context.Context,
	request *contactsv1.SendContactRequestRequest,
) (*contactsv1.SendContactRequestResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.sendContactRequest == nil {
		return nil, status.Error(codes.Unimplemented, "send contact request is not configured")
	}
	result, err := s.sendContactRequest.Execute(ctx, types.SendContactRequestCommand{
		AuthContext:    authFromProto(ctx, request.GetAuthContext()),
		TargetUserID:   types.UserID(request.GetTargetUserId()),
		IdempotencyKey: request.GetIdempotencyKey(),
		Message:        request.GetMessage(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &contactsv1.SendContactRequestResponse{
		RequestId:        result.RequestID,
		TenantId:         string(result.TenantID),
		SenderUserId:     string(result.SenderUserID),
		ReceiverUserId:   string(result.ReceiverUserID),
		Status:           requestStatusToProto(result.Status),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) RespondContactRequest(
	ctx context.Context,
	request *contactsv1.RespondContactRequestRequest,
) (*contactsv1.RespondContactRequestResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.respondContactRequest == nil {
		return nil, status.Error(codes.Unimplemented, "respond contact request is not configured")
	}
	result, err := s.respondContactRequest.Execute(ctx, types.RespondContactRequestCommand{
		AuthContext:    authFromProto(ctx, request.GetAuthContext()),
		RequestID:      request.GetRequestId(),
		Decision:       decisionFromProto(request.GetDecision()),
		IdempotencyKey: request.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &contactsv1.RespondContactRequestResponse{
		RequestId:        result.RequestID,
		TenantId:         string(result.TenantID),
		SenderUserId:     string(result.SenderUserID),
		ReceiverUserId:   string(result.ReceiverUserID),
		Status:           requestStatusToProto(result.Status),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) CancelContactRequest(
	ctx context.Context,
	request *contactsv1.CancelContactRequestRequest,
) (*contactsv1.CancelContactRequestResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.cancelContactRequest == nil {
		return nil, status.Error(codes.Unimplemented, "cancel contact request is not configured")
	}
	result, err := s.cancelContactRequest.Execute(ctx, types.CancelContactRequestCommand{
		AuthContext:    authFromProto(ctx, request.GetAuthContext()),
		RequestID:      request.GetRequestId(),
		IdempotencyKey: request.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &contactsv1.CancelContactRequestResponse{
		RequestId:        result.RequestID,
		TenantId:         string(result.TenantID),
		SenderUserId:     string(result.SenderUserID),
		ReceiverUserId:   string(result.ReceiverUserID),
		Status:           requestStatusToProto(result.Status),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) ListContactRequests(
	ctx context.Context,
	request *contactsv1.ListContactRequestsRequest,
) (*contactsv1.ListContactRequestsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.listContactRequests == nil {
		return nil, status.Error(codes.Unimplemented, "list contact requests is not configured")
	}
	result, err := s.listContactRequests.Execute(ctx, types.ListContactRequestsCommand{
		AuthContext: authFromProto(ctx, request.GetAuthContext()),
		Direction:   requestListDirectionFromProto(request.GetDirection()),
		Status:      requestStatusFromProto(request.GetStatus()),
		PageSize:    int(request.GetPageSize()),
		PageToken:   request.GetPageToken(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	requests := make([]*contactsv1.ContactRequestItem, 0, len(result.Requests))
	for _, item := range result.Requests {
		requests = append(requests, &contactsv1.ContactRequestItem{
			RequestId:       item.RequestID,
			SenderUserId:    string(item.SenderUserID),
			ReceiverUserId:  string(item.ReceiverUserID),
			Status:          requestStatusToProto(item.Status),
			Message:         item.Message,
			CreatedAtUnixMs: item.CreatedAtUnixMS,
			UpdatedAtUnixMs: item.UpdatedAtUnixMS,
			DecidedAtUnixMs: item.DecidedAtUnixMS,
		})
	}
	return &contactsv1.ListContactRequestsResponse{
		TenantId:      string(result.TenantID),
		UserId:        string(result.UserID),
		Direction:     requestListDirectionToProto(result.Direction),
		Status:        requestStatusToProto(result.Status),
		Requests:      requests,
		NextPageToken: result.NextPageToken,
	}, nil
}

func (s *Server) ListContacts(
	ctx context.Context,
	request *contactsv1.ListContactsRequest,
) (*contactsv1.ListContactsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.listContacts == nil {
		return nil, status.Error(codes.Unimplemented, "list contacts is not configured")
	}
	result, err := s.listContacts.Execute(ctx, types.ListContactsCommand{
		AuthContext: authFromProto(ctx, request.GetAuthContext()),
		PageSize:    int(request.GetPageSize()),
		PageToken:   request.GetPageToken(),
		Query:       request.GetQuery(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	contacts := make([]*contactsv1.ContactItem, 0, len(result.Contacts))
	for _, item := range result.Contacts {
		contacts = append(contacts, &contactsv1.ContactItem{
			ContactUserId:   string(item.ContactUserID),
			Status:          edgeStatusToProto(item.Status),
			Version:         item.Version,
			SourceRequestId: item.SourceRequestID,
			CreatedAtUnixMs: item.CreatedAtUnixMS,
			UpdatedAtUnixMs: item.UpdatedAtUnixMS,
			Remark:          item.Remark,
		})
	}
	return &contactsv1.ListContactsResponse{
		TenantId:      string(result.TenantID),
		OwnerUserId:   string(result.OwnerUserID),
		Contacts:      contacts,
		NextPageToken: result.NextPageToken,
	}, nil
}

func (s *Server) GetContactState(
	ctx context.Context,
	request *contactsv1.GetContactStateRequest,
) (*contactsv1.GetContactStateResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.getContactState == nil {
		return nil, status.Error(codes.Unimplemented, "get contact state is not configured")
	}
	result, err := s.getContactState.Execute(ctx, types.GetContactStateCommand{
		AuthContext: authFromProto(ctx, request.GetAuthContext()),
		OtherUserID: types.UserID(request.GetOtherUserId()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &contactsv1.GetContactStateResponse{
		TenantId:        string(result.TenantID),
		OwnerUserId:     string(result.OwnerUserID),
		ContactUserId:   string(result.ContactUserID),
		Status:          edgeStatusToProto(result.Status),
		SourceRequestId: result.SourceRequestID,
		Version:         result.Version,
		Remark:          result.Remark,
	}, nil
}

func (s *Server) DeleteContact(
	ctx context.Context,
	request *contactsv1.DeleteContactRequest,
) (*contactsv1.DeleteContactResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.deleteContact == nil {
		return nil, status.Error(codes.Unimplemented, "delete contact is not configured")
	}
	result, err := s.deleteContact.Execute(ctx, types.DeleteContactCommand{
		AuthContext:    authFromProto(ctx, request.GetAuthContext()),
		ContactUserID:  types.UserID(request.GetContactUserId()),
		IdempotencyKey: request.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &contactsv1.DeleteContactResponse{
		TenantId:         string(result.TenantID),
		OwnerUserId:      string(result.OwnerUserID),
		ContactUserId:    string(result.ContactUserID),
		Status:           edgeStatusToProto(result.Status),
		SourceRequestId:  result.SourceRequestID,
		Version:          result.Version,
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) BlockContact(
	ctx context.Context,
	request *contactsv1.BlockContactRequest,
) (*contactsv1.BlockContactResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.blockContact == nil {
		return nil, status.Error(codes.Unimplemented, "block contact is not configured")
	}
	result, err := s.blockContact.Execute(ctx, types.BlockContactCommand{
		AuthContext:    authFromProto(ctx, request.GetAuthContext()),
		ContactUserID:  types.UserID(request.GetContactUserId()),
		IdempotencyKey: request.GetIdempotencyKey(),
		Reason:         request.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &contactsv1.BlockContactResponse{
		TenantId:         string(result.TenantID),
		OwnerUserId:      string(result.OwnerUserID),
		ContactUserId:    string(result.ContactUserID),
		Status:           edgeStatusToProto(result.Status),
		SourceRequestId:  result.SourceRequestID,
		Version:          result.Version,
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) UnblockContact(
	ctx context.Context,
	request *contactsv1.UnblockContactRequest,
) (*contactsv1.UnblockContactResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.unblockContact == nil {
		return nil, status.Error(codes.Unimplemented, "unblock contact is not configured")
	}
	result, err := s.unblockContact.Execute(ctx, types.UnblockContactCommand{
		AuthContext:    authFromProto(ctx, request.GetAuthContext()),
		ContactUserID:  types.UserID(request.GetContactUserId()),
		IdempotencyKey: request.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &contactsv1.UnblockContactResponse{
		TenantId:         string(result.TenantID),
		OwnerUserId:      string(result.OwnerUserID),
		ContactUserId:    string(result.ContactUserID),
		Status:           edgeStatusToProto(result.Status),
		SourceRequestId:  result.SourceRequestID,
		Version:          result.Version,
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (s *Server) UpdateContactRemark(
	ctx context.Context,
	request *contactsv1.UpdateContactRemarkRequest,
) (*contactsv1.UpdateContactRemarkResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.updateContactRemark == nil {
		return nil, status.Error(codes.Unimplemented, "update contact remark is not configured")
	}
	result, err := s.updateContactRemark.Execute(ctx, types.UpdateContactRemarkCommand{
		AuthContext:    authFromProto(ctx, request.GetAuthContext()),
		ContactUserID:  types.UserID(request.GetContactUserId()),
		IdempotencyKey: request.GetIdempotencyKey(),
		Remark:         request.GetRemark(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &contactsv1.UpdateContactRemarkResponse{
		TenantId:         string(result.TenantID),
		OwnerUserId:      string(result.OwnerUserID),
		ContactUserId:    string(result.ContactUserID),
		Status:           edgeStatusToProto(result.Status),
		SourceRequestId:  result.SourceRequestID,
		Version:          result.Version,
		Remark:           result.Remark,
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func authFromProto(ctx context.Context, auth *contactsv1.AuthContext) types.AuthContext {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		if auth != nil {
			if verified.DeviceID == "" {
				verified.DeviceID = auth.GetDeviceId()
			}
			if verified.SessionID == "" {
				verified.SessionID = auth.GetSessionId()
			}
			if verified.TraceID == "" {
				verified.TraceID = auth.GetTraceId()
			}
			if verified.RequestID == "" {
				verified.RequestID = auth.GetRequestId()
			}
		}
		return verified
	}
	if auth == nil {
		return types.AuthContext{}
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  auth.GetDeviceId(),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}
}

func decisionFromProto(value contactsv1.ContactDecision) types.ContactDecision {
	switch value {
	case contactsv1.ContactDecision_CONTACT_DECISION_ACCEPT:
		return types.ContactDecisionAccept
	case contactsv1.ContactDecision_CONTACT_DECISION_DECLINE:
		return types.ContactDecisionDecline
	default:
		return ""
	}
}

func requestStatusFromProto(value contactsv1.ContactRequestStatus) types.ContactRequestStatus {
	switch value {
	case contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING:
		return types.ContactRequestStatusPending
	case contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_ACCEPTED:
		return types.ContactRequestStatusAccepted
	case contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_DECLINED:
		return types.ContactRequestStatusDeclined
	case contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_CANCELED:
		return types.ContactRequestStatusCanceled
	case contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_EXPIRED:
		return types.ContactRequestStatusExpired
	default:
		return ""
	}
}

func requestStatusToProto(value types.ContactRequestStatus) contactsv1.ContactRequestStatus {
	switch value {
	case types.ContactRequestStatusPending:
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_PENDING
	case types.ContactRequestStatusAccepted:
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_ACCEPTED
	case types.ContactRequestStatusDeclined:
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_DECLINED
	case types.ContactRequestStatusCanceled:
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_CANCELED
	case types.ContactRequestStatusExpired:
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_EXPIRED
	default:
		return contactsv1.ContactRequestStatus_CONTACT_REQUEST_STATUS_UNSPECIFIED
	}
}

func requestListDirectionFromProto(value contactsv1.ContactRequestListDirection) types.ContactRequestListDirection {
	switch value {
	case contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING:
		return types.ContactRequestListDirectionIncoming
	case contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_OUTGOING:
		return types.ContactRequestListDirectionOutgoing
	default:
		return ""
	}
}

func requestListDirectionToProto(value types.ContactRequestListDirection) contactsv1.ContactRequestListDirection {
	switch value {
	case types.ContactRequestListDirectionIncoming:
		return contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_INCOMING
	case types.ContactRequestListDirectionOutgoing:
		return contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_OUTGOING
	default:
		return contactsv1.ContactRequestListDirection_CONTACT_REQUEST_LIST_DIRECTION_UNSPECIFIED
	}
}

func edgeStatusToProto(value types.ContactEdgeStatus) contactsv1.ContactEdgeStatus {
	switch value {
	case types.ContactEdgeStatusActive:
		return contactsv1.ContactEdgeStatus_CONTACT_EDGE_STATUS_ACTIVE
	case types.ContactEdgeStatusDeleted:
		return contactsv1.ContactEdgeStatus_CONTACT_EDGE_STATUS_DELETED
	case types.ContactEdgeStatusBlocked:
		return contactsv1.ContactEdgeStatus_CONTACT_EDGE_STATUS_BLOCKED
	default:
		return contactsv1.ContactEdgeStatus_CONTACT_EDGE_STATUS_UNSPECIFIED
	}
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid contact request")
	case errors.Is(err, types.ErrContactRequestNotFound):
		return status.Error(codes.NotFound, "contact request not found")
	case errors.Is(err, types.ErrContactNotFound):
		return status.Error(codes.NotFound, "contact not found")
	case errors.Is(err, types.ErrContactAlreadyExists):
		return status.Error(codes.AlreadyExists, "contact already exists")
	case errors.Is(err, types.ErrContactRequestConflict):
		return status.Error(codes.FailedPrecondition, "contact request conflict")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed), errors.Is(err, types.ErrOutboxWriteFailed):
		return status.Error(codes.Unavailable, "contacts storage unavailable")
	case errors.Is(err, types.ErrServiceOverloaded):
		return status.Error(codes.Unavailable, "service overloaded")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
