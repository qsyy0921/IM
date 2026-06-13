package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGetSendContextConvertsResponse(t *testing.T) {
	executor := &fakeGetSendContextExecutor{
		result: types.ConversationSendContext{
			TenantID:            "tenant-1",
			ConversationID:      "conv-1",
			MemberVersion:       5,
			PermissionVersion:   7,
			ConversationMode:    types.ConversationModeLocalRowLock,
			FanoutMode:          types.FanoutModeWriteFanout,
			FanoutPolicyVersion: 3,
			CurrentSeqShard:     "local",
			DirectPeerUserID:    "user-2",
		},
	}
	server := NewServer(executor)

	response, err := server.GetSendContext(context.Background(), &conversationv1.GetSendContextRequest{
		TenantId:       "tenant-1",
		ConversationId: "conv-1",
		UserId:         "user-1",
		TraceId:        "trace-1",
	})
	if err != nil {
		t.Fatalf("get send context: %v", err)
	}
	if executor.command.TenantID != "tenant-1" ||
		executor.command.ConversationID != "conv-1" ||
		executor.command.UserID != "user-1" ||
		executor.command.TraceID != "trace-1" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetConversationMode() != conversationv1.ConversationMode_CONVERSATION_MODE_LOCAL_ROW_LOCK ||
		response.GetFanoutMode() != conversationv1.FanoutMode_FANOUT_MODE_WRITE_FANOUT ||
		response.GetMemberVersion() != 5 ||
		response.GetPermissionVersion() != 7 ||
		response.GetFanoutPolicyVersion() != 3 ||
		response.GetCurrentSeqShard() != "local" ||
		response.GetDirectPeerUserId() != "user-2" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestGetSendContextMapsErrors(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		code    codes.Code
		message string
	}{
		{name: "invalid argument", err: types.NewInvalidArgument("tenant_id is required"), code: codes.InvalidArgument, message: "invalid argument"},
		{name: "not found", err: types.NewConversationNotFound("missing"), code: codes.NotFound, message: "conversation not found"},
		{name: "member change not found", err: types.NewMemberChangeNotFound("missing"), code: codes.NotFound, message: "member change not found"},
		{name: "member inactive", err: types.NewMemberNotActive("left"), code: codes.PermissionDenied, message: "conversation member is not active"},
		{name: "permission denied", err: types.NewPermissionDenied("not admin"), code: codes.PermissionDenied, message: "permission denied"},
		{name: "member conflict", err: types.NewMemberConflict("version conflict"), code: codes.FailedPrecondition, message: "member conflict"},
		{name: "db read", err: types.NewDBReadFailed("select conversations timeout"), code: codes.Unavailable, message: "conversation read failed"},
		{name: "db write", err: types.NewDBWriteFailed("insert member_change_saga failed"), code: codes.Unavailable, message: "conversation write failed"},
		{name: "outbox write", err: types.NewOutboxWriteFailed("message_outbox constraint failed"), code: codes.Unavailable, message: "outbox write failed"},
		{name: "sequencer unavailable", err: types.NewSequencerUnavailable("hot shard"), code: codes.Unavailable, message: "sequencer unavailable"},
		{name: "unknown", err: errors.New("boom"), code: codes.Internal, message: "conversation service internal error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(&fakeGetSendContextExecutor{err: tc.err})

			_, err := server.GetSendContext(context.Background(), &conversationv1.GetSendContextRequest{
				TenantId:       "tenant-1",
				ConversationId: "conv-1",
				UserId:         "user-1",
			})
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected status error, got %v", err)
			}
			if st.Code() != tc.code {
				t.Fatalf("expected %s, got %s", tc.code, st.Code())
			}
			if st.Message() != tc.message {
				t.Fatalf("expected stable message %q, got %q", tc.message, st.Message())
			}
			if strings.Contains(st.Message(), "select conversations") || strings.Contains(st.Message(), "boom") {
				t.Fatalf("status message leaked internal detail: %q", st.Message())
			}
		})
	}
}

func TestGetSendContextMapsValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		request *conversationv1.GetSendContextRequest
	}{
		{
			name: "missing tenant",
			request: &conversationv1.GetSendContextRequest{
				ConversationId: "conv-1",
				UserId:         "user-1",
			},
		},
		{
			name: "missing conversation",
			request: &conversationv1.GetSendContextRequest{
				TenantId: "tenant-1",
				UserId:   "user-1",
			},
		},
		{
			name: "missing user",
			request: &conversationv1.GetSendContextRequest{
				TenantId:       "tenant-1",
				ConversationId: "conv-1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewServer(&fakeGetSendContextExecutor{validate: true}).GetSendContext(context.Background(), tc.request)
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected status error, got %v", err)
			}
			if st.Code() != codes.InvalidArgument {
				t.Fatalf("expected invalid argument, got %s", st.Code())
			}
			if st.Message() != "invalid argument" {
				t.Fatalf("expected stable message, got %q", st.Message())
			}
		})
	}
}

func TestGetSendContextRejectsNilRequest(t *testing.T) {
	_, err := NewServer(&fakeGetSendContextExecutor{}).GetSendContext(context.Background(), nil)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %s", st.Code())
	}
}

func TestConversationAuthMetadataOverridesBodyForMemberCommands(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
		metadataSessionID, "trusted-session",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		create := &fakeCreateMemberChangeExecutor{}
		transfer := &fakeTransferConversationOwnerExecutor{}
		get := &fakeGetMemberChangeExecutor{}
		list := &fakeListConversationMembersExecutor{}
		server := NewServer(
			&fakeGetSendContextExecutor{},
			WithCreateMemberChange(create),
			WithTransferConversationOwner(transfer),
			WithGetMemberChange(get),
			WithListConversationMembers(list),
		)

		if _, err := server.CreateMemberChange(ctx, &conversationv1.CreateMemberChangeRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conv-1",
			TargetUserId:   "target-1",
		}); err != nil {
			t.Fatalf("create member change: %v", err)
		}
		if _, err := server.TransferConversationOwner(ctx, &conversationv1.TransferConversationOwnerRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conv-1",
			NewOwnerUserId: "target-1",
		}); err != nil {
			t.Fatalf("transfer owner: %v", err)
		}
		if _, err := server.GetMemberChange(ctx, &conversationv1.GetMemberChangeRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conv-1",
			ChangeId:       "change-1",
		}); err != nil {
			t.Fatalf("get member change: %v", err)
		}
		if _, err := server.ListConversationMembers(ctx, &conversationv1.ListConversationMembersRequest{
			AuthContext:    testSpoofedAuthContext(),
			ConversationId: "conv-1",
			PageSize:       20,
		}); err != nil {
			t.Fatalf("list conversation members: %v", err)
		}

		assertTrustedMetadataAuth(t, create.command.AuthContext)
		assertTrustedMetadataAuth(t, transfer.command.AuthContext)
		assertTrustedMetadataAuth(t, get.command.AuthContext)
		assertTrustedMetadataAuth(t, list.command.AuthContext)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestConversationAuthMetadataDoesNotRequireBodyAuthContext(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
		metadataTraceID, "trusted-trace",
		metadataRequestID, "trusted-request",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		list := &fakeListConversationMembersExecutor{}
		server := NewServer(
			&fakeGetSendContextExecutor{},
			WithListConversationMembers(list),
		)
		if _, err := server.ListConversationMembers(ctx, &conversationv1.ListConversationMembersRequest{
			ConversationId: "conv-1",
			PageSize:       20,
		}); err != nil {
			t.Fatalf("list conversation members: %v", err)
		}
		auth := list.command.AuthContext
		if auth.TenantID != "trusted-tenant" ||
			auth.UserID != "trusted-user" ||
			auth.DeviceID != "trusted-device" ||
			auth.TraceID != "trusted-trace" ||
			auth.RequestID != "trusted-request" {
			t.Fatalf("unexpected verified auth: %+v", auth)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestVerifiedAuthUnaryInterceptorRequiresTrustedIdentity(t *testing.T) {
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, request any) (any, error) {
		t.Fatal("handler should not be called without verified auth")
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v (%v)", status.Code(err), err)
	}
}

func TestVerifiedAuthUnaryInterceptorAllowsGetSendContextWithoutMetadata(t *testing.T) {
	interceptor := VerifiedAuthUnaryInterceptor(true)
	called := false
	_, err := interceptor(
		context.Background(),
		nil,
		&grpcgo.UnaryServerInfo{FullMethod: conversationv1.ConversationService_GetSendContext_FullMethodName},
		func(ctx context.Context, request any) (any, error) {
			called = true
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("expected get send context to bypass verified user auth metadata: %v", err)
	}
	if !called {
		t.Fatalf("handler was not called")
	}
}

func TestCreateMemberChangeConvertsRequestAndResponse(t *testing.T) {
	executor := &fakeCreateMemberChangeExecutor{
		result: types.MemberChangeResult{
			ChangeID:          "change-1",
			TenantID:          "tenant-1",
			ConversationID:    "conv-1",
			TargetUserID:      "target-1",
			OperatorUserID:    "owner-1",
			ChangeType:        types.MemberChangeTypeJoin,
			Status:            types.MemberChangeStatusOutboxEnqueued,
			BoundarySeq:       12,
			MemberVersion:     6,
			PermissionVersion: 8,
			IdempotentReplay:  true,
		},
	}
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithCreateMemberChange(executor),
	)

	response, err := server.CreateMemberChange(context.Background(), &conversationv1.CreateMemberChangeRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  "tenant-1",
			UserId:    "owner-1",
			DeviceId:  "device-1",
			SessionId: "session-1",
			TraceId:   "trace-1",
			RequestId: "request-1",
		},
		ConversationId:        "conv-1",
		TargetUserId:          "target-1",
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: 5,
		IdempotencyKey:        "idem-1",
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "invite",
	})
	if err != nil {
		t.Fatalf("create member change: %v", err)
	}
	if executor.command.AuthContext.TenantID != "tenant-1" ||
		executor.command.AuthContext.UserID != "owner-1" ||
		executor.command.AuthContext.DeviceID != "device-1" ||
		executor.command.AuthContext.SessionID != "session-1" ||
		executor.command.AuthContext.TraceID != "trace-1" ||
		executor.command.AuthContext.RequestID != "request-1" ||
		executor.command.ConversationID != "conv-1" ||
		executor.command.TargetUserID != "target-1" ||
		executor.command.ChangeType != types.MemberChangeTypeJoin ||
		executor.command.TargetRole != types.MemberRoleMember ||
		executor.command.ExpectedMemberVersion != 5 ||
		executor.command.IdempotencyKey != "idem-1" ||
		executor.command.ConflictPolicy != types.MemberChangeConflictPolicyReject ||
		executor.command.Reason != "invite" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetChangeId() != "change-1" ||
		response.GetChangeType() != conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN ||
		response.GetStatus() != conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED ||
		response.GetBoundarySeq() != 12 ||
		response.GetMemberVersion() != 6 ||
		response.GetPermissionVersion() != 8 ||
		!response.GetIdempotentReplay() {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestCreateMemberChangeMapsValidationErrors(t *testing.T) {
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithCreateMemberChange(&fakeCreateMemberChangeExecutor{validate: true}),
	)
	_, err := server.CreateMemberChange(context.Background(), &conversationv1.CreateMemberChangeRequest{
		AuthContext:    validProtoAuthContext(),
		ConversationId: "conv-1",
		ChangeType:     conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:     conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ConflictPolicy: conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		IdempotencyKey: "idem-1",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument || st.Message() != "invalid argument" {
		t.Fatalf("unexpected status: %s %q", st.Code(), st.Message())
	}
}

func TestCreateMemberChangeRejectsNilRequest(t *testing.T) {
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithCreateMemberChange(&fakeCreateMemberChangeExecutor{}),
	)
	_, err := server.CreateMemberChange(context.Background(), nil)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %s", st.Code())
	}
}

