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
}

type session struct {
	auth     types.AuthContext
	outbound chan<- types.ServerFrame
	evicted  chan<- types.SessionEviction
	seen     map[string]struct{}
}

func NewRegistry() *Registry {
	return &Registry{
		sessions: make(map[string]*session),
		byUser:   make(map[string]map[string]struct{}),
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

	if previous, ok := registry.sessions[registration.SessionID]; ok {
		delete(registry.byUser[userKey(previous.auth)], registration.SessionID)
	}
	registry.sessions[registration.SessionID] = &session{
		auth:     registration.AuthContext,
		outbound: registration.Outbound,
		evicted:  registration.Evicted,
		seen:     make(map[string]struct{}),
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
			result.Enqueued++
		case <-ctx.Done():
			return result, ctx.Err()
		default:
			registry.evictLocked(sessionID, target, types.SessionEviction{
				Reason: "slow_session",
			})
			result.Dropped++
			result.Evicted++
		}
	}
	return result, nil
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
