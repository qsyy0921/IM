package memory

import (
	"context"
	"testing"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

func TestRegistryEnqueueNotificationDeduplicatesPerSession(t *testing.T) {
	registry := NewRegistry()
	outbound := make(chan types.ServerFrame, 2)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	notification := testNotification()
	first, err := registry.EnqueueNotification(context.Background(), notification)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := registry.EnqueueNotification(context.Background(), notification)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if first.Enqueued != 1 || second.Enqueued != 0 || len(outbound) != 1 {
		t.Fatalf("unexpected first=%+v second=%+v queue=%d", first, second, len(outbound))
	}
}

func TestRegistryEnqueueNotificationFailsClosedWhenQueueFull(t *testing.T) {
	registry := NewRegistry()
	outbound := make(chan types.ServerFrame)
	evicted := make(chan types.SessionEviction, 1)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		Outbound:    outbound,
		Evicted:     evicted,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	result, err := registry.EnqueueNotification(context.Background(), testNotification())
	if err != nil {
		t.Fatalf("queue full should evict session and let consumer commit: result=%+v err=%v", result, err)
	}
	if result.Dropped != 1 || result.Enqueued != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	select {
	case eviction := <-evicted:
		if eviction.Reason != "slow_session" ||
			len(eviction.Conversations) != 0 {
			t.Fatalf("unexpected eviction: %+v", eviction)
		}
	default:
		t.Fatalf("expected eviction signal")
	}
	next, err := registry.EnqueueNotification(context.Background(), testNotification())
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if next.MatchedSessions != 0 {
		t.Fatalf("full queue session should have been evicted: %+v", next)
	}
}

func TestRegistryReplaysResumeBufferAfterLastReceived(t *testing.T) {
	registry := NewRegistry()
	outbound := make(chan types.ServerFrame, 4)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		ResumeToken: "resume-1",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	first := testNotification()
	first.EventID = "delivery-event-1"
	first.ConversationSeq = 7
	second := testNotification()
	second.EventID = "delivery-event-2"
	second.ConversationSeq = 8
	if _, err := registry.EnqueueNotification(context.Background(), first); err != nil {
		t.Fatalf("first notify: %v", err)
	}
	if _, err := registry.EnqueueNotification(context.Background(), second); err != nil {
		t.Fatalf("second notify: %v", err)
	}
	registry.Unregister("session-1")

	resumedOutbound := make(chan types.ServerFrame, 2)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-2",
		ResumeToken: "resume-1",
		LastReceived: []types.ConversationCursor{{
			ConversationID: "conversation-1",
			Seq:            7,
		}},
		Outbound: resumedOutbound,
	}); err != nil {
		t.Fatalf("resume register: %v", err)
	}
	if len(resumedOutbound) != 1 {
		t.Fatalf("expected one replayed frame, got %d", len(resumedOutbound))
	}
	replayed := <-resumedOutbound
	if replayed.Op != types.OpDeliveryNotify ||
		replayed.EventID != "delivery-event-2" ||
		replayed.ConversationSeq != 8 {
		t.Fatalf("unexpected replay: %+v", replayed)
	}
}

