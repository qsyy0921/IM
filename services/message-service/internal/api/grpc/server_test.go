package grpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSendMessageConvertsRequestAndResponse(t *testing.T) {
	acceptedAt := time.Unix(100, 200).UTC()
	receivedAt := time.Unix(90, 0).UTC()
	executor := &fakeSendMessageExecutor{
		result: types.SendMessageResult{
			MessageID:        "msg-1",
			ConversationID:   "conv-1",
			ConversationSeq:  7,
			AcceptedAt:       acceptedAt,
			IdempotentReplay: true,
		},
	}
	server := NewServer(executor, WithClock(func() time.Time { return receivedAt }))

	response, err := server.SendMessage(context.Background(), testSendMessageRequest())
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if response.GetMessageId() != "msg-1" ||
		response.GetConversationId() != "conv-1" ||
		response.GetConversationSeq() != 7 ||
		!response.GetIdempotentReplay() ||
		!response.GetAcceptedAt().AsTime().Equal(acceptedAt) {
		t.Fatalf("unexpected response: %+v", response)
	}
	if executor.calls != 1 {
		t.Fatalf("expected one use case call, got %d", executor.calls)
	}
	command := executor.command
	if command.AuthContext.TenantID != "tenant-1" ||
		command.AuthContext.UserID != "user-1" ||
		command.AuthContext.DeviceID != "device-1" ||
		command.AuthContext.SessionID != "session-1" ||
		command.AuthContext.TraceID != "trace-1" ||
		command.AuthContext.RequestID != "request-1" ||
		command.ConversationID != "conv-1" ||
		command.ClientMsgID != "client-1" ||
		command.MessageType != types.MessageTypeText ||
		string(command.PayloadJSON) != `{"text":"hello"}` ||
		command.ReceivedAt != receivedAt {
		t.Fatalf("unexpected command: %+v payload=%s", command, command.PayloadJSON)
	}
	if len(command.AttachmentIDs) != 2 || command.AttachmentIDs[0] != "att-2" || command.AttachmentIDs[1] != "att-1" {
		t.Fatalf("unexpected attachment ids: %+v", command.AttachmentIDs)
	}
}

