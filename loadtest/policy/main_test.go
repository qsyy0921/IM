package main

import (
	"math"
	"testing"
	"time"
)

func TestBuildCapacitySummary(t *testing.T) {
	started := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	s := summary{
		StartedAt:              started,
		FinishedAt:             started.Add(4 * time.Second),
		ExpectedAllowed:        true,
		ExpectedPermissionVer:  7,
		ExpectedClassification: "INTERNAL",
		Actions: []actionSummary{
			{Action: "SEND", Allowed: true, LatencyMS: 5},
			{Action: "EDIT", Allowed: true, LatencyMS: 15},
			{Action: "REVOKE", Allowed: false, LatencyMS: 30},
			{Action: "DELETE", Allowed: true, LatencyMS: 10},
		},
		LatenciesMS: map[string]float64{
			"send":   5,
			"edit":   15,
			"revoke": 30,
			"delete": 10,
		},
	}

	capacity := buildCapacitySummary(s)
	if capacity == nil {
		t.Fatal("expected capacity summary")
	}
	if capacity.ActionCount != 4 {
		t.Fatalf("action count = %d, want 4", capacity.ActionCount)
	}
	if capacity.AllowedActionCount != 3 || capacity.DeniedActionCount != 1 {
		t.Fatalf("unexpected allow/deny counts: %+v", capacity)
	}
	assertFloatNear(t, capacity.DecisionsPerSecond, 1)
	assertFloatNear(t, capacity.LatencyP95MS, 30)
	assertFloatNear(t, capacity.LatencyP99MS, 30)
	if !capacity.ExpectedAllowed || capacity.PermissionVersion != 7 || capacity.Classification != "INTERNAL" {
		t.Fatalf("unexpected policy expectation fields: %+v", capacity)
	}
}

func TestBuildCapacitySummaryRequiresPositiveDuration(t *testing.T) {
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	if capacity := buildCapacitySummary(summary{StartedAt: now, FinishedAt: now}); capacity != nil {
		t.Fatalf("expected nil capacity for zero duration, got %+v", capacity)
	}
}

func assertFloatNear(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("float = %f, want %f", got, want)
	}
}
