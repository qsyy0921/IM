package memory

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"sync"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type Config struct {
	ResumeBufferTTL           time.Duration
	ConversationFanoutBuckets int
	ConversationSignalPolicy  types.ConversationSignalPolicy
	Now                       func() time.Time
}

type Registry struct {
	mu             sync.RWMutex
	sessions       map[string]*session
	byUser         map[string]map[string]struct{}
	byConversation map[string]map[string]struct{}
	resumes        map[string]*resumeState
	metrics        Metrics
	config         Config
}

type Metrics struct {
	ConnectedSessions                        int    `json:"connected_sessions"`
	SessionQueueFullCount                    uint64 `json:"session_queue_full_count"`
	SlowSessionEvictedCount                  uint64 `json:"slow_session_evicted_count"`
	IdentitySessionEvictedCount              uint64 `json:"identity_session_evicted_count"`
	ConversationSubscriptionCount            int    `json:"conversation_subscription_count"`
	ConversationSignalMatchedCount           uint64 `json:"conversation_signal_matched_count"`
	ConversationSignalEnqueuedCount          uint64 `json:"conversation_signal_enqueued_count"`
	ConversationSignalSuppressedEventCount   uint64 `json:"conversation_signal_suppressed_event_count"`
	ConversationSignalSuppressedSessionCount uint64 `json:"conversation_signal_suppressed_session_count"`
	ResumeBufferReplayCount                  uint64 `json:"resume_buffer_replay_count"`
	ResumeBufferMissCount                    uint64 `json:"resume_buffer_miss_count"`
	ResumeBufferStoredFrames                 int    `json:"resume_buffer_stored_frames"`
	ResumeBufferTokenCount                   int    `json:"resume_buffer_token_count"`
	ResumeBufferExpiredCount                 uint64 `json:"resume_buffer_expired_count"`
}

type session struct {
	auth          types.AuthContext
	resumeToken   string
	outbound      chan<- types.ServerFrame
	evicted       chan<- types.SessionEviction
	seen          map[string]struct{}
	conversations map[string]struct{}
}

type outboundTarget struct {
	sessionID string
	session   *session
	outbound  chan<- types.ServerFrame
}

type resumeState struct {
	auth      types.AuthContext
	frames    []types.ServerFrame
	expiresAt time.Time
}

func NewRegistry() *Registry {
	return NewRegistryWithConfig(Config{})
}

func NewRegistryWithConfig(config Config) *Registry {
	if config.ResumeBufferTTL <= 0 {
		config.ResumeBufferTTL = types.DefaultResumeBufferTTL
	}
	if config.ConversationFanoutBuckets <= 0 {
		config.ConversationFanoutBuckets = 1
	}
	config.ConversationSignalPolicy = types.NormalizeConversationSignalPolicy(config.ConversationSignalPolicy)
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Registry{
		sessions:       make(map[string]*session),
		byUser:         make(map[string]map[string]struct{}),
		byConversation: make(map[string]map[string]struct{}),
		resumes:        make(map[string]*resumeState),
		config:         config,
	}
}

