package memory

import (
	"context"
	"testing"
	"time"

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

func TestRegistryConversationSignalRequiresSubscription(t *testing.T) {
	registry := NewRegistry()
	subscribedOutbound := make(chan types.ServerFrame, 2)
	unsubscribedOutbound := make(chan types.ServerFrame, 2)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1", SessionID: "session-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		Outbound:    subscribedOutbound,
	}); err != nil {
		t.Fatalf("register subscribed: %v", err)
	}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-2", DeviceID: "device-2", SessionID: "session-2"},
		SessionID:   "session-2",
		Outbound:    unsubscribedOutbound,
	}); err != nil {
		t.Fatalf("register unsubscribed: %v", err)
	}
	if _, err := registry.SubscribeConversation(context.Background(), types.ConversationSubscriptionCommand{
		AuthContext:    auth,
		ConversationID: "conversation-1",
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	notification := testNotification()
	notification.Kind = types.DeliveryNotificationKindConversationSignal
	notification.UserID = ""
	result, err := registry.EnqueueConversationSignal(context.Background(), notification)
	if err != nil {
		t.Fatalf("conversation signal: %v", err)
	}
	if result.MatchedSessions != 1 || result.Enqueued != 1 || len(subscribedOutbound) != 1 || len(unsubscribedOutbound) != 0 {
		t.Fatalf("unexpected signal result=%+v subscribed=%d unsubscribed=%d", result, len(subscribedOutbound), len(unsubscribedOutbound))
	}
	frame := <-subscribedOutbound
	if frame.Op != types.OpDeliveryNotify ||
		frame.ConversationID != "conversation-1" ||
		frame.ConversationSeq != 7 ||
		!frame.PullRequired {
		t.Fatalf("unexpected signal frame: %+v", frame)
	}
	if _, err := registry.UnsubscribeConversation(context.Background(), types.ConversationSubscriptionCommand{
		AuthContext:    auth,
		ConversationID: "conversation-1",
	}); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	notification.EventID = "delivery-event-2"
	result, err = registry.EnqueueConversationSignal(context.Background(), notification)
	if err != nil {
		t.Fatalf("conversation signal after unsubscribe: %v", err)
	}
	if result.MatchedSessions != 0 || result.Enqueued != 0 {
		t.Fatalf("unexpected signal after unsubscribe: %+v", result)
	}
}

func TestRegistryEvictDeviceClosesMatchingSessions(t *testing.T) {
	registry := NewRegistry()
	firstEvicted := make(chan types.SessionEviction, 1)
	secondEvicted := make(chan types.SessionEviction, 1)
	otherEvicted := make(chan types.SessionEviction, 1)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     firstEvicted,
	}); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-2",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     secondEvicted,
	}); err != nil {
		t.Fatalf("register second: %v", err)
	}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-2"},
		SessionID:   "session-3",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     otherEvicted,
	}); err != nil {
		t.Fatalf("register other: %v", err)
	}

	result, err := registry.EvictDevice(context.Background(), "tenant-1", "user-1", "device-1", "identity_revoked")
	if err != nil {
		t.Fatalf("evict device: %v", err)
	}
	if result.MatchedSessions != 2 || result.Evicted != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertEvictionReason(t, firstEvicted, "identity_revoked")
	assertEvictionReason(t, secondEvicted, "identity_revoked")
	select {
	case eviction := <-otherEvicted:
		t.Fatalf("other device must not be evicted: %+v", eviction)
	default:
	}
	if metrics := registry.Metrics(); metrics.IdentitySessionEvictedCount != 2 || metrics.ConnectedSessions != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestRegistryEvictSessionClosesOnlyMatchingSession(t *testing.T) {
	registry := NewRegistry()
	targetEvicted := make(chan types.SessionEviction, 1)
	otherEvicted := make(chan types.SessionEviction, 1)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     targetEvicted,
	}); err != nil {
		t.Fatalf("register target: %v", err)
	}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-2",
		Outbound:    make(chan types.ServerFrame, 1),
		Evicted:     otherEvicted,
	}); err != nil {
		t.Fatalf("register other: %v", err)
	}

	result, err := registry.EvictSession(context.Background(), "tenant-1", "user-1", "device-1", "session-1", "identity_revoked")
	if err != nil {
		t.Fatalf("evict session: %v", err)
	}
	if result.MatchedSessions != 1 || result.Evicted != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertEvictionReason(t, targetEvicted, "identity_revoked")
	select {
	case eviction := <-otherEvicted:
		t.Fatalf("other session must not be evicted: %+v", eviction)
	default:
	}
	if metrics := registry.Metrics(); metrics.IdentitySessionEvictedCount != 1 || metrics.ConnectedSessions != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
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

