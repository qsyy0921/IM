package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type outboxRepairOutput struct {
	GeneratedAt  string `json:"generated_at"`
	EventIDCount int    `json:"event_id_count"`
	Requested    int    `json:"requested"`
	Repaired     int    `json:"repaired"`
	Skipped      int    `json:"skipped"`
}

func writeOutboxRepairOutput(path string, stats types.OutboxRepairStats, eventIDCount int) error {
	output := outboxRepairOutput{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		EventIDCount: eventIDCount,
		Requested:    stats.Requested,
		Repaired:     stats.Repaired,
		Skipped:      stats.Skipped,
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
