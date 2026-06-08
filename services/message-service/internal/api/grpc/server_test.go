package grpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
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

func TestSendMessageRejectsInvalidRequest(t *testing.T) {
	server := NewServer(&fakeSendMessageExecutor{})

	_, err := server.SendMessage(context.Background(), &messagev1.SendMessageRequest{})
	assertStatusDetail(t, err, codes.InvalidArgument, messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_UNSPECIFIED, false, "")
}

func TestSendMessageRejectsUnsupportedMessageType(t *testing.T) {
	server := NewServer(&fakeSendMessageExecutor{})
	request := testSendMessageRequest()
	request.MessageType = "IMAGE"

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

func testSendMessageRequest() *messagev1.SendMessageRequest {
	payload, err := structpb.NewStruct(map[string]any{"text": "hello"})
	if err != nil {
		panic(err)
	}
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
		Payload:        payload,
		AttachmentIds:  []string{"att-2", "att-1"},
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
	if len(details) != 1 {
		t.Fatalf("expected one error detail, got %d", len(details))
	}
	detail, ok := details[0].(*messagev1.MessageError)
	if !ok {
		t.Fatalf("expected MessageError detail, got %T", details[0])
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