func TestMessageServiceClientSendMessageThroughGRPC(t *testing.T) {
	acceptedAt := time.Unix(100, 0).UTC()
	executor := &fakeSendMessageExecutor{
		result: types.SendMessageResult{
			MessageID:       "msg-1",
			ConversationID:  "conv-1",
			ConversationSeq: 9,
			AcceptedAt:      acceptedAt,
		},
	}

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpcgo.NewServer()
	Register(grpcServer, NewServer(executor))
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpcgo.ErrServerStopped) {
			t.Errorf("serve bufconn grpc: %v", err)
		}
	}()
	defer grpcServer.Stop()

	conn, err := grpcgo.NewClient(
		"passthrough:///bufnet",
		grpcgo.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpcgo.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn grpc: %v", err)
	}
	defer conn.Close()

	response, err := messagev1.NewMessageServiceClient(conn).SendMessage(context.Background(), testSendMessageRequest())
	if err != nil {
		t.Fatalf("client send message: %v", err)
	}
	if response.GetMessageId() != "msg-1" || response.GetConversationSeq() != 9 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestMessageAuthMetadataOverridesBodyForAllCommands(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
		metadataSessionID, "trusted-session",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		sendExecutor := &fakeSendMessageExecutor{result: types.SendMessageResult{MessageID: "msg-1", ConversationID: "conv-1"}}
		editExecutor := &fakeEditMessageExecutor{result: types.MessageChangeResult{MessageID: "msg-1", ConversationID: "conv-1"}}
		revokeExecutor := &fakeRevokeMessageExecutor{result: types.MessageChangeResult{MessageID: "msg-1", ConversationID: "conv-1"}}
		deleteExecutor := &fakeDeleteMessageExecutor{result: types.MessageChangeResult{MessageID: "msg-1", ConversationID: "conv-1"}}
		server := NewServer(
			sendExecutor,
			WithEditMessage(editExecutor),
			WithRevokeMessage(revokeExecutor),
			WithDeleteMessage(deleteExecutor),
		)

		sendRequest := testSendMessageRequest()
		sendRequest.AuthContext = testSpoofedAuthContext()
		if _, err := server.SendMessage(ctx, sendRequest); err != nil {
			t.Fatalf("send message: %v", err)
		}
		if _, err := server.EditMessage(ctx, testEditMessageRequest()); err != nil {
			t.Fatalf("edit message: %v", err)
		}
		if _, err := server.RevokeMessage(ctx, testRevokeMessageRequest()); err != nil {
			t.Fatalf("revoke message: %v", err)
		}
		if _, err := server.DeleteMessage(ctx, testDeleteMessageRequest()); err != nil {
			t.Fatalf("delete message: %v", err)
		}

		assertTrustedMetadataAuth(t, sendExecutor.command.AuthContext)
		assertTrustedMetadataAuth(t, editExecutor.command.AuthContext)
		assertTrustedMetadataAuth(t, revokeExecutor.command.AuthContext)
		assertTrustedMetadataAuth(t, deleteExecutor.command.AuthContext)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestMessageAuthMetadataDoesNotRequireBodyAuthContext(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-user",
		metadataDeviceID, "trusted-device",
		metadataTraceID, "trusted-trace",
		metadataRequestID, "trusted-request",
	))
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		executor := &fakeSendMessageExecutor{result: types.SendMessageResult{MessageID: "msg-1", ConversationID: "conv-1"}}
		server := NewServer(executor)
		request := testSendMessageRequest()
		request.AuthContext = nil
		if _, err := server.SendMessage(ctx, request); err != nil {
			t.Fatalf("send message: %v", err)
		}
		command := executor.command
		if command.AuthContext.TenantID != "trusted-tenant" ||
			command.AuthContext.UserID != "trusted-user" ||
			command.AuthContext.DeviceID != "trusted-device" ||
			command.AuthContext.TraceID != "trusted-trace" ||
			command.AuthContext.RequestID != "trusted-request" {
			t.Fatalf("unexpected verified auth without body auth: %+v", command.AuthContext)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestVerifiedAuthUnaryInterceptorRequiresTrustedIdentity(t *testing.T) {
	interceptor := VerifiedAuthUnaryInterceptor(true)
	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called without verified auth")
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v (%v)", status.Code(err), err)
	}
}

func TestSendMessageRejectsInvalidRequest(t *testing.T) {
	server := NewServer(&fakeSendMessageExecutor{})

	_, err := server.SendMessage(context.Background(), &messagev1.SendMessageRequest{})
	assertStatusDetail(t, err, codes.InvalidArgument, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, false, "")
}

func TestSendMessageRejectsUnsupportedMessageType(t *testing.T) {
	server := NewServer(&fakeSendMessageExecutor{})
	request := testSendMessageRequest()
	request.MessageType = "STICKER"

	_, err := server.SendMessage(context.Background(), request)
	assertStatusDetail(
		t,
		err,
		codes.InvalidArgument,
		messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSUPPORTED_MESSAGE_TYPE,
		false,
		"request-1",
	)
}

func TestSendMessageAcceptsImageAttachmentMessage(t *testing.T) {
	executor := &fakeSendMessageExecutor{
		result: types.SendMessageResult{
			MessageID:       "msg-image",
			ConversationID:  "conv-1",
			ConversationSeq: 11,
			AcceptedAt:      time.Unix(100, 0).UTC(),
		},
	}
	server := NewServer(executor)
	request := testSendMessageRequest()
	request.MessageType = string(types.MessageTypeImage)
	request.AttachmentIds = []string{"img-1"}
	request.Payload = mustStruct(map[string]any{"caption": "hello", "width": float64(640), "height": float64(480)})

	if _, err := server.SendMessage(context.Background(), request); err != nil {
		t.Fatalf("send image attachment message: %v", err)
	}
	if executor.command.MessageType != types.MessageTypeImage ||
		len(executor.command.AttachmentIDs) != 1 ||
		executor.command.AttachmentIDs[0] != "img-1" {
		t.Fatalf("unexpected image command: %+v", executor.command)
	}
}