func TestCreateMemberChangeRequiresExecutor(t *testing.T) {
	_, err := NewServer(&fakeGetSendContextExecutor{}).CreateMemberChange(
		context.Background(),
		&conversationv1.CreateMemberChangeRequest{},
	)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Fatalf("expected unimplemented, got %s", st.Code())
	}
}

func TestTransferConversationOwnerConvertsRequestAndResponse(t *testing.T) {
	executor := &fakeTransferConversationOwnerExecutor{
		result: types.TransferConversationOwnerResult{
			ChangeID:            "change-owner-1",
			TenantID:            "tenant-1",
			ConversationID:      "conv-1",
			PreviousOwnerUserID: "owner-1",
			NewOwnerUserID:      "user-2",
			Status:              types.MemberChangeStatusOutboxEnqueued,
			BoundarySeq:         12,
			MemberVersion:       6,
			PermissionVersion:   8,
			IdempotentReplay:    true,
		},
	}
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithTransferConversationOwner(executor),
	)

	response, err := server.TransferConversationOwner(context.Background(), &conversationv1.TransferConversationOwnerRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  "tenant-1",
			UserId:    "owner-1",
			DeviceId:  "device-1",
			SessionId: "session-1",
			TraceId:   "trace-1",
			RequestId: "request-1",
		},
		ConversationId:        "conv-1",
		NewOwnerUserId:        "user-2",
		ExpectedMemberVersion: 5,
		IdempotencyKey:        "idem-owner-1",
		Reason:                "handoff",
	})
	if err != nil {
		t.Fatalf("transfer conversation owner: %v", err)
	}
	if executor.command.AuthContext.TenantID != "tenant-1" ||
		executor.command.AuthContext.UserID != "owner-1" ||
		executor.command.AuthContext.DeviceID != "device-1" ||
		executor.command.AuthContext.SessionID != "session-1" ||
		executor.command.AuthContext.TraceID != "trace-1" ||
		executor.command.AuthContext.RequestID != "request-1" ||
		executor.command.ConversationID != "conv-1" ||
		executor.command.NewOwnerUserID != "user-2" ||
		executor.command.ExpectedMemberVersion != 5 ||
		executor.command.IdempotencyKey != "idem-owner-1" ||
		executor.command.Reason != "handoff" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetChangeId() != "change-owner-1" ||
		response.GetTenantId() != "tenant-1" ||
		response.GetConversationId() != "conv-1" ||
		response.GetPreviousOwnerUserId() != "owner-1" ||
		response.GetNewOwnerUserId() != "user-2" ||
		response.GetStatus() != conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_OUTBOX_ENQUEUED ||
		response.GetBoundarySeq() != 12 ||
		response.GetMemberVersion() != 6 ||
		response.GetPermissionVersion() != 8 ||
		!response.GetIdempotentReplay() {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestTransferConversationOwnerMapsValidationErrors(t *testing.T) {
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithTransferConversationOwner(&fakeTransferConversationOwnerExecutor{validate: true}),
	)
	_, err := server.TransferConversationOwner(context.Background(), &conversationv1.TransferConversationOwnerRequest{
		AuthContext:    validProtoAuthContext(),
		ConversationId: "conv-1",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument || st.Message() != "invalid argument" {
		t.Fatalf("unexpected status: %s %q", st.Code(), st.Message())
	}
}

func TestTransferConversationOwnerRejectsNilRequest(t *testing.T) {
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithTransferConversationOwner(&fakeTransferConversationOwnerExecutor{}),
	)
	_, err := server.TransferConversationOwner(context.Background(), nil)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %s", st.Code())
	}
}

