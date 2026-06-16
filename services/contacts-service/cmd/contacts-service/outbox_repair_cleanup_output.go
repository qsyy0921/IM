package main

import (
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
)

type outboxRepairCleanupOutput struct {
	GeneratedAt      string            `json:"generated_at"`
	Deleted          int64             `json:"deleted"`
	Cutoff           string            `json:"cutoff"`
	RetentionSeconds int64             `json:"retention_seconds"`
	BatchSize        int               `json:"batch_size"`
	DryRun           bool              `json:"dry_run"`
	Filters          map[string]string `json:"filters,omitempty"`
}

func writeOutboxRepairCleanupOutput(path string, stats postgresinfra.OutboxRepairCleanupStats, cutoff time.Time, retention time.Duration, batchSize int, dryRun bool, filters map[string]string) error {
	return writeJSONFile(path, outboxRepairCleanupOutput{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Deleted:          stats.Deleted,
		Cutoff:           cutoff.UTC().Format(time.RFC3339Nano),
		RetentionSeconds: int64(retention.Seconds()),
		BatchSize:        batchSize,
		DryRun:           dryRun,
		Filters:          compactCleanupFilters(filters),
	})
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
