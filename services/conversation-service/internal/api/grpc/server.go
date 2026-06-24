package grpc

import (
	"context"
	"errors"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GetSendContextExecutor interface {
	Execute(context.Context, types.GetSendContextCommand) (types.ConversationSendContext, error)
}

type CreateConversationExecutor interface {
	Execute(context.Context, types.CreateConversationCommand) (types.CreateConversationResult, error)
}

type CreateMemberChangeExecutor interface {
	Execute(context.Context, types.CreateMemberChangeCommand) (types.MemberChangeResult, error)
}

type TransferConversationOwnerExecutor interface {
	Execute(context.Context, types.TransferConversationOwnerCommand) (types.TransferConversationOwnerResult, error)
}

type GetMemberChangeExecutor interface {
	Execute(context.Context, types.GetMemberChangeCommand) (types.MemberChangeDetail, error)
}

type ListConversationMembersExecutor interface {
	Execute(context.Context, types.ListConversationMembersCommand) (types.ListConversationMembersResult, error)
}

type GetConversationProfileExecutor interface {
	Execute(context.Context, types.GetConversationProfileCommand) (types.ConversationProfileResult, error)
}

type UpdateConversationProfileExecutor interface {
	Execute(context.Context, types.UpdateConversationProfileCommand) (types.ConversationProfileResult, error)
}

type CreateConversationNoteExecutor interface {
	Execute(context.Context, types.CreateConversationNoteCommand) (types.ConversationNoteResult, error)
}

type Option func(*Server)

type Server struct {
	conversationv1.UnimplementedConversationServiceServer
	getSendContext         GetSendContextExecutor
	createConversation     CreateConversationExecutor
	createMemberChange     CreateMemberChangeExecutor
	transferOwner          TransferConversationOwnerExecutor
	getMemberChange        GetMemberChangeExecutor
	listConversationMember ListConversationMembersExecutor
	getConversationProfile GetConversationProfileExecutor
	updateProfile          UpdateConversationProfileExecutor
	createNote             CreateConversationNoteExecutor
}

func NewServer(getSendContext GetSendContextExecutor, opts ...Option) *Server {
	server := &Server{getSendContext: getSendContext}
	for _, opt := range opts {
		opt(server)
	}
	return server
}

func WithCreateMemberChange(executor CreateMemberChangeExecutor) Option {
	return func(server *Server) {
		server.createMemberChange = executor
	}
}

func WithCreateConversation(executor CreateConversationExecutor) Option {
	return func(server *Server) {
		server.createConversation = executor
	}
}

func WithTransferConversationOwner(executor TransferConversationOwnerExecutor) Option {
	return func(server *Server) {
		server.transferOwner = executor
	}
}

func WithGetMemberChange(executor GetMemberChangeExecutor) Option {
	return func(server *Server) {
		server.getMemberChange = executor
	}
}

func WithListConversationMembers(executor ListConversationMembersExecutor) Option {
	return func(server *Server) {
		server.listConversationMember = executor
	}
}

func WithGetConversationProfile(executor GetConversationProfileExecutor) Option {
	return func(server *Server) {
		server.getConversationProfile = executor
	}
}

func WithUpdateConversationProfile(executor UpdateConversationProfileExecutor) Option {
	return func(server *Server) {
		server.updateProfile = executor
	}
}

func WithCreateConversationNote(executor CreateConversationNoteExecutor) Option {
	return func(server *Server) {
		server.createNote = executor
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	conversationv1.RegisterConversationServiceServer(registrar, server)
}

func (s *Server) GetSendContext(
	ctx context.Context,
	request *conversationv1.GetSendContextRequest,
) (*conversationv1.GetSendContextResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	result, err := s.getSendContext.Execute(ctx, types.GetSendContextCommand{
		TenantID:       types.TenantID(request.GetTenantId()),
		ConversationID: types.ConversationID(request.GetConversationId()),
		UserID:         types.UserID(request.GetUserId()),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.GetSendContextResponse{
		TenantId:            string(result.TenantID),
		ConversationId:      string(result.ConversationID),
		MemberVersion:       result.MemberVersion,
		PermissionVersion:   result.PermissionVersion,
		ConversationMode:    toProtoConversationMode(result.ConversationMode),
		FanoutMode:          toProtoFanoutMode(result.FanoutMode),
		FanoutPolicyVersion: result.FanoutPolicyVersion,
		CurrentSeqShard:     result.CurrentSeqShard,
		DirectPeerUserId:    string(result.DirectPeerUserID),
	}, nil
}

func (s *Server) CreateConversation(
	ctx context.Context,
	request *conversationv1.CreateConversationRequest,
) (*conversationv1.CreateConversationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.createConversation == nil {
		return nil, status.Error(codes.Unimplemented, "create conversation is not configured")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := s.createConversation.Execute(ctx, types.CreateConversationCommand{
		AuthContext:      auth,
		ConversationID:   types.ConversationID(request.GetConversationId()),
		ConversationType: fromProtoConversationType(request.GetConversationType()),
		DirectPeerUserID: types.UserID(request.GetDirectPeerUserId()),
		IdempotencyKey:   request.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.CreateConversationResponse{
		TenantId:          string(result.TenantID),
		ConversationId:    string(result.ConversationID),
		ConversationType:  toProtoConversationType(result.ConversationType),
		DirectPeerUserId:  string(result.DirectPeerUserID),
		BoundarySeq:       result.BoundarySeq,
		MemberVersion:     result.MemberVersion,
		PermissionVersion: result.PermissionVersion,
		IdempotentReplay:  result.IdempotentReplay,
	}, nil
}

func (s *Server) CreateMemberChange(
	ctx context.Context,
	request *conversationv1.CreateMemberChangeRequest,
) (*conversationv1.CreateMemberChangeResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.createMemberChange == nil {
		return nil, status.Error(codes.Unimplemented, "create member change is not configured")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := s.createMemberChange.Execute(ctx, types.CreateMemberChangeCommand{
		AuthContext:           auth,
		ConversationID:        types.ConversationID(request.GetConversationId()),
		TargetUserID:          types.UserID(request.GetTargetUserId()),
		ChangeType:            fromProtoMemberChangeType(request.GetChangeType()),
		TargetRole:            fromProtoMemberRole(request.GetTargetRole()),
		ExpectedMemberVersion: request.GetExpectedMemberVersion(),
		IdempotencyKey:        request.GetIdempotencyKey(),
		ConflictPolicy:        fromProtoConflictPolicy(request.GetConflictPolicy()),
		Reason:                request.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.CreateMemberChangeResponse{
		ChangeId:          string(result.ChangeID),
		TenantId:          string(result.TenantID),
		ConversationId:    string(result.ConversationID),
		TargetUserId:      string(result.TargetUserID),
		ChangeType:        toProtoMemberChangeType(result.ChangeType),
		Status:            toProtoMemberChangeStatus(result.Status),
		BoundarySeq:       result.BoundarySeq,
		MemberVersion:     result.MemberVersion,
		PermissionVersion: result.PermissionVersion,
		IdempotentReplay:  result.IdempotentReplay,
	}, nil
}

func (s *Server) TransferConversationOwner(
	ctx context.Context,
	request *conversationv1.TransferConversationOwnerRequest,
) (*conversationv1.TransferConversationOwnerResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.transferOwner == nil {
		return nil, status.Error(codes.Unimplemented, "transfer conversation owner is not configured")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := s.transferOwner.Execute(ctx, types.TransferConversationOwnerCommand{
		AuthContext:           auth,
		ConversationID:        types.ConversationID(request.GetConversationId()),
		NewOwnerUserID:        types.UserID(request.GetNewOwnerUserId()),
		ExpectedMemberVersion: request.GetExpectedMemberVersion(),
		IdempotencyKey:        request.GetIdempotencyKey(),
		Reason:                request.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.TransferConversationOwnerResponse{
		ChangeId:            string(result.ChangeID),
		TenantId:            string(result.TenantID),
		ConversationId:      string(result.ConversationID),
		PreviousOwnerUserId: string(result.PreviousOwnerUserID),
		NewOwnerUserId:      string(result.NewOwnerUserID),
		Status:              toProtoMemberChangeStatus(result.Status),
		BoundarySeq:         result.BoundarySeq,
		MemberVersion:       result.MemberVersion,
		PermissionVersion:   result.PermissionVersion,
		IdempotentReplay:    result.IdempotentReplay,
	}, nil
}

func (s *Server) GetMemberChange(
	ctx context.Context,
	request *conversationv1.GetMemberChangeRequest,
) (*conversationv1.GetMemberChangeResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.getMemberChange == nil {
		return nil, status.Error(codes.Unimplemented, "get member change is not configured")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := s.getMemberChange.Execute(ctx, types.GetMemberChangeCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(request.GetConversationId()),
		ChangeID:       types.ChangeID(request.GetChangeId()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.GetMemberChangeResponse{
		ChangeId:          string(result.ChangeID),
		TenantId:          string(result.TenantID),
		ConversationId:    string(result.ConversationID),
		TargetUserId:      string(result.TargetUserID),
		OperatorUserId:    string(result.OperatorUserID),
		ChangeType:        toProtoMemberChangeType(result.ChangeType),
		Status:            toProtoMemberChangeStatus(result.Status),
		BoundarySeq:       result.BoundarySeq,
		MemberVersion:     result.MemberVersion,
		PermissionVersion: result.PermissionVersion,
		OldRole:           toProtoMemberRole(result.OldRole),
		NewRole:           toProtoMemberRole(result.NewRole),
		Reason:            result.Reason,
		LastError:         result.LastError,
	}, nil
}

func (s *Server) ListConversationMembers(
	ctx context.Context,
	request *conversationv1.ListConversationMembersRequest,
) (*conversationv1.ListConversationMembersResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.listConversationMember == nil {
		return nil, status.Error(codes.Unimplemented, "list conversation members is not configured")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := s.listConversationMember.Execute(ctx, types.ListConversationMembersCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(request.GetConversationId()),
		PageSize:       int(request.GetPageSize()),
		PageToken:      request.GetPageToken(),
		RoleFilter:     fromProtoMemberRole(request.GetRoleFilter()),
		RoleFilters:    fromProtoMemberRoles(request.GetRoleFilters()),
		Sort:           fromProtoConversationMemberListSort(request.GetSort()),
		UserIDPrefix:   request.GetUserIdPrefix(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	members := make([]*conversationv1.ConversationMember, 0, len(result.Members))
	for _, member := range result.Members {
		members = append(members, &conversationv1.ConversationMember{
			UserId:            string(member.UserID),
			Role:              toProtoMemberRole(member.Role),
			Status:            toProtoMemberStatus(member.Status),
			JoinSeq:           member.JoinSeq,
			LeaveSeq:          member.LeaveSeq,
			MemberVersion:     member.MemberVersion,
			PermissionVersion: member.PermissionVersion,
			UpdatedAtUnixMs:   member.UpdatedAt.UnixMilli(),
		})
	}
	return &conversationv1.ListConversationMembersResponse{
		TenantId:          string(result.TenantID),
		ConversationId:    string(result.ConversationID),
		MemberVersion:     result.MemberVersion,
		PermissionVersion: result.PermissionVersion,
		Members:           members,
		NextPageToken:     result.NextPageToken,
	}, nil
}

func (s *Server) GetConversationProfile(
	ctx context.Context,
	request *conversationv1.GetConversationProfileRequest,
) (*conversationv1.GetConversationProfileResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.getConversationProfile == nil {
		return nil, status.Error(codes.Unimplemented, "get conversation profile is not configured")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := s.getConversationProfile.Execute(ctx, types.GetConversationProfileCommand{
		AuthContext:    auth,
		ConversationID: types.ConversationID(request.GetConversationId()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.GetConversationProfileResponse{
		Profile: toProtoConversationProfile(result),
	}, nil
}

func (s *Server) UpdateConversationProfile(
	ctx context.Context,
	request *conversationv1.UpdateConversationProfileRequest,
) (*conversationv1.UpdateConversationProfileResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.updateProfile == nil {
		return nil, status.Error(codes.Unimplemented, "update conversation profile is not configured")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := s.updateProfile.Execute(ctx, types.UpdateConversationProfileCommand{
		AuthContext:            auth,
		ConversationID:         types.ConversationID(request.GetConversationId()),
		Title:                  request.GetTitle(),
		AvatarURI:              request.GetAvatarUri(),
		Announcement:           request.GetAnnouncement(),
		ExpectedProfileVersion: request.GetExpectedProfileVersion(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.UpdateConversationProfileResponse{
		Profile: toProtoConversationProfile(result),
	}, nil
}

func (s *Server) CreateConversationNote(
	ctx context.Context,
	request *conversationv1.CreateConversationNoteRequest,
) (*conversationv1.CreateConversationNoteResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.createNote == nil {
		return nil, status.Error(codes.Unimplemented, "create conversation note is not configured")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := s.createNote.Execute(ctx, types.CreateConversationNoteCommand{
		AuthContext:       auth,
		ConversationID:    types.ConversationID(request.GetConversationId()),
		Body:              request.GetBody(),
		IdempotencyKey:    request.GetIdempotencyKey(),
		SourceToolName:    request.GetSourceToolName(),
		SourceProposalID:  request.GetSourceProposalId(),
		SourceApprovalID:  request.GetSourceApprovalId(),
		SourceExecutionID: request.GetSourceExecutionId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.CreateConversationNoteResponse{
		Note:             toProtoConversationNote(result),
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func authFromProto(ctx context.Context, auth *conversationv1.AuthContext) (types.AuthContext, bool) {
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

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrConversationNotFound):
		return status.Error(codes.NotFound, "conversation not found")
	case errors.Is(err, types.ErrMemberChangeNotFound):
		return status.Error(codes.NotFound, "member change not found")
	case errors.Is(err, types.ErrMemberNotActive):
		return status.Error(codes.PermissionDenied, "conversation member is not active")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrMemberConflict):
		return status.Error(codes.FailedPrecondition, "member conflict")
	case errors.Is(err, types.ErrProfileConflict):
		return status.Error(codes.FailedPrecondition, "conversation profile conflict")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "conversation read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "conversation write failed")
	case errors.Is(err, types.ErrOutboxWriteFailed):
		return status.Error(codes.Unavailable, "outbox write failed")
	case errors.Is(err, types.ErrSequencerUnavailable):
		return status.Error(codes.Unavailable, "sequencer unavailable")
	default:
		return status.Error(codes.Internal, "conversation service internal error")
	}
}

func toProtoConversationProfile(result types.ConversationProfileResult) *conversationv1.ConversationProfile {
	return &conversationv1.ConversationProfile{
		TenantId:          string(result.TenantID),
		ConversationId:    string(result.ConversationID),
		ConversationType:  toProtoConversationType(result.ConversationType),
		Title:             result.Title,
		AvatarUri:         result.AvatarURI,
		Announcement:      result.Announcement,
		ProfileVersion:    result.ProfileVersion,
		MemberVersion:     result.MemberVersion,
		PermissionVersion: result.PermissionVersion,
		UpdatedAtUnixMs:   result.UpdatedAt.UnixMilli(),
	}
}

func toProtoConversationNote(result types.ConversationNoteResult) *conversationv1.ConversationNote {
	return &conversationv1.ConversationNote{
		TenantId:          string(result.TenantID),
		ConversationId:    string(result.ConversationID),
		NoteId:            string(result.NoteID),
		AuthorUserId:      string(result.AuthorUserID),
		Body:              result.Body,
		SourceToolName:    result.SourceToolName,
		SourceProposalId:  result.SourceProposalID,
		SourceApprovalId:  result.SourceApprovalID,
		SourceExecutionId: result.SourceExecutionID,
		CreatedAtUnixMs:   result.CreatedAt.UnixMilli(),
	}
}

func toProtoConversationMode(mode types.ConversationMode) conversationv1.ConversationMode {
	switch mode {
	case types.ConversationModeLocalRowLock:
		return conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK
	case types.ConversationModeSequencerBlock:
		return conversationv1.ConversationMode_CONVERSATION_MODE_SEQUENCER_BLOCK
	default:
		return conversationv1.ConversationMode_CONVERSATION_MODE_UNSPECIFIED
	}
}

func fromProtoConversationType(value conversationv1.ConversationType) types.ConversationType {
	switch value {
	case conversationv1.ConversationType_CONVERSATION_TYPE_GROUP:
		return types.ConversationTypeGroup
	case conversationv1.ConversationType_CONVERSATION_TYPE_DIRECT:
		return types.ConversationTypeDirect
	default:
		return ""
	}
}

func toProtoConversationType(value types.ConversationType) conversationv1.ConversationType {
	switch value {
	case types.ConversationTypeGroup:
		return conversationv1.ConversationType_CONVERSATION_TYPE_GROUP
	case types.ConversationTypeDirect:
		return conversationv1.ConversationType_CONVERSATION_TYPE_DIRECT
	default:
		return conversationv1.ConversationType_CONVERSATION_TYPE_UNSPECIFIED
	}
}

func fromProtoMemberChangeType(value conversationv1.MemberChangeType) types.MemberChangeType {
	switch value {
	case conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN:
		return types.MemberChangeTypeJoin
	case conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_LEAVE:
		return types.MemberChangeTypeLeave
	case conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_REMOVE:
		return types.MemberChangeTypeRemove
	case conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_ROLE_CHANGED:
		return types.MemberChangeTypeRoleChanged
	default:
		return ""
	}
}

func toProtoMemberChangeType(value types.MemberChangeType) conversationv1.MemberChangeType {
	switch value {
	case types.MemberChangeTypeJoin:
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN
	case types.MemberChangeTypeLeave:
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_LEAVE
	case types.MemberChangeTypeRemove:
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_REMOVE
	case types.MemberChangeTypeRoleChanged:
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_ROLE_CHANGED
	case types.MemberChangeTypeOwnerTransfer:
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_OWNER_TRANSFER
	default:
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_UNSPECIFIED
	}
}

func fromProtoMemberRole(value conversationv1.MemberRole) types.MemberRole {
	switch value {
	case conversationv1.MemberRole_MEMBER_ROLE_OWNER:
		return types.MemberRoleOwner
	case conversationv1.MemberRole_MEMBER_ROLE_ADMIN:
		return types.MemberRoleAdmin
	case conversationv1.MemberRole_MEMBER_ROLE_MEMBER:
		return types.MemberRoleMember
	default:
		return ""
	}
}

func fromProtoMemberRoles(values []conversationv1.MemberRole) []types.MemberRole {
	roles := make([]types.MemberRole, 0, len(values))
	for _, value := range values {
		roles = append(roles, fromProtoMemberRole(value))
	}
	return roles
}

func fromProtoConversationMemberListSort(value conversationv1.ConversationMemberListSort) string {
	switch value {
	case conversationv1.ConversationMemberListSort_CONVERSATION_MEMBER_LIST_SORT_USER_ID_ASC:
		return types.ConversationMemberListSortUserIDAsc
	case conversationv1.ConversationMemberListSort_CONVERSATION_MEMBER_LIST_SORT_ROLE_USER_ID_ASC:
		return types.ConversationMemberListSortRoleUserIDAsc
	default:
		return ""
	}
}

func toProtoMemberRole(value types.MemberRole) conversationv1.MemberRole {
	switch value {
	case types.MemberRoleOwner:
		return conversationv1.MemberRole_MEMBER_ROLE_OWNER
	case types.MemberRoleAdmin:
		return conversationv1.MemberRole_MEMBER_ROLE_ADMIN
	case types.MemberRoleMember:
		return conversationv1.MemberRole_MEMBER_ROLE_MEMBER
	default:
		return conversationv1.MemberRole_MEMBER_ROLE_UNSPECIFIED
	}
}

func toProtoMemberStatus(value types.MemberStatus) conversationv1.MemberStatus {
	switch value {
	case types.MemberStatusActive:
		return conversationv1.MemberStatus_MEMBER_STATUS_ACTIVE
	case types.MemberStatusLeft:
		return conversationv1.MemberStatus_MEMBER_STATUS_LEFT
	case types.MemberStatusBanned:
		return conversationv1.MemberStatus_MEMBER_STATUS_BANNED
	default:
		return conversationv1.MemberStatus_MEMBER_STATUS_UNSPECIFIED
	}
}

func fromProtoConflictPolicy(value conversationv1.MemberChangeConflictPolicy) types.MemberChangeConflictPolicy {
	switch value {
	case conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT:
		return types.MemberChangeConflictPolicyReject
	case conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_MERGE:
		return types.MemberChangeConflictPolicyMerge
	case conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_COMPENSATE:
		return types.MemberChangeConflictPolicyCompensate
	default:
		return ""
	}
}

func toProtoMemberChangeStatus(value types.MemberChangeStatus) conversationv1.MemberChangeStatus {
	switch value {
	case types.MemberChangeStatusPendingBoundary:
		return conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_PENDING_BOUNDARY
	case types.MemberChangeStatusBoundaryAllocated:
		return conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_BOUNDARY_ALLOCATED
	case types.MemberChangeStatusMemberUpdated:
		return conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_MEMBER_UPDATED
	case types.MemberChangeStatusOutboxEnqueued:
		return conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED
	case types.MemberChangeStatusEventPublished:
		return conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_EVENT_PUBLISHED
	case types.MemberChangeStatusDone:
		return conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_DONE
	case types.MemberChangeStatusFailedCompensated:
		return conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_FAILED_COMPENSATED
	default:
		return conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_UNSPECIFIED
	}
}

func toProtoFanoutMode(mode types.FanoutMode) conversationv1.FanoutMode {
	switch mode {
	case types.FanoutModeWriteFanout:
		return conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT
	case types.FanoutModeHybridFanout:
		return conversationv1.FanoutMode_FANOUT_MODE_HYBRID_FANOUT
	case types.FanoutModeReadFanout:
		return conversationv1.FanoutMode_FANOUT_MODE_READ_FANOUT
	case types.FanoutModeBroadcastSignal:
		return conversationv1.FanoutMode_FANOUT_MODE_BROADCAST_SIGNAL
	default:
		return conversationv1.FanoutMode_FANOUT_MODE_UNSPECIFIED
	}
}
