package memory

import (
	"context"
	"sync"

	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type Registry struct {
	mu       sync.RWMutex
	sessions map[string]*session
	byUser   map[string]map[string]struct{}
	resumes  map[string]*resumeState
}

type session struct {
	auth        types.AuthContext
	resumeToken string
	outbound    chan<- types.ServerFrame
	evicted     chan<- types.SessionEviction
	seen        map[string]struct{}
}

type resumeState struct {
	auth   types.AuthContext
	frames []types.ServerFrame
}

func NewRegistry() *Registry {
	return &Registry{
		sessions: make(map[string]*session),
		byUser:   make(map[string]map[string]struct{}),
		resumes:  make(map[string]*resumeState),
	}
}

func (registry *Registry) Register(ctx context.Context, registration types.SessionRegistration) error {
	if registration.SessionID == "" || registration.Outbound == nil {
		return types.NewInvalidFrame("session registration is incomplete")
	}
	if err := registration.AuthContext.Validate(); err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()

	var state *resumeState
	bufferMiss := false
	if registration.ResumeToken != "" {
		var ok bool
		state, ok = registry.resumes[registration.ResumeToken]
		if ok && !sameDevice(state.auth, registration.AuthContext) {
			return types.ErrPermissionDenied
		}
		if !ok {
			bufferMiss = registration.ResumeRequested
			state = &resumeState{auth: registration.AuthContext}
			registry.resumes[registration.ResumeToken] = state
		}
	}
	if previous, ok := registry.sessions[registration.SessionID]; ok {
		delete(registry.byUser[userKey(previous.auth)], registration.SessionID)
	}
	registry.sessions[registration.SessionID] = &session{
		auth:        registration.AuthContext,
		resumeToken: registration.ResumeToken,
		outbound:    registration.Outbound,
		evicted:     registration.Evicted,
		seen:        make(map[string]struct{}),
	}
	if state != nil {
		if bufferMiss || registry.replayLocked(registration, state) {
			registry.enqueueResumeHintLocked(registration.Outbound)
		}
	}
	key := userKey(registration.AuthContext)
	if registry.byUser[key] == nil {
		registry.byUser[key] = make(map[string]struct{})
	}
	registry.byUser[key][registration.SessionID] = struct{}{}
	return nil
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
}

func (registry *Registry) EnqueueNotification(
	ctx context.Context,
	notification types.DeliveryNotification,
) (types.NotifyDeliveryResult, error) {
	key := notification.TenantID + "\x1f" + notification.UserID
	registry.mu.Lock()
	defer registry.mu.Unlock()

	sessionIDs := registry.byUser[key]
	result := types.NotifyDeliveryResult{MatchedSessions: len(sessionIDs)}
	if len(sessionIDs) == 0 {
		return result, nil
	}
	frame := domain.DeliveryNotify(notification)
	for sessionID := range sessionIDs {
		target := registry.sessions[sessionID]
		if target == nil {
			continue
		}
		if _, ok := target.seen[notification.EventID]; ok {
			continue
		}
		select {
		case target.outbound <- frame:
			target.seen[notification.EventID] = struct{}{}
			registry.appendResumeLocked(target.resumeToken, frame)
			result.Enqueued++
		case <-ctx.Done():
			return result, ctx.Err()
		default:
			registry.appendResumeLocked(target.resumeToken, frame)
			registry.evictLocked(sessionID, target, types.SessionEviction{
				Reason: "slow_session",
			})
			result.Dropped++
			result.Evicted++
		}
	}
	return result, nil
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
	for _, frame := range state.frames {
		if frame.Op != types.OpDeliveryNotify {
			continue
		}
		if frame.ConversationID != "" && frame.ConversationSeq <= lastReceived[frame.ConversationID] {
			continue
		}
		select {
		case registration.Outbound <- frame:
		default:
			return true
		}
	}
	return false
}

func (registry *Registry) enqueueResumeHintLocked(outbound chan<- types.ServerFrame) {
	select {
	case outbound <- domain.ResumeHint("buffer_miss", nil):
	default:
	}
}

func (registry *Registry) appendResumeLocked(resumeToken string, frame types.ServerFrame) {
	if resumeToken == "" || frame.Op != types.OpDeliveryNotify {
		return
	}
	state := registry.resumes[resumeToken]
	if state == nil {
		return
	}
	state.frames = append(state.frames, frame)
	if len(state.frames) > types.DefaultResumeBufferSize {
		copy(state.frames, state.frames[len(state.frames)-types.DefaultResumeBufferSize:])
		state.frames = state.frames[:types.DefaultResumeBufferSize]
	}
}

func (registry *Registry) evictLocked(sessionID string, target *session, eviction types.SessionEviction) {
	delete(registry.sessions, sessionID)
	key := userKey(target.auth)
	delete(registry.byUser[key], sessionID)
	if len(registry.byUser[key]) == 0 {
		delete(registry.byUser, key)
	}
	if target.evicted == nil {
		return
	}
	select {
	case target.evicted <- eviction:
	default:
	}
}

func userKey(auth types.AuthContext) string {
	return auth.TenantID + "\x1f" + auth.UserID
}

func sameDevice(left types.AuthContext, right types.AuthContext) bool {
	return left.TenantID == right.TenantID &&
		left.UserID == right.UserID &&
		left.DeviceID == right.DeviceID
}
