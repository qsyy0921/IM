package main

import (
	"math"
	"testing"
	"time"
)

func TestBuildCapacitySummary(t *testing.T) {
	started := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	s := summary{
		StartedAt:                  started,
		FinishedAt:                 started.Add(5 * time.Second),
		Login:                      tokenSummary{GatewayTokenSet: true},
		Refresh:                    tokenSummary{GatewayTokenSet: true},
		PostResetLogin:             tokenSummary{GatewayTokenSet: true},
		RefreshWithMFA:             tokenSummary{GatewayTokenSet: true},
		MFALogin:                   tokenSummary{GatewayTokenSet: true},
		RefreshWithoutMFA:          expectedErrorSummary{Occurred: true},
		LoginWithoutMFA:            expectedErrorSummary{Occurred: true},
		ConfirmMFAEnrollment:       mfaConfirmSummary{RecoveryCodeCount: 8},
		RegenerateMFARecoveryCodes: mfaRegenerateSummary{RecoveryCodeCount: 8},
		ChallengeDeliveryOutbox:    outboxStats{Total: 2, Pending: 0, Delivered: 2, DLQ: 0},
		ChallengeRow:               challengeRow{DeliveryAttemptCount: 1},
		LatenciesMS: map[string]float64{
			"register_user":                  10,
			"login":                          20,
			"refresh_gateway_token":          30,
			"request_verification_challenge": 40,
			"confirm_verification_challenge": 50,
		},
	}

	capacity := buildCapacitySummary(s)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if capacity.OperationCount != 5 {
		t.Fatalf("operation count = %d, want 5", capacity.OperationCount)
	}
	if capacity.TokenIssueCount != 5 {
		t.Fatalf("token issue count = %d, want 5", capacity.TokenIssueCount)
	}
	if capacity.ExpectedErrorCount != 2 {
		t.Fatalf("expected error count = %d, want 2", capacity.ExpectedErrorCount)
	}
	if capacity.MFARecoveryCodeCount != 16 {
		t.Fatalf("recovery code count = %d, want 16", capacity.MFARecoveryCodeCount)
	}
	if capacity.ChallengeDeliveryOutboxTotal != 2 ||
		capacity.ChallengeDeliveryOutboxDelivered != 2 ||
		capacity.ChallengeDeliveryOutboxPending != 0 ||
		capacity.ChallengeDeliveryOutboxDLQ != 0 {
		t.Fatalf("unexpected outbox fields: %+v", capacity)
	}
	if capacity.ChallengeDeliveryAttemptCount != 1 {
		t.Fatalf("delivery attempt count = %d, want 1", capacity.ChallengeDeliveryAttemptCount)
	}
	assertFloatNear(t, capacity.OperationsPerSecond, 1)
	assertFloatNear(t, capacity.LatencyP95MS, 50)
	assertFloatNear(t, capacity.LatencyP99MS, 50)
}

func TestBuildCapacitySummaryRequiresPositiveDuration(t *testing.T) {
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	if capacity := buildCapacitySummary(summary{StartedAt: now, FinishedAt: now}); capacity != nil {
		t.Fatalf("expected nil capacity for zero duration, got %+v", capacity)
	}
}

func TestBuildCapacitySummaryUsesCapacityCounters(t *testing.T) {
	started := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	capacity := buildCapacitySummary(summary{
		StartedAt:                  started,
		FinishedAt:                 started.Add(10 * time.Second),
		CapacityMode:               true,
		VUs:                        4,
		ConfiguredDurationSeconds:  10,
		ChallengeDeliveryOutbox:    outboxStats{Total: 4, Delivered: 4},
		capacityOperationCount:     20,
		capacityTokenIssueCount:    10,
		capacityExpectedErrorCount: 2,
		capacityLatencySamples:     []float64{1, 2, 3, 4, 5},
	})
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if !capacity.CapacityMode || capacity.VUs != 4 || capacity.ConfiguredDurationSeconds != 10 {
		t.Fatalf("unexpected capacity fields: %+v", capacity)
	}
	if capacity.OperationCount != 20 || capacity.TokenIssueCount != 10 || capacity.ExpectedErrorCount != 2 {
		t.Fatalf("unexpected counters: %+v", capacity)
	}
	assertFloatNear(t, capacity.OperationsPerSecond, 2)
	assertFloatNear(t, capacity.LatencyP95MS, 5)
	assertFloatNear(t, capacity.LatencyP99MS, 5)
}

func assertFloatNear(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("float = %f, want %f", got, want)
	}
}
