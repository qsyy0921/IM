package main

import (
	"time"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type outboxRepairOutput struct {
	GeneratedAt  string `json:"generated_at"`
	EventIDCount int    `json:"event_id_count"`
	Requested    int    `json:"requested"`
	Repaired     int    `json:"repaired"`
	Skipped      int    `json:"skipped"`
}

func writeOutboxRepairOutput(path string, stats types.OutboxRepairStats, eventIDCount int) error {
	return writeJSONFile(path, outboxRepairOutput{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		EventIDCount: eventIDCount,
		Requested:    stats.Requested,
		Repaired:     stats.Repaired,
		Skipped:      stats.Skipped,
	})
}
