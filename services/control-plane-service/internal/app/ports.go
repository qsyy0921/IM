package app

import (
	"context"

	"github.com/qsyy0921/IM/services/control-plane-service/internal/domain"
	"github.com/qsyy0921/IM/services/control-plane-service/internal/types"
)

type Repository interface {
	PublishConfigVersion(ctx context.Context, prepared domain.PreparedConfigVersion, eventID string) (types.ConfigVersion, error)
	RollbackConfigVersion(ctx context.Context, prepared domain.PreparedConfigRollback, eventID string) (types.ConfigVersion, bool, error)
	GetConfigSnapshot(ctx context.Context, command types.GetConfigSnapshotCommand) (types.ConfigSnapshot, error)
	AckAppliedConfigVersion(ctx context.Context, command types.AckAppliedConfigVersionCommand, eventID string) (types.AppliedConfigVersion, error)
}

type EventIDGenerator interface {
	NewEventID() (string, error)
}