func (registry *Registry) Register(
	ctx context.Context,
	registration types.SessionRegistration,
) (types.SessionRegistrationResult, error) {
	if registration.SessionID == "" || registration.Outbound == nil {
		return types.SessionRegistrationResult{}, types.NewInvalidFrame("session registration is incomplete")
	}
	if err := registration.AuthContext.Validate(); err != nil {
		return types.SessionRegistrationResult{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	now := registry.config.Now()
	registry.pruneExpiredResumesLocked(now)

	var state *resumeState
	bufferMiss := false
	effectiveResumeToken := registration.ResumeToken
	if registration.ResumeToken != "" {
		var ok bool
		state, ok = registry.resumes[registration.ResumeToken]
		if ok && !sameDevice(state.auth, registration.AuthContext) {
			return types.SessionRegistrationResult{}, types.ErrPermissionDenied
		}
		if !ok {
			bufferMiss = registration.ResumeRequested
			if registration.ResumeRequested {
				effectiveResumeToken = domain.NewOpaqueID("resume")
			}
			state = &resumeState{
				auth:      registration.AuthContext,
				expiresAt: registry.resumeExpiresAt(now),
			}
			registry.resumes[effectiveResumeToken] = state
		}
	}
	if state != nil {
		state.expiresAt = registry.resumeExpiresAt(now)
	}
	if previous, ok := registry.sessions[registration.SessionID]; ok {
		delete(registry.byUser[userKey(previous.auth)], registration.SessionID)
		registry.removeConversationSubscriptionsLocked(registration.SessionID, previous)
	}
	registry.sessions[registration.SessionID] = &session{
		auth:          registration.AuthContext,
		resumeToken:   effectiveResumeToken,
		outbound:      registration.Outbound,
		evicted:       registration.Evicted,
		seen:          make(map[string]struct{}),
		conversations: make(map[string]struct{}),
	}
	if state != nil {
		if bufferMiss || registry.replayLocked(registration, state) {
			registry.enqueueResumeHintLocked(registration.Outbound)
			registry.metrics.ResumeBufferMissCount++
		}
	}
	key := userKey(registration.AuthContext)
	if registry.byUser[key] == nil {
		registry.byUser[key] = make(map[string]struct{})
	}
	registry.byUser[key][registration.SessionID] = struct{}{}
	return types.SessionRegistrationResult{ResumeToken: effectiveResumeToken}, nil
}

func (registry *Registry) Unregister(sessionID string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	existing, ok := registry.sessions[sessionID]
	if !ok {
		return
	}
	delete(registry.sessions, sessionID)
	key := userKey(existing.auth)
	delete(registry.byUser[key], sessionID)
	if len(registry.byUser[key]) == 0 {
		delete(registry.byUser, key)
	}
	registry.removeConversationSubscriptionsLocked(sessionID, existing)
}

func (registry *Registry) EnqueueNotification(
	ctx context.Context,
	notification types.DeliveryNotification,
) (types.NotifyDeliveryResult, error) {
	key := notification.TenantID + "\x1f" + notification.UserID
	registry.mu.Lock()
	registry.pruneExpiredResumesLocked(registry.config.Now())

	sessionIDs := registry.byUser[key]
	result := types.NotifyDeliveryResult{MatchedSessions: len(sessionIDs)}
	if len(sessionIDs) == 0 {
		registry.mu.Unlock()
		return result, nil
	}
	frame, err := encodedDeliveryFrame(notification)
	if err != nil {
		registry.mu.Unlock()
		return result, err
	}
	targets := registry.collectOutboundTargetsLocked(sessionIDs, notification.EventID, frame)
	registry.mu.Unlock()
	return registry.enqueueOutboundTargets(ctx, frame, result, targets)
}

func (registry *Registry) SubscribeConversation(
	ctx context.Context,
	command types.ConversationSubscriptionCommand,
) (types.ConversationSubscriptionResult, error) {
	if err := command.AuthContext.Validate(); err != nil {
		return types.ConversationSubscriptionResult{}, err
	}
	if command.ConversationID == "" {
		return types.ConversationSubscriptionResult{}, types.NewInvalidFrame("conversation_id is required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	target := registry.sessions[command.AuthContext.SessionID]
	if target == nil ||
		target.auth.TenantID != command.AuthContext.TenantID ||
		target.auth.UserID != command.AuthContext.UserID ||
		target.auth.DeviceID != command.AuthContext.DeviceID {
		return types.ConversationSubscriptionResult{}, types.ErrPermissionDenied
	}
	key := conversationKey(command.AuthContext.TenantID, command.ConversationID)
	if target.conversations == nil {
		target.conversations = make(map[string]struct{})
	}
	target.conversations[key] = struct{}{}
	if registry.byConversation[key] == nil {
		registry.byConversation[key] = make(map[string]struct{})
	}
	registry.byConversation[key][command.AuthContext.SessionID] = struct{}{}
	return types.ConversationSubscriptionResult{ConversationID: command.ConversationID, Subscribed: true}, nil
}

func (registry *Registry) UnsubscribeConversation(
	ctx context.Context,
	command types.ConversationSubscriptionCommand,
) (types.ConversationSubscriptionResult, error) {
	if err := command.AuthContext.Validate(); err != nil {
		return types.ConversationSubscriptionResult{}, err
	}
	if command.ConversationID == "" {
		return types.ConversationSubscriptionResult{}, types.NewInvalidFrame("conversation_id is required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	target := registry.sessions[command.AuthContext.SessionID]
	if target == nil ||
		target.auth.TenantID != command.AuthContext.TenantID ||
		target.auth.UserID != command.AuthContext.UserID ||
		target.auth.DeviceID != command.AuthContext.DeviceID {
		return types.ConversationSubscriptionResult{}, types.ErrPermissionDenied
	}
	key := conversationKey(command.AuthContext.TenantID, command.ConversationID)
	delete(target.conversations, key)
	delete(registry.byConversation[key], command.AuthContext.SessionID)
	if len(registry.byConversation[key]) == 0 {
		delete(registry.byConversation, key)
	}
	return types.ConversationSubscriptionResult{ConversationID: command.ConversationID, Subscribed: false}, nil
}

func (registry *Registry) EnqueueConversationSignal(
	ctx context.Context,
	notification types.DeliveryNotification,
) (types.NotifyDeliveryResult, error) {
	notification.Kind = types.DeliveryNotificationKindConversationSignal
	if err := notification.Validate(); err != nil {
		return types.NotifyDeliveryResult{}, err
	}
	key := conversationKey(notification.TenantID, notification.ConversationID)
	registry.mu.Lock()
	registry.pruneExpiredResumesLocked(registry.config.Now())

	sessionIDs := registry.byConversation[key]
	result := types.NotifyDeliveryResult{MatchedSessions: len(sessionIDs)}
	if !registry.shouldEmitConversationSignal(notification, len(sessionIDs)) {
		registry.metrics.ConversationSignalSuppressedEventCount++
		registry.metrics.ConversationSignalSuppressedSessionCount += uint64(len(sessionIDs))
		registry.mu.Unlock()
		return result, nil
	}
	if len(sessionIDs) == 0 {
		registry.mu.Unlock()
		return result, nil
	}
	registry.metrics.ConversationSignalMatchedCount += uint64(len(sessionIDs))
	frame, err := encodedDeliveryFrame(notification)
	if err != nil {
		registry.mu.Unlock()
		return result, err
	}
	targets := registry.collectOutboundTargetsLocked(sessionIDs, notification.EventID, frame)
	registry.mu.Unlock()

	result, err = registry.enqueueConversationOutboundTargets(ctx, frame, result, targets)
	if result.Enqueued > 0 {
		registry.mu.Lock()
		registry.metrics.ConversationSignalEnqueuedCount += uint64(result.Enqueued)
		registry.mu.Unlock()
	}
	return result, err
}

func (registry *Registry) shouldEmitConversationSignal(notification types.DeliveryNotification, subscriberCount int) bool {
	if notification.ConversationSignalSampleEvery > 0 {
		return notification.ConversationSeq%int64(notification.ConversationSignalSampleEvery) == 0
	}
	return registry.config.ConversationSignalPolicy.ShouldEmitForSubscribers(
		notification.ConversationSeq,
		notification.FanoutMode,
		subscriberCount,
	)
}

func (registry *Registry) enqueueConversationOutboundTargets(
	ctx context.Context,
	frame types.ServerFrame,
	result types.NotifyDeliveryResult,
	targets []outboundTarget,
) (types.NotifyDeliveryResult, error) {
	buckets := registry.config.ConversationFanoutBuckets
	if buckets <= 1 || len(targets) <= 1 {
		return registry.enqueueOutboundTargets(ctx, frame, result, targets)
	}
	if buckets > len(targets) {
		buckets = len(targets)
	}
	targetBuckets := make([][]outboundTarget, buckets)
	for _, target := range targets {
		bucket := conversationFanoutBucket(target.sessionID, buckets)
		targetBuckets[bucket] = append(targetBuckets[bucket], target)
	}

	type fanoutBucketResult struct {
		result types.NotifyDeliveryResult
		err    error
	}
	resultCh := make(chan fanoutBucketResult, buckets)
	launched := 0
	for _, bucketTargets := range targetBuckets {
		if len(bucketTargets) == 0 {
			continue
		}
		launched++
		go func(targets []outboundTarget) {
			bucketResult, err := registry.enqueueOutboundTargets(ctx, frame, types.NotifyDeliveryResult{}, targets)
			resultCh <- fanoutBucketResult{result: bucketResult, err: err}
		}(bucketTargets)
	}

	var firstErr error
	for index := 0; index < launched; index++ {
		bucket := <-resultCh
		result.Enqueued += bucket.result.Enqueued
		result.Dropped += bucket.result.Dropped
		result.Evicted += bucket.result.Evicted
		if firstErr == nil && bucket.err != nil {
			firstErr = bucket.err
		}
	}
	return result, firstErr
}

func (registry *Registry) EvictDevice(ctx context.Context, tenantID string, userID string, deviceID string, reason string) (types.SessionEvictionResult, error) {
	if reason == "" {
		reason = "identity_revoked"
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	matches := make([]string, 0)
	for sessionID, existing := range registry.sessions {
		if existing.auth.TenantID == tenantID &&
			existing.auth.UserID == userID &&
			existing.auth.DeviceID == deviceID {
			matches = append(matches, sessionID)
		}
	}
	result := types.SessionEvictionResult{MatchedSessions: len(matches)}
	for _, sessionID := range matches {
		target := registry.sessions[sessionID]
		if target == nil {
			continue
		}
		registry.evictLocked(sessionID, target, types.SessionEviction{Reason: reason})
		registry.metrics.IdentitySessionEvictedCount++
		result.Evicted++
	}
	return result, nil
}

func (registry *Registry) EvictSession(ctx context.Context, tenantID string, userID string, deviceID string, sessionID string, reason string) (types.SessionEvictionResult, error) {
	if reason == "" {
		reason = "identity_revoked"
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	target := registry.sessions[sessionID]
	if target == nil ||
		target.auth.TenantID != tenantID ||
		target.auth.UserID != userID ||
		target.auth.DeviceID != deviceID {
		return types.SessionEvictionResult{}, nil
	}
	registry.evictLocked(sessionID, target, types.SessionEviction{Reason: reason})
	registry.metrics.IdentitySessionEvictedCount++
	return types.SessionEvictionResult{MatchedSessions: 1, Evicted: 1}, nil
}

func (registry *Registry) Metrics() Metrics {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.pruneExpiredResumesLocked(registry.config.Now())
	snapshot := registry.metrics
	snapshot.ConnectedSessions = len(registry.sessions)
	for _, sessions := range registry.byConversation {
		snapshot.ConversationSubscriptionCount += len(sessions)
	}
	for _, state := range registry.resumes {
		snapshot.ResumeBufferStoredFrames += len(state.frames)
	}
	snapshot.ResumeBufferTokenCount = len(registry.resumes)
	return snapshot
}

func (registry *Registry) replayLocked(registration types.SessionRegistration, state *resumeState) bool {
	if len(state.frames) == 0 {
		return false
	}
	lastReceived := make(map[string]int64, len(registration.LastReceived))
	for _, cursor := range registration.LastReceived {
		if cursor.ConversationID == "" {
			continue
		}
		if cursor.Seq > lastReceived[cursor.ConversationID] {
			lastReceived[cursor.ConversationID] = cursor.Seq
		}
	}
	oldestByConversation := make(map[string]int64)
	for _, frame := range state.frames {
		if frame.Op != types.OpDeliveryNotify || frame.ConversationID == "" {
			continue
		}
		if oldestByConversation[frame.ConversationID] == 0 ||
			frame.ConversationSeq < oldestByConversation[frame.ConversationID] {
			oldestByConversation[frame.ConversationID] = frame.ConversationSeq
		}
	}
	for conversationID, seq := range lastReceived {
		oldest := oldestByConversation[conversationID]
		if oldest > 0 && seq+1 < oldest {
			return true
		}
	}
	pendingFrames := make([]types.ServerFrame, 0, len(state.frames))
	for _, frame := range state.frames {
		if !isResumeFrame(frame) {
			continue
		}
		if frame.Op == types.OpDeliveryNotify && frame.ConversationID != "" && frame.ConversationSeq <= lastReceived[frame.ConversationID] {
			continue
		}
		pendingFrames = append(pendingFrames, frame)
	}
	if len(pendingFrames) == 0 {
		return false
	}
	if available := cap(registration.Outbound) - len(registration.Outbound); available < len(pendingFrames) {
		return true
	}
	for _, frame := range pendingFrames {
		registration.Outbound <- stampOutboundFrame(frame)
		registry.metrics.ResumeBufferReplayCount++
	}
	return false
}

func (registry *Registry) enqueueResumeHintLocked(outbound chan<- types.ServerFrame) {
	select {
	case outbound <- stampOutboundFrame(domain.ResumeHint("buffer_miss", nil)):
	default:
	}
}

func (registry *Registry) collectOutboundTargetsLocked(
	sessionIDs map[string]struct{},
	eventID string,
	frame types.ServerFrame,
) []outboundTarget {
	targets := make([]outboundTarget, 0, len(sessionIDs))
	for sessionID := range sessionIDs {
		target := registry.sessions[sessionID]
		if target == nil {
			continue
		}
		if _, ok := target.seen[eventID]; ok {
			continue
		}
		target.seen[eventID] = struct{}{}
		registry.appendResumeLocked(target.resumeToken, frame)
		targets = append(targets, outboundTarget{
			sessionID: sessionID,
			session:   target,
			outbound:  target.outbound,
		})
	}
	return targets
}

func (registry *Registry) enqueueOutboundTargets(
	ctx context.Context,
	frame types.ServerFrame,
	result types.NotifyDeliveryResult,
	targets []outboundTarget,
) (types.NotifyDeliveryResult, error) {
	for _, target := range targets {
		stampedFrame := stampOutboundFrame(frame)
		select {
		case target.outbound <- stampedFrame:
			result.Enqueued++
		case <-ctx.Done():
			return result, ctx.Err()
		default:
			if registry.evictSlowTarget(target.sessionID, target.session) {
				result.Dropped++
				result.Evicted++
			}
		}
	}
	return result, nil
}

func stampOutboundFrame(frame types.ServerFrame) types.ServerFrame {
	if frame.EnqueuedAtMS == 0 {
		frame.EnqueuedAtMS = time.Now().UnixMilli()
	}
	return frame
}

func (registry *Registry) appendResumeLocked(resumeToken string, frame types.ServerFrame) {
	if resumeToken == "" || !isResumeFrame(frame) {
		return
	}
	state := registry.resumes[resumeToken]
	if state == nil {
		return
	}
	state.expiresAt = registry.resumeExpiresAt(registry.config.Now())
	state.frames = append(state.frames, frame)
	if len(state.frames) > types.DefaultResumeBufferSize {
		copy(state.frames, state.frames[len(state.frames)-types.DefaultResumeBufferSize:])
		state.frames = state.frames[:types.DefaultResumeBufferSize]
	}
}

func isResumeFrame(frame types.ServerFrame) bool {
	return frame.Op == types.OpDeliveryNotify || frame.Op == types.OpDeliveryHide
}

func (registry *Registry) evictLocked(sessionID string, target *session, eviction types.SessionEviction) {
	delete(registry.sessions, sessionID)
	key := userKey(target.auth)
	delete(registry.byUser[key], sessionID)
	if len(registry.byUser[key]) == 0 {
		delete(registry.byUser, key)
	}
	registry.removeConversationSubscriptionsLocked(sessionID, target)
	if target.evicted == nil {
		return
	}
	select {
	case target.evicted <- eviction:
	default:
	}
}

func (registry *Registry) evictSlowTarget(sessionID string, target *session) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.sessions[sessionID] != target {
		return false
	}
	registry.evictLocked(sessionID, target, types.SessionEviction{
		Reason: "slow_session",
	})
	registry.metrics.SessionQueueFullCount++
	registry.metrics.SlowSessionEvictedCount++
	return true
}

func userKey(auth types.AuthContext) string {
	return auth.TenantID + "\x1f" + auth.UserID
}

func conversationKey(tenantID string, conversationID string) string {
	return tenantID + "\x1f" + conversationID
}

func conversationFanoutBucket(sessionID string, buckets int) int {
	if buckets <= 1 {
		return 0
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(sessionID))
	return int(hash.Sum32() % uint32(buckets))
}

func encodedDeliveryFrame(notification types.DeliveryNotification) (types.ServerFrame, error) {
	frame := domain.DeliveryNotify(notification)
	payload, err := json.Marshal(frame)
	if err != nil {
		return types.ServerFrame{}, err
	}
	frame.EncodedPayload = payload
	return frame, nil
}

func (registry *Registry) removeConversationSubscriptionsLocked(sessionID string, target *session) {
	for key := range target.conversations {
		delete(registry.byConversation[key], sessionID)
		if len(registry.byConversation[key]) == 0 {
			delete(registry.byConversation, key)
		}
	}
	target.conversations = nil
}

func sameDevice(left types.AuthContext, right types.AuthContext) bool {
	return left.TenantID == right.TenantID &&
		left.UserID == right.UserID &&
		left.DeviceID == right.DeviceID
}

func (registry *Registry) resumeExpiresAt(now time.Time) time.Time {
	return now.Add(registry.config.ResumeBufferTTL)
}

func (registry *Registry) pruneExpiredResumesLocked(now time.Time) {
	activeTokens := make(map[string]struct{})
	for _, existing := range registry.sessions {
		if existing.resumeToken != "" {
			activeTokens[existing.resumeToken] = struct{}{}
		}
	}
	for token, state := range registry.resumes {
		if _, ok := activeTokens[token]; ok {
			state.expiresAt = registry.resumeExpiresAt(now)
			continue
		}
		if !state.expiresAt.IsZero() && !now.Before(state.expiresAt) {
			delete(registry.resumes, token)
			registry.metrics.ResumeBufferExpiredCount++
		}
	}
}
