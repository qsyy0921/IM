package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func writeSummary(resultDir string, result *summary) error {
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		return err
	}
	result.Capacity = buildCapacitySummary(result)
	result.ResultFile = filepath.Join(resultDir, "sendmessage-summary.json")
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(result.ResultFile, encoded, 0644)
}

type commitInfo struct {
	Short       string
	Full        string
	Dirty       bool
	StatusShort string
}

func currentCommit() commitInfo {
	if override := commitInfoFromEnv(); override.Short != "" {
		return override
	}
	shortOutput, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return commitInfo{Short: "unknown", Full: "unknown"}
	}
	fullOutput, err := exec.Command("git", "rev-parse", "HEAD").Output()
	full := "unknown"
	if err == nil {
		full = strings.TrimSpace(string(fullOutput))
	}
	statusOutput, err := exec.Command("git", "status", "--short").Output()
	statusShort := ""
	if err == nil {
		statusShort = strings.TrimSpace(string(statusOutput))
	}
	short := strings.TrimSpace(string(shortOutput))
	dirty := statusShort != ""
	if dirty {
		short += "-dirty"
	}
	return commitInfo{
		Short:       short,
		Full:        full,
		Dirty:       dirty,
		StatusShort: statusShort,
	}
}

func commitInfoFromEnv() commitInfo {
	short := strings.TrimSpace(os.Getenv("NEXUSIM_COMMIT"))
	if short == "" {
		return commitInfo{}
	}
	full := strings.TrimSpace(os.Getenv("NEXUSIM_COMMIT_FULL"))
	if full == "" {
		full = short
	}
	statusShort := strings.TrimSpace(os.Getenv("NEXUSIM_GIT_STATUS_SHORT"))
	dirty, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("NEXUSIM_GIT_DIRTY")))
	if statusShort != "" {
		dirty = true
	}
	if dirty && !strings.HasSuffix(short, "-dirty") {
		short += "-dirty"
	}
	return commitInfo{
		Short:       short,
		Full:        full,
		Dirty:       dirty,
		StatusShort: statusShort,
	}
}

func durationMS(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}

func buildCapacitySummary(result *summary) *capacitySummary {
	if result == nil {
		return nil
	}
	duration := observedRunDuration(result)
	if duration <= 0 {
		return nil
	}
	durationSeconds := duration.Seconds()
	return &capacitySummary{
		DurationMS:            durationMS(duration),
		TargetCount:           observedTargetCount(result),
		VUs:                   result.VUs,
		ConversationCount:     result.ConversationCount,
		RequestCount:          result.RequestCount,
		SuccessCount:          result.SuccessCount,
		ErrorCount:            result.ErrorCount,
		LogicalRequestCount:   result.LogicalRequestCount,
		LogicalSuccessCount:   result.LogicalSuccessCount,
		RequestRPS:            ratePerSecond(result.RequestCount, durationSeconds),
		AcceptedRPS:           ratePerSecond(result.SuccessCount, durationSeconds),
		ErrorRPS:              ratePerSecond(result.ErrorCount, durationSeconds),
		LogicalRequestRPS:     ratePerSecond(result.LogicalRequestCount, durationSeconds),
		LogicalAcceptedRPS:    ratePerSecond(result.LogicalSuccessCount, durationSeconds),
		SuccessRate:           result.SuccessRate,
		LogicalSuccessRate:    result.LogicalSuccessRate,
		P95MS:                 result.P95MS,
		P99MS:                 result.P99MS,
		LogicalP95MS:          result.LogicalP95MS,
		LogicalP99MS:          result.LogicalP99MS,
		OutboxPublishedCount:  result.OutboxPublishedCount,
		OutboxPendingCount:    result.OutboxPendingCount,
		OutboxDLQCount:        result.OutboxDLQCount,
		ServicePGPoolMaxConns: pgPoolMaxConns(result.ServicePGPool),
		RelayPGPoolMaxConns:   pgPoolMaxConns(result.RelayPGPool),
	}
}

func observedRunDuration(result *summary) time.Duration {
	started, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(result.StartedAt))
	if err != nil {
		return 0
	}
	finished, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(result.FinishedAt))
	if err != nil {
		return 0
	}
	return finished.Sub(started)
}

func observedTargetCount(result *summary) int {
	if len(result.Targets) > 0 {
		return len(result.Targets)
	}
	if strings.TrimSpace(result.Target) != "" {
		return 1
	}
	return 0
}

func ratePerSecond(count int64, durationSeconds float64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return float64(count) / durationSeconds
}

func pgPoolMaxConns(stats *pgPoolStats) *int32 {
	if stats == nil {
		return nil
	}
	value := stats.MaxConns
	return &value
}
