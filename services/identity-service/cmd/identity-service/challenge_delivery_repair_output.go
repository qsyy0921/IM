package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type challengeDeliveryRepairOutput struct {
	GeneratedAt   string `json:"generated_at"`
	DeliveryCount int    `json:"delivery_count"`
	Mode          string `json:"mode"`
	DryRun        bool   `json:"dry_run"`
	Requested     int    `json:"requested"`
	Audited       int    `json:"audited"`
	Mutated       int    `json:"mutated"`
	Skipped       int    `json:"skipped"`
}

func writeChallengeDeliveryRepairOutput(path string, stats types.ChallengeDeliveryRepairStats, deliveryCount int, mode string, dryRun bool) error {
	output := challengeDeliveryRepairOutput{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		DeliveryCount: deliveryCount,
		Mode:          mode,
		DryRun:        dryRun,
		Requested:     stats.Requested,
		Audited:       stats.Audited,
		Mutated:       stats.Mutated,
		Skipped:       stats.Skipped,
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
