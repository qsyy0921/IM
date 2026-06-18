package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	result.Success = runErr == nil
	if runErr != nil {
		result.Error = runErr.Error()
	}
	result.Capacity = buildCapacitySummary(*result)
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	bytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	path := filepath.Join(cfg.resultDir, "identity-summary.json")
	if err := os.WriteFile(path, append(bytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	fmt.Printf("summary: %s\n", path)
	if runErr != nil {
		return runErr
	}
	return nil
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

func buildCapacitySummary(s summary) *capacitySummary {
	duration := s.FinishedAt.Sub(s.StartedAt).Seconds()
	if duration <= 0 {
		return nil
	}
	operationCount := len(s.LatenciesMS)
	if s.capacityOperationCount > 0 {
		operationCount = s.capacityOperationCount
	}
	tokenIssueCount := tokenIssueCount(s)
	if s.capacityTokenIssueCount > 0 {
		tokenIssueCount = s.capacityTokenIssueCount
	}
	expectedErrorCount := expectedErrorCount(s)
	if s.capacityExpectedErrorCount > 0 {
		expectedErrorCount = s.capacityExpectedErrorCount
	}
	return &capacitySummary{
		CapacityMode:                     s.CapacityMode,
		VUs:                              s.VUs,
		ConfiguredDurationSeconds:        s.ConfiguredDurationSeconds,
		DurationSeconds:                  duration,
		OperationCount:                   operationCount,
		TokenIssueCount:                  tokenIssueCount,
		ExpectedErrorCount:               expectedErrorCount,
		ChallengeDeliveryOutboxTotal:     s.ChallengeDeliveryOutbox.Total,
		ChallengeDeliveryOutboxPending:   s.ChallengeDeliveryOutbox.Pending,
		ChallengeDeliveryOutboxDelivered: s.ChallengeDeliveryOutbox.Delivered,
		ChallengeDeliveryOutboxDLQ:       s.ChallengeDeliveryOutbox.DLQ,
		ChallengeDeliveryAttemptCount:    s.ChallengeRow.DeliveryAttemptCount,
		OperationsPerSecond:              ratePerSecond(operationCount, duration),
		LatencyP95MS:                     latencyQuantileFromSummary(s, 0.95),
		LatencyP99MS:                     latencyQuantileFromSummary(s, 0.99),
		MFARecoveryCodeCount:             recoveryCodeCount(s),
	}
}

func tokenIssueCount(s summary) int {
	count := 0
	if s.Login.GatewayTokenSet {
		count++
	}
	if s.Refresh.GatewayTokenSet {
		count++
	}
	if s.PostResetLogin.GatewayTokenSet {
		count++
	}
	if s.RefreshWithMFA.GatewayTokenSet {
		count++
	}
	if s.MFALogin.GatewayTokenSet {
		count++
	}
	return count
}

func expectedErrorCount(s summary) int {
	count := 0
	if s.RefreshWithoutMFA.Occurred {
		count++
	}
	if s.LoginWithoutMFA.Occurred {
		count++
	}
	return count
}

func recoveryCodeCount(s summary) int {
	return s.ConfirmMFAEnrollment.RecoveryCodeCount + s.RegenerateMFARecoveryCodes.RecoveryCodeCount
}

func ratePerSecond(count int, durationSeconds float64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return float64(count) / durationSeconds
}

func latencyQuantile(values map[string]float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, 0, len(values))
	for _, value := range values {
		sorted = append(sorted, value)
	}
	sort.Float64s(sorted)
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func latencyQuantileFromSummary(s summary, quantile float64) float64 {
	if len(s.capacityLatencySamples) > 0 {
		return latencyQuantileSlice(s.capacityLatencySamples, quantile)
	}
	return latencyQuantile(s.LatenciesMS, quantile)
}

func latencyQuantileSlice(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func generateTOTPCode(secret string, now time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return ""
	}
	counter := uint64(now.Unix() / 30)
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counterBytes[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)
	return fmt.Sprintf("%06d", value%1_000_000)
}

func gitOutput(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitDirty() bool {
	return strings.TrimSpace(gitOutput("status", "--short")) != ""
}
