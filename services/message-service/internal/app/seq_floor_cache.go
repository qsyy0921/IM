package app

import (
	"context"
	"strings"
	"sync"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type seqFloorLoader func(context.Context, types.TenantID, types.ConversationID) (int64, error)

type seqFloorCache struct {
	mu       sync.Mutex
	values   map[string]int64
	inflight map[string]*seqFloorInflight
}

type seqFloorInflight struct {
	ready chan struct{}
	value int64
	err   error
}

func newSeqFloorCache() *seqFloorCache {
	return &seqFloorCache{
		values:   map[string]int64{},
		inflight: map[string]*seqFloorInflight{},
	}
}

func (cache *seqFloorCache) minimumStartSeq(
	ctx context.Context,
	command types.SendMessageCommand,
	load seqFloorLoader,
) (int64, error) {
	if cache == nil {
		return loadSeqFloor(ctx, command, load)
	}
	key := seqFloorCacheKey(command)
	for {
		cache.mu.Lock()
		if value, ok := cache.values[key]; ok {
			cache.mu.Unlock()
			return value, nil
		}
		if current, ok := cache.inflight[key]; ok {
			ready := current.ready
			cache.mu.Unlock()
			select {
			case <-ready:
				if current.err != nil {
					return 0, current.err
				}
				return current.value, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
		current := &seqFloorInflight{ready: make(chan struct{})}
		cache.inflight[key] = current
		cache.mu.Unlock()

		value, err := loadSeqFloor(ctx, command, load)

		cache.mu.Lock()
		current.value = value
		current.err = err
		if err == nil {
			cache.values[key] = value
		}
		delete(cache.inflight, key)
		close(current.ready)
		cache.mu.Unlock()
		return value, err
	}
}

func loadSeqFloor(ctx context.Context, command types.SendMessageCommand, load seqFloorLoader) (int64, error) {
	value, err := load(ctx, command.AuthContext.TenantID, command.ConversationID)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 1, nil
	}
	return value, nil
}

func seqFloorCacheKey(command types.SendMessageCommand) string {
	return strings.Join([]string{
		string(command.AuthContext.TenantID),
		string(command.ConversationID),
	}, "\x1f")
}
