package main

import (
	"errors"
	"time"
)

type outboxRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
	DryRun    bool
}

func outboxRepairCleanupConfigFromEnv() (outboxRepairCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_RETENTION", 7*24*time.Hour)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	batchSize := envInt("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE", 200)
	if batchSize <= 0 {
		return outboxRepairCleanupConfig{}, errors.New("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE must be positive")
	}
	return outboxRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
		DryRun:    envBool("NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_DRY_RUN", false),
	}, nil
}