func TestSendMessageAcceptsVoiceAttachmentMessage(t *testing.T) {
	executor := &fakeSendMessageExecutor{
		result: types.SendMessageResult{
			MessageID:       "msg-voice",
			ConversationID:  "conv-1",
			ConversationSeq: 12,
			AcceptedAt:      time.Unix(100, 0).UTC(),
		},
	}
	server := NewServer(executor)
	request := testSendMessageRequest()
	request.MessageType = string(types.MessageTypeVoice)
	request.AttachmentIds = []string{"voice-1"}
	request.Payload = mustStruct(map[string]any{"duration_ms": float64(3200)})

	if _, err := server.SendMessage(context.Background(), request); err != nil {
		t.Fatalf("send voice attachment message: %v", err)
	}
	if executor.command.MessageType != types.MessageTypeVoice ||
		len(executor.command.AttachmentIDs) != 1 ||
		executor.command.AttachmentIDs[0] != "voice-1" {
		t.Fatalf("unexpected voice command: %+v", executor.command)
	}
}

func TestSendMessageAcceptsLocationAndCardMessages(t *testing.T) {
	for _, tc := range []struct {
		name        string
		messageType types.MessageType
		payload     map[string]any
	}{
		{
			name:        "location",
			messageType: types.MessageTypeLocation,
			payload: map[string]any{
				"latitude":  float64(31.2304),
				"longitude": float64(121.4737),
				"label":     "Shanghai",
			},
		},
		{
			name:        "card",
			messageType: types.MessageTypeCard,
			payload: map[string]any{
				"card_type": "contact",
				"user_id":   "user-2",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor := &fakeSendMessageExecutor{
				result: types.SendMessageResult{
					MessageID:       types.MessageID("msg-" + tc.name),
					ConversationID:  "conv-1",
					ConversationSeq: 13,
					AcceptedAt:      time.Unix(100, 0).UTC(),
				},
			}
			server := NewServer(executor)
			request := testSendMessageRequest()
			request.MessageType = string(tc.messageType)
			request.Payload = mustStruct(tc.payload)

			if _, err := server.SendMessage(context.Background(), request); err != nil {
				t.Fatalf("send %s message: %v", tc.messageType, err)
			}
			if executor.command.MessageType != tc.messageType {
				t.Fatalf("unexpected command: %+v", executor.command)
			}
		})
	}
}

func TestSendMessageRejectsAttachmentMessageWithoutAttachments(t *testing.T) {
	server := NewServer(&fakeSendMessageExecutor{})
	request := testSendMessageRequest()
	request.MessageType = string(types.MessageTypeVoice)
	request.AttachmentIds = nil

	_, err := server.SendMessage(context.Background(), request)
	assertStatusDetail(
		t,
		err,
		codes.InvalidArgument,
		messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED,
		false,
		"request-1",
	)
}

func TestSendMessageMapsUseCaseErrors(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		grpcCode  codes.Code
		errorCode messagev1.MessageErrorCode
		retryable bool
	}{
		{
			name:      "permission denied",
			err:       types.NewPermissionDenied("blocked"),
			grpcCode:  codes.PermissionDenied,
			errorCode: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_PERMISSION_DENIED,
		},
		{
			name:      "idempotency conflict",
			err:       types.NewIdempotencyConflict("different command"),
			grpcCode:  codes.Aborted,
			errorCode: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_IDEMPOTENCY_CONFLICT,
		},
		{
			name:      "conversation not found",
			err:       types.NewConversationNotFound("missing"),
			grpcCode:  codes.NotFound,
			errorCode: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_CONVERSATION_NOT_FOUND,
		},
		{
			name:      "db write failed",
			err:       types.NewDBWriteFailed("deadlock"),
			grpcCode:  codes.Unavailable,
			errorCode: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_DB_WRITE_FAILED,
			retryable: true,
		},
		{
			name:      "outbox write failed",
			err:       types.NewOutboxWriteFailed("constraint"),
			grpcCode:  codes.Unavailable,
			errorCode: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_OUTBOX_WRITE_FAILED,
			retryable: true,
		},
		{
			name:      "service overloaded",
			err:       types.NewServiceOverloaded("pg pool saturated"),
			grpcCode:  codes.Unavailable,
			errorCode: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SERVICE_OVERLOADED,
			retryable: true,
		},
		{
			name:      "dependency version mismatch",
			err:       types.NewDependencyVersionMismatch("version drift"),
			grpcCode:  codes.Unavailable,
			errorCode: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED,
			retryable: true,
		},
		{
			name:      "unknown",
			err:       errors.New("unexpected"),
			grpcCode:  codes.Internal,
			errorCode: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(&fakeSendMessageExecutor{err: tc.err})

			_, err := server.SendMessage(context.Background(), testSendMessageRequest())
			assertStatusDetail(t, err, tc.grpcCode, tc.errorCode, tc.retryable, "request-1")
		})
	}
}