func TestTransferConversationOwnerRequiresExecutor(t *testing.T) {
	_, err := NewServer(&fakeGetSendContextExecutor{}).TransferConversationOwner(
		context.Background(),
		&conversationv1.TransferConversationOwnerRequest{},
	)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Fatalf("expected unimplemented, got %s", st.Code())
	}
}

func TestGetMemberChangeConvertsRequestAndResponse(t *testing.T) {
	executor := &fakeGetMemberChangeExecutor{
		result: types.MemberChangeDetail{
			ChangeID:          "change-1",
			TenantID:          "tenant-1",
			ConversationID:    "conv-1",
			TargetUserID:      "target-1",
			OperatorUserID:    "owner-1",
			ChangeType:        types.MemberChangeTypeJoin,
			Status:            types.MemberChangeStatusDone,
			BoundarySeq:       12,
			MemberVersion:     6,
			PermissionVersion: 8,
			OldRole:           "",
			NewRole:           types.MemberRoleMember,
			Reason:            "invite",
			LastError:         "last error",
		},
	}
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithGetMemberChange(executor),
	)

	response, err := server.GetMemberChange(context.Background(), &conversationv1.GetMemberChangeRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  "tenant-1",
			UserId:    "owner-1",
			DeviceId:  "device-1",
			SessionId: "session-1",
			TraceId:   "trace-1",
			RequestId: "request-1",
		},
		ConversationId: "conv-1",
		ChangeId:       "change-1",
	})
	if err != nil {
		t.Fatalf("get member change: %v", err)
	}
	if executor.command.AuthContext.TenantID != "tenant-1" ||
		executor.command.AuthContext.UserID != "owner-1" ||
		executor.command.ConversationID != "conv-1" ||
		executor.command.ChangeID != "change-1" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetChangeId() != "change-1" ||
		response.GetTenantId() != "tenant-1" ||
		response.GetConversationId() != "conv-1" ||
		response.GetTargetUserId() != "target-1" ||
		response.GetOperatorUserId() != "owner-1" ||
		response.GetChangeType() != conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN ||
		response.GetStatus() != conversationv1.MemberChangeStatus_MEMBER_CHANGE_STATUS_DONE ||
		response.GetBoundarySeq() != 12 ||
		response.GetMemberVersion() != 6 ||
		response.GetPermissionVersion() != 8 ||
		response.GetOldRole() != conversationv1.MemberRole_MEMBER_ROLE_UNSPECIFIED ||
		response.GetNewRole() != conversationv1.MemberRole_MEMBER_ROLE_MEMBER ||
		response.GetReason() != "invite" ||
		response.GetLastError() != "last error" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestGetMemberChangeMapsValidationErrors(t *testing.T) {
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithGetMemberChange(&fakeGetMemberChangeExecutor{validate: true}),
	)
	_, err := server.GetMemberChange(context.Background(), &conversationv1.GetMemberChangeRequest{
		AuthContext:    validProtoAuthContext(),
		ConversationId: "conv-1",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument || st.Message() != "invalid argument" {
		t.Fatalf("unexpected status: %s %q", st.Code(), st.Message())
	}
}

func TestGetMemberChangeRequiresExecutor(t *testing.T) {
	_, err := NewServer(&fakeGetSendContextExecutor{}).GetMemberChange(
		context.Background(),
		&conversationv1.GetMemberChangeRequest{},
	)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Fatalf("expected unimplemented, got %s", st.Code())
	}
}

