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
}