func TestSendMessageSanitizesInternalErrorMessages(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		publicMessage string
	}{
		{
			name:          "database",
			err:           types.NewDBWriteFailed(`pq: duplicate key value violates unique constraint "message_log_tenant_id_sender_id_device_id_client_msg_id_key"`),
			publicMessage: "database write failed",
		},
		{
			name:          "outbox",
			err:           types.NewOutboxWriteFailed(`insert message_outbox failed: relation "message_outbox" does not exist`),
			publicMessage: "outbox write failed",
		},
		{
			name:          "unknown",
			err:           errors.New("panic: internal stack trace"),
			publicMessage: "message service internal error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(&fakeSendMessageExecutor{err: tc.err})

			_, err := server.SendMessage(context.Background(), testSendMessageRequest())
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected status error, got %v", err)
			}
			if st.Message() != tc.publicMessage {
				t.Fatalf("unexpected status message %q", st.Message())
			}
			details := st.Details()
			if len(details) != 1 {
				t.Fatalf("expected one error detail, got %d", len(details))
			}
			detail, ok := details[0].(*messagev1.MessageError)
			if !ok {
				t.Fatalf("expected MessageError detail, got %T", details[0])
			}
			if detail.GetMessage() != tc.publicMessage {
				t.Fatalf("unexpected detail message %q", detail.GetMessage())
			}
		})
	}
}

func TestSendMessageServiceOverloadedIncludesRetryInfo(t *testing.T) {
	server := NewServer(&fakeSendMessageExecutor{err: types.NewServiceOverloaded("pg pool saturated")})

	_, err := server.SendMessage(context.Background(), testSendMessageRequest())
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Fatalf("expected unavailable, got %s", st.Code())
	}
	var retryInfo *errdetails.RetryInfo
	for _, detail := range st.Details() {
		if candidate, ok := detail.(*errdetails.RetryInfo); ok {
			retryInfo = candidate
		}
	}
	if retryInfo == nil {
		t.Fatalf("expected RetryInfo detail, got %+v", st.Details())
	}
	if retryInfo.GetRetryDelay().AsDuration() != serviceOverloadedRetryDelay {
		t.Fatalf("unexpected retry delay: %s", retryInfo.GetRetryDelay().AsDuration())
	}
}

func TestSendMessageServiceOverloadedUsesDynamicRetryInfo(t *testing.T) {
	server := NewServer(&fakeSendMessageExecutor{
		err: types.NewServiceOverloadedWithRetryDelay("adaptive limit", 1500*time.Millisecond),
	})

	_, err := server.SendMessage(context.Background(), testSendMessageRequest())
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	var retryInfo *errdetails.RetryInfo
	for _, detail := range st.Details() {
		if candidate, ok := detail.(*errdetails.RetryInfo); ok {
			retryInfo = candidate
		}
	}
	if retryInfo == nil {
		t.Fatalf("expected RetryInfo detail, got %+v", st.Details())
	}
	if retryInfo.GetRetryDelay().AsDuration() != 1500*time.Millisecond {
		t.Fatalf("unexpected retry delay: %s", retryInfo.GetRetryDelay().AsDuration())
	}
}

func testSendMessageRequest() *messagev1.SendMessageRequest {
	return &messagev1.SendMessageRequest{
		AuthContext: &messagev1.AuthContext{
			TenantId:  "tenant-1",
			UserId:    "user-1",
			DeviceId:  "device-1",
			SessionId: "session-1",
			TraceId:   "trace-1",
			RequestId: "request-1",
		},
		ConversationId: "conv-1",
		ClientMsgId:    "client-1",
		MessageType:    string(types.MessageTypeText),
		Payload:        mustStruct(map[string]any{"text": "hello"}),
		AttachmentIds:  []string{"att-2", "att-1"},
	}
}