func TestListConversationMembersConvertsRequestAndResponse(t *testing.T) {
	updatedAt := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	executor := &fakeListConversationMembersExecutor{
		result: types.ListConversationMembersResult{
			TenantID:          "tenant-1",
			ConversationID:    "conv-1",
			MemberVersion:     10,
			PermissionVersion: 12,
			NextPageToken:     "next-token",
			Members: []types.ConversationMember{
				{
					UserID:            "user-1",
					Role:              types.MemberRoleOwner,
					Status:            types.MemberStatusActive,
					JoinSeq:           1,
					MemberVersion:     10,
					PermissionVersion: 12,
					UpdatedAt:         updatedAt,
				},
			},
		},
	}
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithListConversationMembers(executor),
	)

	response, err := server.ListConversationMembers(context.Background(), &conversationv1.ListConversationMembersRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  "tenant-1",
			UserId:    "user-1",
			DeviceId:  "device-1",
			SessionId: "session-1",
			TraceId:   "trace-1",
			RequestId: "request-1",
		},
		ConversationId: "conv-1",
		PageSize:       50,
		PageToken:      "page-token",
	})
	if err != nil {
		t.Fatalf("list conversation members: %v", err)
	}
	if executor.command.AuthContext.TenantID != "tenant-1" ||
		executor.command.AuthContext.UserID != "user-1" ||
		executor.command.AuthContext.DeviceID != "device-1" ||
		executor.command.AuthContext.SessionID != "session-1" ||
		executor.command.AuthContext.TraceID != "trace-1" ||
		executor.command.AuthContext.RequestID != "request-1" ||
		executor.command.ConversationID != "conv-1" ||
		executor.command.PageSize != 50 ||
		executor.command.PageToken != "page-token" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetTenantId() != "tenant-1" ||
		response.GetConversationId() != "conv-1" ||
		response.GetMemberVersion() != 10 ||
		response.GetPermissionVersion() != 12 ||
		response.GetNextPageToken() != "next-token" ||
		len(response.GetMembers()) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	member := response.GetMembers()[0]
	if member.GetUserId() != "user-1" ||
		member.GetRole() != conversationv1.MemberRole_MEMBER_ROLE_OWNER ||
		member.GetStatus() != conversationv1.MemberStatus_MEMBER_STATUS_ACTIVE ||
		member.GetJoinSeq() != 1 ||
		member.GetMemberVersion() != 10 ||
		member.GetPermissionVersion() != 12 ||
		member.GetUpdatedAtUnixMs() != updatedAt.UnixMilli() {
		t.Fatalf("unexpected member: %+v", member)
	}
}