func TestRegistryReplaysHiddenFrameAtAlreadyReceivedSeq(t *testing.T) {
	registry := NewRegistry()
	outbound := make(chan types.ServerFrame, 2)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		ResumeToken: "resume-1",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	hidden := testNotification()
	hidden.Kind = types.DeliveryNotificationKindInboxItemHidden
	hidden.EventID = "delivery-event-hide-1"
	hidden.SourceEventType = "delivery.inbox_item.hidden.v1"
	if _, err := registry.EnqueueNotification(context.Background(), hidden); err != nil {
		t.Fatalf("hidden notify: %v", err)
	}
	registry.Unregister("session-1")

	resumedOutbound := make(chan types.ServerFrame, 1)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-2",
		ResumeToken: "resume-1",
		LastReceived: []types.ConversationCursor{{
			ConversationID: hidden.ConversationID,
			Seq:            hidden.ConversationSeq,
		}},
		Outbound: resumedOutbound,
	}); err != nil {
		t.Fatalf("resume register: %v", err)
	}
	select {
	case replayed := <-resumedOutbound:
		if replayed.Op != types.OpDeliveryHide || replayed.EventID != "delivery-event-hide-1" {
			t.Fatalf("unexpected replay: %+v", replayed)
		}
	default:
		t.Fatalf("expected hidden frame replay despite last_received cursor")
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

func TestRegistryResumeDoesNotPartiallyReplayWhenOutboundCapacityIsInsufficient(t *testing.T) {
	registry := NewRegistry()
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	onlineOutbound := make(chan types.ServerFrame, 4)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		ResumeToken: "resume-1",
		Outbound:    onlineOutbound,
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

	resumedOutbound := make(chan types.ServerFrame, 1)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext:     auth,
		SessionID:       "session-2",
		ResumeToken:     "resume-1",
		ResumeRequested: true,
		LastReceived: []types.ConversationCursor{{
			ConversationID: "conversation-1",
			Seq:            6,
		}},
		Outbound: resumedOutbound,
	}); err != nil {
		t.Fatalf("resume register: %v", err)
	}
	if len(resumedOutbound) != 1 {
		t.Fatalf("expected one broad buffer miss hint, got %d frames", len(resumedOutbound))
	}
	hint := <-resumedOutbound
	if hint.Op != types.OpResumeHint || hint.Reason != "buffer_miss" || !hint.PullRequired {
		t.Fatalf("unexpected frame after insufficient replay capacity: %+v", hint)
	}
	if hint.EventID == first.EventID || hint.EventID == second.EventID {
		t.Fatalf("resume must not partially replay delivery notify when capacity is insufficient: %+v", hint)
	}
	metrics := registry.Metrics()
	if metrics.ResumeBufferReplayCount != 0 || metrics.ResumeBufferMissCount != 1 {
		t.Fatalf("unexpected resume metrics after insufficient capacity: %+v", metrics)
	}
}

