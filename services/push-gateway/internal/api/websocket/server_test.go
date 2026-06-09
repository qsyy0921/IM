package websocket

import (
	"context"
	"encoding/json"
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
}

func (client *fakeDeliveryClient) AckDelivery(ctx context.Context, command types.AckDeliveryCommand) (types.AckDeliveryResult, error) {
	client.last = command
	return types.AckDeliveryResult{
		TenantID:        command.AuthContext.TenantID,
		UserID:          command.AuthContext.UserID,
		DeviceID:        command.AuthContext.DeviceID,
		ConversationID:  command.ConversationID,
		LastReceivedSeq: command.ReceivedSeq,
	}, nil
}
