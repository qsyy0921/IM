package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
)

type SeqBlockRepository interface {
	AllocateSeqBlock(
		ctx context.Context,
		command types.AllocateSeqBlockCommand,
		leaseTTL time.Duration,
	) (types.SeqBlockLease, error)
	ExpireSeqBlockLeases(ctx context.Context, command types.ExpireLeasesCommand) (types.ExpireLeasesResult, error)
	CreateGapMarker(ctx context.Context, command types.GapMarkerCommand) (types.GapMarker, error)
	CloseGapMarker(ctx context.Context, command types.CloseGapMarkerCommand) (types.GapMarker, error)
	AuditGapMarkers(ctx context.Context, tenantID string, conversationID string, status string, limit int) ([]types.GapMarker, error)
}
