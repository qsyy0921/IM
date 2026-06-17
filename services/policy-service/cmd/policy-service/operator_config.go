package main

import "time"

type outboxRepairCleanupConfig struct {
	Retention time.Duration
	BatchSize int
	DryRun    bool
}

func outboxRepairCleanupConfigFromEnv() (outboxRepairCleanupConfig, error) {
	retention, err := envPositiveDuration("NEXUSIM_POLICY_OUTBOX_REPAIR_RETENTION", 7*24*time.Hour)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	batchSize, err := envPositiveInt("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_BATCH_SIZE", 5000)
	if err != nil {
		return outboxRepairCleanupConfig{}, err
	}
	return outboxRepairCleanupConfig{
		Retention: retention,
		BatchSize: batchSize,
		DryRun:    envBool("NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_DRY_RUN", false),
	}, nil
}