func mustStruct(values map[string]any) *structpb.Struct {
	payload, err := structpb.NewStruct(values)
	if err != nil {
		panic(err)
	}
	return payload
}

func testEditMessageRequest() *messagev1.EditMessageRequest {
	payload, err := structpb.NewStruct(map[string]any{"text": "edited"})
	if err != nil {
		panic(err)
	}
	return &messagev1.EditMessageRequest{
		AuthContext:    testSpoofedAuthContext(),
		ConversationId: "conv-1",
		MessageId:      "msg-1",
		IdempotencyKey: "edit-1",
		Payload:        payload,
		Reason:         "fix typo",
	}
}

func testRevokeMessageRequest() *messagev1.RevokeMessageRequest {
	return &messagev1.RevokeMessageRequest{
		AuthContext:    testSpoofedAuthContext(),
		ConversationId: "conv-1",
		MessageId:      "msg-1",
		IdempotencyKey: "revoke-1",
		Reason:         "sender revoke",
	}
}

func testDeleteMessageRequest() *messagev1.DeleteMessageRequest {
	return &messagev1.DeleteMessageRequest{
		AuthContext:    testSpoofedAuthContext(),
		ConversationId: "conv-1",
		MessageId:      "msg-1",
		IdempotencyKey: "delete-1",
		DeleteScope:    messagev1.DeleteScope_DELETE_SCOPE_CONVERSATION_VIEW,
		Reason:         "delete message",
	}
}

func testSpoofedAuthContext() *messagev1.AuthContext {
	return &messagev1.AuthContext{
		TenantId:  "spoofed-tenant",
		UserId:    "spoofed-user",
		DeviceId:  "spoofed-device",
		SessionId: "spoofed-session",
		TraceId:   "body-trace",
		RequestId: "body-request",
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

func assertStatusDetail(
	t *testing.T,
	err error,
	expectedCode codes.Code,
	expectedErrorCode messagev1.MessageErrorCode,
	expectedRetryable bool,
	expectedCorrelationID string,
) {
	t.Helper()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if st.Code() != expectedCode {
		t.Fatalf("expected grpc code %s, got %s", expectedCode, st.Code())
	}
	details := st.Details()
	var detail *messagev1.MessageError
	for _, candidate := range details {
		if messageError, ok := candidate.(*messagev1.MessageError); ok {
			detail = messageError
			break
		}
	}
	if detail == nil {
		t.Fatalf("expected MessageError detail, got %+v", details)
	}
	if detail.GetCode() != expectedErrorCode ||
		detail.GetRetryable() != expectedRetryable ||
		detail.GetCorrelationId() != expectedCorrelationID {
		t.Fatalf("unexpected error detail: %+v", detail)
	}
}

type fakeSendMessageExecutor struct {
	result  types.SendMessageResult
	err     error
	calls   int
	command types.SendMessageCommand
}

func (f *fakeSendMessageExecutor) Execute(_ context.Context, command types.SendMessageCommand) (types.SendMessageResult, error) {
	f.calls++
	f.command = command
	return f.result, f.err
}

type fakeEditMessageExecutor struct {
	result  types.MessageChangeResult
	err     error
	command types.EditMessageCommand
}

func (f *fakeEditMessageExecutor) Execute(_ context.Context, command types.EditMessageCommand) (types.MessageChangeResult, error) {
	f.command = command
	return f.result, f.err
}

type fakeRevokeMessageExecutor struct {
	result  types.MessageChangeResult
	err     error
	command types.RevokeMessageCommand
}

func (f *fakeRevokeMessageExecutor) Execute(_ context.Context, command types.RevokeMessageCommand) (types.MessageChangeResult, error) {
	f.command = command
	return f.result, f.err
}

type fakeDeleteMessageExecutor struct {
	result  types.MessageChangeResult
	err     error
	command types.DeleteMessageCommand
}

func (f *fakeDeleteMessageExecutor) Execute(_ context.Context, command types.DeleteMessageCommand) (types.MessageChangeResult, error) {
	f.command = command
	return f.result, f.err
}
