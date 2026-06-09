package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/app"
	"github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/memory"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestDecodeClientFrameRejectsInvalidJSON(t *testing.T) {
	_, _, err := DecodeClientFrame(json.RawMessage(`{`))
	if err == nil {
		t.Fatalf("expected invalid frame")
	}
}

func TestDecodeClientFrameDeliveryAck(t *testing.T) {
	frame, requestID, err := DecodeClientFrame(json.RawMessage(`{"op":"delivery.ack","request_id":"r1","conversation_id":"c1","received_seq":7}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if requestID != "r1" || frame.ConversationID != "c1" || frame.ReceivedSeq != 7 {
		t.Fatalf("unexpected frame: %+v request=%s", frame, requestID)
	}
}

func TestWebSocketPingAndAck(t *testing.T) {
	registry := memory.NewRegistry()
	delivery := &fakeDeliveryClient{}
	server := NewServer(
		app.NewConnectSessionUseCase(registry),
		app.NewDisconnectSessionUseCase(registry),
		app.NewHandleClientFrameUseCase(delivery),
		Config{QueueSize: 8, HeartbeatInterval: time.Second},
	)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := nhooyr.Dial(ctx, "ws"+httpServer.URL[len("http"):]+"/ws?tenant_id=tenant-1&user_id=user-1&device_id=device-1", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(nhooyr.StatusNormalClosure, "")

	if err := wsjson.Write(ctx, conn, types.ClientFrame{
		Op:        types.OpClientHello,
		RequestID: "hello-1",
		DeviceID:  "device-1",
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	var hello types.ServerFrame
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if hello.Op != types.OpServerHello || hello.RequestID != "hello-1" || hello.SessionID == "" {
		t.Fatalf("unexpected hello: %+v", hello)
	}

	if _, err := app.NewNotifyDeliveryUseCase(registry).Execute(ctx, types.NotifyDeliveryCommand{
		Notification: types.DeliveryNotification{
			EventID:         "delivery-event-1",
			TenantID:        "tenant-1",
			UserID:          "user-1",
			ConversationID:  "conversation-1",
			ConversationSeq: 11,
			SourceEventID:   "timeline-event-1",
			SourceEventType: "message.persisted.v1",
			MessageID:       "message-1",
			CorrelationID:   "corr-1",
		},
	}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	var notify types.ServerFrame
	if err := wsjson.Read(ctx, conn, &notify); err != nil {
		t.Fatalf("read notify: %v", err)
	}
	if notify.Op != types.OpDeliveryNotify ||
		notify.EventID != "delivery-event-1" ||
		notify.ConversationSeq != 11 ||
		notify.SourceEventID != "timeline-event-1" ||
		notify.SourceEventType != "message.persisted.v1" ||
		notify.MessageID != "message-1" ||
		!notify.PullRequired {
		t.Fatalf("unexpected notify: %+v", notify)
	}

	if err := wsjson.Write(ctx, conn, types.ClientFrame{Op: types.OpClientPing, RequestID: "ping-1"}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	var pong types.ServerFrame
	if err := wsjson.Read(ctx, conn, &pong); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if pong.Op != types.OpServerPong || pong.RequestID != "ping-1" || pong.ServerTimeMS <= 0 {
		t.Fatalf("unexpected pong: %+v", pong)
	}

	if err := wsjson.Write(ctx, conn, types.ClientFrame{
		Op:             types.OpDeliveryAck,
		RequestID:      "ack-1",
		ConversationID: "conversation-1",
		ReceivedSeq:    12,
	}); err != nil {
		t.Fatalf("write ack: %v", err)
	}
	var ackOK types.ServerFrame
	if err := wsjson.Read(ctx, conn, &ackOK); err != nil {
		t.Fatalf("read ack ok: %v", err)
	}
	if ackOK.Op != types.OpDeliveryAckOK ||
		ackOK.RequestID != "ack-1" ||
		ackOK.ConversationID != "conversation-1" ||
		ackOK.LastReceivedSeq != 12 {
		t.Fatalf("unexpected ack ok: %+v", ackOK)
	}
	if delivery.last.AuthContext.TenantID != "tenant-1" ||
		delivery.last.AuthContext.UserID != "user-1" ||
		delivery.last.AuthContext.DeviceID != "device-1" ||
		delivery.last.AuthContext.SessionID != hello.SessionID {
		t.Fatalf("ack must use session auth context: %+v", delivery.last)
	}
}

func TestErrorFrameIncludesRetryableFalse(t *testing.T) {
	encoded, err := json.Marshal(app.PublicErrorFrame("r1", types.NewInvalidFrame("bad")))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(encoded) || !containsString(string(encoded), `"retryable":false`) {
		t.Fatalf("retryable=false must be explicit: %s", encoded)
	}
}

func containsString(value string, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return false
}

type fakeDeliveryClient struct {
	last types.AckDeliveryCommand
	err  error
}

func (client *fakeDeliveryClient) AckDelivery(ctx context.Context, command types.AckDeliveryCommand) (types.AckDeliveryResult, error) {
	client.last = command
	if client.err != nil {
		return types.AckDeliveryResult{}, client.err
	}
	return types.AckDeliveryResult{
		TenantID:        command.AuthContext.TenantID,
		UserID:          command.AuthContext.UserID,
		DeviceID:        command.AuthContext.DeviceID,
		ConversationID:  command.ConversationID,
		LastReceivedSeq: command.ReceivedSeq,
	}, nil
}

func TestWebSocketAckPermissionDeniedIsNotRetryable(t *testing.T) {
	registry := memory.NewRegistry()
	delivery := &fakeDeliveryClient{err: types.ErrPermissionDenied}
	server := NewServer(
		app.NewConnectSessionUseCase(registry),
		app.NewDisconnectSessionUseCase(registry),
		app.NewHandleClientFrameUseCase(delivery),
		Config{QueueSize: 8, HeartbeatInterval: time.Second},
	)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := nhooyr.Dial(ctx, "ws"+httpServer.URL[len("http"):]+"/ws?tenant_id=tenant-1&user_id=user-1&device_id=device-1", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(nhooyr.StatusNormalClosure, "")

	if err := wsjson.Write(ctx, conn, types.ClientFrame{
		Op:        types.OpClientHello,
		RequestID: "hello-1",
		DeviceID:  "device-1",
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	var hello types.ServerFrame
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		t.Fatalf("read hello: %v", err)
	}

	if err := wsjson.Write(ctx, conn, types.ClientFrame{
		Op:             types.OpDeliveryAck,
		RequestID:      "ack-denied",
		ConversationID: "conversation-1",
		ReceivedSeq:    12,
	}); err != nil {
		t.Fatalf("write ack: %v", err)
	}
	var frame types.ServerFrame
	if err := wsjson.Read(ctx, conn, &frame); err != nil {
		t.Fatalf("read error: %v", err)
	}
	if frame.Op != types.OpError ||
		frame.RequestID != "ack-denied" ||
		frame.Code != "PERMISSION_DENIED" ||
		frame.Message != "permission denied" ||
		frame.Retryable {
		t.Fatalf("unexpected permission error frame: %+v", frame)
	}
}

func TestEnqueueOutboundReturnsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	outbound := make(chan types.ServerFrame)
	cancel()
	if err := enqueueOutbound(ctx, outbound, types.ServerFrame{Op: types.OpServerPong}); err == nil {
		t.Fatalf("expected context cancellation")
	}
}

func TestWebSocketDisconnectUnregistersBeforeFurtherNotify(t *testing.T) {
	registry := memory.NewRegistry()
	server := NewServer(
		app.NewConnectSessionUseCase(registry),
		app.NewDisconnectSessionUseCase(registry),
		app.NewHandleClientFrameUseCase(&fakeDeliveryClient{}),
		Config{QueueSize: 1, HeartbeatInterval: time.Second},
	)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := nhooyr.Dial(ctx, "ws"+httpServer.URL[len("http"):]+"/ws?tenant_id=tenant-1&user_id=user-1&device_id=device-1", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := wsjson.Write(ctx, conn, types.ClientFrame{
		Op:        types.OpClientHello,
		RequestID: "hello-1",
		DeviceID:  "device-1",
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	var hello types.ServerFrame
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if err := conn.Close(nhooyr.StatusNormalClosure, "done"); err != nil {
		t.Fatalf("close client: %v", err)
	}

	notify := app.NewNotifyDeliveryUseCase(registry)
	deadline := time.Now().Add(2 * time.Second)
	for {
		result, err := notify.Execute(ctx, types.NotifyDeliveryCommand{Notification: types.DeliveryNotification{
			EventID:         "delivery-event-disconnect",
			TenantID:        "tenant-1",
			UserID:          "user-1",
			ConversationID:  "conversation-1",
			ConversationSeq: 7,
			SourceEventType: "message.persisted.v1",
		}})
		if err != nil {
			t.Fatalf("notify after disconnect: %v", err)
		}
		if result.MatchedSessions == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session was not unregistered after disconnect: %+v", result)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWebSocketReplaysResumeBuffer(t *testing.T) {
	registry := memory.NewRegistry()
	server := NewServer(
		app.NewConnectSessionUseCase(registry),
		app.NewDisconnectSessionUseCase(registry),
		app.NewHandleClientFrameUseCase(&fakeDeliveryClient{}),
		Config{QueueSize: 4, HeartbeatInterval: time.Second},
	)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	target := "ws" + httpServer.URL[len("http"):] + "/ws?tenant_id=tenant-1&user_id=user-1&device_id=device-1"
	conn, _, err := nhooyr.Dial(ctx, target, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := wsjson.Write(ctx, conn, types.ClientFrame{
		Op:        types.OpClientHello,
		RequestID: "hello-1",
		DeviceID:  "device-1",
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	var hello types.ServerFrame
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if hello.ResumeToken == "" {
		t.Fatalf("expected resume token")
	}
	if _, err := app.NewNotifyDeliveryUseCase(registry).Execute(ctx, types.NotifyDeliveryCommand{
		Notification: types.DeliveryNotification{
			EventID:         "delivery-event-7",
			TenantID:        "tenant-1",
			UserID:          "user-1",
			ConversationID:  "conversation-1",
			ConversationSeq: 7,
			SourceEventType: "message.persisted.v1",
			MessageID:       "message-7",
		},
	}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	var notify types.ServerFrame
	if err := wsjson.Read(ctx, conn, &notify); err != nil {
		t.Fatalf("read notify: %v", err)
	}
	if notify.EventID != "delivery-event-7" {
		t.Fatalf("unexpected notify: %+v", notify)
	}
	_ = conn.Close(nhooyr.StatusNormalClosure, "resume")

	resumed, _, err := nhooyr.Dial(ctx, target, nil)
	if err != nil {
		t.Fatalf("redial: %v", err)
	}
	defer resumed.Close(nhooyr.StatusNormalClosure, "")
	if err := wsjson.Write(ctx, resumed, types.ClientFrame{
		Op:          types.OpClientHello,
		RequestID:   "hello-2",
		DeviceID:    "device-1",
		ResumeToken: hello.ResumeToken,
		LastReceived: []types.ConversationCursor{{
			ConversationID: "conversation-1",
			Seq:            6,
		}},
	}); err != nil {
		t.Fatalf("write resume hello: %v", err)
	}
	var resumedHello types.ServerFrame
	if err := wsjson.Read(ctx, resumed, &resumedHello); err != nil {
		t.Fatalf("read resumed hello: %v", err)
	}
	if resumedHello.ResumeToken != hello.ResumeToken {
		t.Fatalf("expected same resume token, got %+v", resumedHello)
	}
	var replay types.ServerFrame
	if err := wsjson.Read(ctx, resumed, &replay); err != nil {
		t.Fatalf("read replay: %v", err)
	}
	if replay.Op != types.OpDeliveryNotify ||
		replay.EventID != "delivery-event-7" ||
		replay.ConversationSeq != 7 ||
		!replay.PullRequired {
		t.Fatalf("unexpected replay: %+v", replay)
	}
}

func TestWebSocketUnknownResumeTokenIsReplaced(t *testing.T) {
	registry := memory.NewRegistry()
	server := NewServer(
		app.NewConnectSessionUseCase(registry),
		app.NewDisconnectSessionUseCase(registry),
		app.NewHandleClientFrameUseCase(&fakeDeliveryClient{}),
		Config{QueueSize: 4, HeartbeatInterval: time.Second},
	)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	target := "ws" + httpServer.URL[len("http"):] + "/ws?tenant_id=tenant-1&user_id=user-1&device_id=device-1"
	conn, _, err := nhooyr.Dial(ctx, target, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := wsjson.Write(ctx, conn, types.ClientFrame{
		Op:          types.OpClientHello,
		RequestID:   "hello-unknown",
		DeviceID:    "device-1",
		ResumeToken: "client-picked-token",
		LastReceived: []types.ConversationCursor{{
			ConversationID: "conversation-1",
			Seq:            6,
		}},
	}); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	var hello types.ServerFrame
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		t.Fatalf("read hello: %v", err)
	}
	if hello.Op != types.OpServerHello ||
		hello.ResumeToken == "" ||
		hello.ResumeToken == "client-picked-token" {
		t.Fatalf("unknown client token must be replaced: %+v", hello)
	}
	var hint types.ServerFrame
	if err := wsjson.Read(ctx, conn, &hint); err != nil {
		t.Fatalf("read resume hint: %v", err)
	}
	if hint.Op != types.OpResumeHint ||
		hint.Reason != "buffer_miss" ||
		!hint.PullRequired ||
		len(hint.Conversations) != 0 {
		t.Fatalf("unexpected buffer miss hint: %+v", hint)
	}

	if _, err := app.NewNotifyDeliveryUseCase(registry).Execute(ctx, types.NotifyDeliveryCommand{
		Notification: types.DeliveryNotification{
			EventID:         "delivery-event-after-unknown",
			TenantID:        "tenant-1",
			UserID:          "user-1",
			ConversationID:  "conversation-1",
			ConversationSeq: 7,
			SourceEventType: "message.persisted.v1",
			MessageID:       "message-7",
		},
	}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	var notify types.ServerFrame
	if err := wsjson.Read(ctx, conn, &notify); err != nil {
		t.Fatalf("read notify: %v", err)
	}
	if notify.EventID != "delivery-event-after-unknown" {
		t.Fatalf("unexpected notify: %+v", notify)
	}
	_ = conn.Close(nhooyr.StatusNormalClosure, "resume")

	resumed, _, err := nhooyr.Dial(ctx, target, nil)
	if err != nil {
		t.Fatalf("redial: %v", err)
	}
	defer resumed.Close(nhooyr.StatusNormalClosure, "")
	if err := wsjson.Write(ctx, resumed, types.ClientFrame{
		Op:          types.OpClientHello,
		RequestID:   "hello-resume",
		DeviceID:    "device-1",
		ResumeToken: hello.ResumeToken,
		LastReceived: []types.ConversationCursor{{
			ConversationID: "conversation-1",
			Seq:            6,
		}},
	}); err != nil {
		t.Fatalf("write resume hello: %v", err)
	}
	var resumedHello types.ServerFrame
	if err := wsjson.Read(ctx, resumed, &resumedHello); err != nil {
		t.Fatalf("read resumed hello: %v", err)
	}
	if resumedHello.ResumeToken != hello.ResumeToken {
		t.Fatalf("expected server token reuse, got %+v", resumedHello)
	}
	var replay types.ServerFrame
	if err := wsjson.Read(ctx, resumed, &replay); err != nil {
		t.Fatalf("read replay: %v", err)
	}
	if replay.Op != types.OpDeliveryNotify ||
		replay.EventID != "delivery-event-after-unknown" ||
		replay.ConversationSeq != 7 {
		t.Fatalf("unexpected replay: %+v", replay)
	}
}

func TestWriteLoopSendsResumeHintAndClosesOnEviction(t *testing.T) {
	outbound := make(chan types.ServerFrame)
	evicted := make(chan types.SessionEviction, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := nhooyr.Accept(writer, request, &nhooyr.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.Close(nhooyr.StatusNormalClosure, "")
		evicted <- types.SessionEviction{
			Reason: "slow_session",
			Conversations: []types.ConversationCursor{{
				ConversationID: "conversation-1",
				Seq:            12,
			}},
		}
		_ = writeLoop(request.Context(), conn, outbound, evicted, time.Second, 0)
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := nhooyr.Dial(ctx, "ws"+httpServer.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(nhooyr.StatusNormalClosure, "")

	var frame types.ServerFrame
	if err := wsjson.Read(ctx, conn, &frame); err != nil {
		t.Fatalf("read resume hint: %v", err)
	}
	if frame.Op != types.OpResumeHint ||
		frame.Reason != "slow_session" ||
		!frame.PullRequired ||
		len(frame.Conversations) != 1 ||
		frame.Conversations[0].ConversationID != "conversation-1" ||
		frame.Conversations[0].Seq != 12 {
		t.Fatalf("unexpected resume hint: %+v", frame)
	}
	var next types.ServerFrame
	err = wsjson.Read(ctx, conn, &next)
	if err == nil {
		t.Fatalf("expected close after resume hint, got frame: %+v", next)
	}
	if nhooyr.CloseStatus(err) != nhooyr.StatusPolicyViolation {
		t.Fatalf("expected policy violation close, got %v", err)
	}
}