func TestRegistryRejectsResumeTokenForDifferentDevice(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:   "session-1",
		ResumeToken: "resume-1",
		Outbound:    make(chan types.ServerFrame, 1),
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	_, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-2"},
		SessionID:   "session-2",
		ResumeToken: "resume-1",
		Outbound:    make(chan types.ServerFrame, 1),
	})
	if err != types.ErrPermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestRegistryResumeUnknownTokenReturnsBufferMissHint(t *testing.T) {
	registry := NewRegistry()
	outbound := make(chan types.ServerFrame, 2)
	registration, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext:     types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:       "session-1",
		ResumeToken:     "resume-missing",
		ResumeRequested: true,
		LastReceived:    []types.ConversationCursor{{ConversationID: "conversation-1", Seq: 7}},
		Outbound:        outbound,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if registration.ResumeToken == "" || registration.ResumeToken == "resume-missing" {
		t.Fatalf("unknown client token must be replaced by server token: %+v", registration)
	}
	if len(outbound) != 1 {
		t.Fatalf("expected resume hint")
	}
	hint := <-outbound
	if hint.Op != types.OpResumeHint || hint.Reason != "buffer_miss" || !hint.PullRequired || len(hint.Conversations) != 0 {
		t.Fatalf("unexpected hint: %+v", hint)
	}

	notification := testNotification()
	notification.EventID = "delivery-event-after-miss"
	notification.ConversationSeq = 9
	if _, err := registry.EnqueueNotification(context.Background(), notification); err != nil {
		t.Fatalf("notify: %v", err)
	}
	<-outbound
	registry.Unregister("session-1")

	oldTokenOutbound := make(chan types.ServerFrame, 2)
	oldRegistration, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext:     types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:       "session-old-token",
		ResumeToken:     "resume-missing",
		ResumeRequested: true,
		LastReceived:    []types.ConversationCursor{{ConversationID: "conversation-1", Seq: 8}},
		Outbound:        oldTokenOutbound,
	})
	if err != nil {
		t.Fatalf("old token register: %v", err)
	}
	if oldRegistration.ResumeToken == "resume-missing" || oldRegistration.ResumeToken == registration.ResumeToken {
		t.Fatalf("old unknown token must not be reused: old=%+v first=%+v", oldRegistration, registration)
	}
	if len(oldTokenOutbound) != 1 {
		t.Fatalf("old unknown token should receive only buffer miss, got %d", len(oldTokenOutbound))
	}
	oldHint := <-oldTokenOutbound
	if oldHint.Op != types.OpResumeHint || oldHint.Reason != "buffer_miss" {
		t.Fatalf("unexpected old token hint: %+v", oldHint)
	}
	registry.Unregister("session-old-token")

	newTokenOutbound := make(chan types.ServerFrame, 2)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext:  types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"},
		SessionID:    "session-new-token",
		ResumeToken:  registration.ResumeToken,
		LastReceived: []types.ConversationCursor{{ConversationID: "conversation-1", Seq: 8}},
		Outbound:     newTokenOutbound,
	}); err != nil {
		t.Fatalf("new token register: %v", err)
	}
	if len(newTokenOutbound) != 1 {
		t.Fatalf("new server token should replay one frame, got %d", len(newTokenOutbound))
	}
	replay := <-newTokenOutbound
	if replay.Op != types.OpDeliveryNotify || replay.EventID != "delivery-event-after-miss" {
		t.Fatalf("unexpected replay: %+v", replay)
	}
}

func TestRegistryResumeGapReturnsBufferMissHint(t *testing.T) {
	registry := NewRegistry()
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		ResumeToken: "resume-1",
		Outbound:    make(chan types.ServerFrame, 2),
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	notification := testNotification()
	notification.EventID = "delivery-event-10"
	notification.ConversationSeq = 10
	if _, err := registry.EnqueueNotification(context.Background(), notification); err != nil {
		t.Fatalf("notify: %v", err)
	}
	registry.Unregister("session-1")

	outbound := make(chan types.ServerFrame, 2)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext:     auth,
		SessionID:       "session-2",
		ResumeToken:     "resume-1",
		ResumeRequested: true,
		LastReceived:    []types.ConversationCursor{{ConversationID: "conversation-1", Seq: 7}},
		Outbound:        outbound,
	}); err != nil {
		t.Fatalf("resume register: %v", err)
	}
	if len(outbound) != 1 {
		t.Fatalf("expected only resume hint, got %d frames", len(outbound))
	}
	hint := <-outbound
	if hint.Op != types.OpResumeHint || hint.Reason != "buffer_miss" {
		t.Fatalf("unexpected hint: %+v", hint)
	}
}

func testNotification() types.DeliveryNotification {
	return types.DeliveryNotification{
		EventID:         "delivery-event-1",
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ConversationID:  "conversation-1",
		ConversationSeq: 7,
		SourceEventID:   "timeline-event-1",
		MessageID:       "message-1",
	}
}
