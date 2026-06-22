package app

import (
	"context"

	"github.com/qsyy0921/IM/services/presence-service/internal/domain"
	"github.com/qsyy0921/IM/services/presence-service/internal/types"
)

type Repository interface {
	UpdatePresence(ctx context.Context, prepared domain.PreparedPresenceUpdate, eventID string) (types.PresenceState, error)
	GetPresenceStates(ctx context.Context, command types.GetPresenceCommand) ([]types.PresenceState, error)
	UpdateTyping(ctx context.Context, prepared domain.PreparedTypingUpdate, eventID string) (types.TypingIndicator, error)
}

type EventIDGenerator interface {
	NewEventID() (string, error)
}