func TestRegistryExpiredResumeTokenReturnsBufferMissAndNewToken(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	registry := NewRegistryWithConfig(Config{
		ResumeBufferTTL: time.Minute,
		Now: func() time.Time {
			return now
		},
	})
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	outbound := make(chan types.ServerFrame, 4)
	registration, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		ResumeToken: "resume-1",
		Outbound:    outbound,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	notification := testNotification()
	notification.EventID = "delivery-event-expiring"
	notification.ConversationSeq = 11
	if _, err := registry.EnqueueNotification(context.Background(), notification); err != nil {
		t.Fatalf("notify: %v", err)
	}
	<-outbound
	registry.Unregister("session-1")

	now = now.Add(time.Minute)
	resumedOutbound := make(chan types.ServerFrame, 2)
	resumedRegistration, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext:     auth,
		SessionID:       "session-2",
		ResumeToken:     registration.ResumeToken,
		ResumeRequested: true,
		LastReceived:    []types.ConversationCursor{{ConversationID: "conversation-1", Seq: 10}},
		Outbound:        resumedOutbound,
	})
	if err != nil {
		t.Fatalf("resume register: %v", err)
	}
	if resumedRegistration.ResumeToken == "" || resumedRegistration.ResumeToken == registration.ResumeToken {
		t.Fatalf("expired resume token must be replaced: first=%+v resumed=%+v", registration, resumedRegistration)
	}
	if len(resumedOutbound) != 1 {
		t.Fatalf("expected only buffer miss hint after expiration, got %d frames", len(resumedOutbound))
	}
	hint := <-resumedOutbound
	if hint.Op != types.OpResumeHint || hint.Reason != "buffer_miss" {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if metrics := registry.Metrics(); metrics.ResumeBufferExpiredCount == 0 {
		t.Fatalf("expected expired token metric, got %+v", metrics)
	}
}

func TestRegistryMetricsPrunesExpiredResumeTokens(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	registry := NewRegistryWithConfig(Config{
		ResumeBufferTTL: time.Minute,
		Now: func() time.Time {
			return now
		},
	})
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		ResumeToken: "resume-1",
		Outbound:    make(chan types.ServerFrame, 1),
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if metrics := registry.Metrics(); metrics.ResumeBufferTokenCount != 1 {
		t.Fatalf("expected one token before expiry, got %+v", metrics)
	}
	registry.Unregister("session-1")
	now = now.Add(time.Minute)
	metrics := registry.Metrics()
	if metrics.ResumeBufferTokenCount != 0 || metrics.ResumeBufferExpiredCount == 0 {
		t.Fatalf("expected metrics to prune expired token, got %+v", metrics)
	}
}

func TestRegistryDoesNotExpireActiveSessionResumeToken(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	registry := NewRegistryWithConfig(Config{
		ResumeBufferTTL: time.Minute,
		Now: func() time.Time {
			return now
		},
	})
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	outbound := make(chan types.ServerFrame, 2)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		ResumeToken: "resume-1",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	now = now.Add(time.Minute)
	if metrics := registry.Metrics(); metrics.ResumeBufferTokenCount != 1 || metrics.ResumeBufferExpiredCount != 0 {
		t.Fatalf("active session token should not expire, got %+v", metrics)
	}
	notification := testNotification()
	notification.EventID = "delivery-event-active-token"
	notification.ConversationSeq = 12
	if _, err := registry.EnqueueNotification(context.Background(), notification); err != nil {
		t.Fatalf("notify: %v", err)
	}
	<-outbound
	registry.Unregister("session-1")

	resumedOutbound := make(chan types.ServerFrame, 2)
	if _, err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext:  auth,
		SessionID:    "session-2",
		ResumeToken:  "resume-1",
		LastReceived: []types.ConversationCursor{{ConversationID: "conversation-1", Seq: 11}},
		Outbound:     resumedOutbound,
	}); err != nil {
		t.Fatalf("resume register: %v", err)
	}
	if len(resumedOutbound) != 1 {
		t.Fatalf("expected replay from active token, got %d frames", len(resumedOutbound))
	}
	replay := <-resumedOutbound
	if replay.Op != types.OpDeliveryNotify || replay.EventID != "delivery-event-active-token" {
		t.Fatalf("unexpected replay: %+v", replay)
	}
}

func assertEvictionReason(t *testing.T, evicted <-chan types.SessionEviction, reason string) {
	t.Helper()
	select {
	case eviction := <-evicted:
		if eviction.Reason != reason {
			t.Fatalf("expected reason %q, got %+v", reason, eviction)
		}
	default:
		t.Fatalf("expected eviction")
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
		SourceEventType: "message.persisted.v1",
		MessageID:       "message-1",
	}
}
