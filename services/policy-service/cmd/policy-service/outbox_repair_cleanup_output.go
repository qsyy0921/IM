package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

type outboxRepairCleanupOutput struct {
	GeneratedAt      string            `json:"generated_at"`
	Deleted          int64             `json:"deleted"`
	Cutoff           string            `json:"cutoff"`
	RetentionSeconds int64             `json:"retention_seconds"`
	BatchSize        int               `json:"batch_size"`
	Filters          map[string]string `json:"filters,omitempty"`
}

func writeOutboxRepairCleanupOutput(path string, stats postgresinfra.OutboxRepairCleanupStats, cutoff time.Time, retention time.Duration, batchSize int, filters map[string]string) error {
	output := outboxRepairCleanupOutput{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Deleted:          stats.Deleted,
		Cutoff:           cutoff.UTC().Format(time.RFC3339Nano),
		RetentionSeconds: int64(retention.Seconds()),
		BatchSize:        batchSize,
		Filters:          compactCleanupFilters(filters),
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

func compactCleanupFilters(filters map[string]string) map[string]string {
	compacted := make(map[string]string, len(filters))
	for key, value := range filters {
		if value != "" {
			compacted[key] = value
		}
	}
	if len(compacted) == 0 {
		return nil
	}
	return compacted
}