func TestListConversationMembersMapsValidationErrors(t *testing.T) {
	server := NewServer(
		&fakeGetSendContextExecutor{},
		WithListConversationMembers(&fakeListConversationMembersExecutor{validate: true}),
	)
	_, err := server.ListConversationMembers(context.Background(), &conversationv1.ListConversationMembersRequest{
		AuthContext:    validProtoAuthContext(),
		ConversationId: "conv-1",
		PageSize:       -1,
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument || st.Message() != "invalid argument" {
		t.Fatalf("unexpected status: %s %q", st.Code(), st.Message())
	}
}

func TestListConversationMembersRequiresExecutor(t *testing.T) {
	_, err := NewServer(&fakeGetSendContextExecutor{}).ListConversationMembers(
		context.Background(),
		&conversationv1.ListConversationMembersRequest{},
	)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Fatalf("expected unimplemented, got %s", st.Code())
	}
}

func testSpoofedAuthContext() *conversationv1.AuthContext {
	return &conversationv1.AuthContext{
		TenantId:  "spoofed-tenant",
		UserId:    "spoofed-user",
		DeviceId:  "spoofed-device",
		SessionId: "spoofed-session",
		TraceId:   "body-trace",
		RequestId: "body-request",
	}
}

func validProtoAuthContext() *conversationv1.AuthContext {
	return &conversationv1.AuthContext{
		TenantId:  "tenant-1",
		UserId:    "user-1",
		DeviceId:  "device-1",
		SessionId: "session-1",
		TraceId:   "trace-1",
		RequestId: "request-1",
	}
}

func assertTrustedMetadataAuth(t *testing.T, auth types.AuthContext) {
	t.Helper()
	if auth.TenantID != "trusted-tenant" ||
		auth.UserID != "trusted-user" ||
		auth.DeviceID != "trusted-device" ||
		auth.SessionID != "trusted-session" ||
		auth.TraceID != "body-trace" ||
		auth.RequestID != "body-request" {
		t.Fatalf("unexpected verified auth: %+v", auth)
	}
}

type fakeGetSendContextExecutor struct {
	result   types.ConversationSendContext
	err      error
	command  types.GetSendContextCommand
	validate bool
}

type fakeGetMemberChangeExecutor struct {
	result   types.MemberChangeDetail
	err      error
	command  types.GetMemberChangeCommand
	validate bool
}

func (f *fakeGetMemberChangeExecutor) Execute(
	_ context.Context,
	command types.GetMemberChangeCommand,
) (types.MemberChangeDetail, error) {
	f.command = command
	if f.validate {
		if err := command.Validate(); err != nil {
			return types.MemberChangeDetail{}, err
		}
	}
	return f.result, f.err
}

func (f *fakeGetSendContextExecutor) Execute(
	_ context.Context,
	command types.GetSendContextCommand,
) (types.ConversationSendContext, error) {
	f.command = command
	if f.validate {
		if err := command.Validate(); err != nil {
			return types.ConversationSendContext{}, err
		}
	}
	return f.result, f.err
}

type fakeCreateMemberChangeExecutor struct {
	result   types.MemberChangeResult
	err      error
	command  types.CreateMemberChangeCommand
	validate bool
}

func (f *fakeCreateMemberChangeExecutor) Execute(
	_ context.Context,
	command types.CreateMemberChangeCommand,
) (types.MemberChangeResult, error) {
	f.command = command
	if f.validate {
		if err := command.Validate(); err != nil {
			return types.MemberChangeResult{}, err
		}
	}
	return f.result, f.err
}

type fakeTransferConversationOwnerExecutor struct {
	result   types.TransferConversationOwnerResult
	err      error
	command  types.TransferConversationOwnerCommand
	validate bool
}

func (f *fakeTransferConversationOwnerExecutor) Execute(
	_ context.Context,
	command types.TransferConversationOwnerCommand,
) (types.TransferConversationOwnerResult, error) {
	f.command = command
	if f.validate {
		if err := command.Validate(); err != nil {
			return types.TransferConversationOwnerResult{}, err
		}
	}
	return f.result, f.err
}

type fakeListConversationMembersExecutor struct {
	result   types.ListConversationMembersResult
	err      error
	command  types.ListConversationMembersCommand
	validate bool
}

func (f *fakeListConversationMembersExecutor) Execute(
	_ context.Context,
	command types.ListConversationMembersCommand,
) (types.ListConversationMembersResult, error) {
	f.command = command
	if f.validate {
		if err := command.Validate(); err != nil {
			return types.ListConversationMembersResult{}, err
		}
	}
	return f.result, f.err
}
