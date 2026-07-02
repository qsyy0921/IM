package main

import "testing"

func TestCompareMatchExpectedOnlyIgnoresHistoricalObservedEvents(t *testing.T) {
	expected := []expectedEvent{
		{EventID: "run-event-1", Seq: 101},
		{EventID: "run-event-2", Seq: 102},
	}
	observed := []observedEvent{
		{EventID: "historical-event", Seq: 100, Partition: 2, Offset: 10},
		{EventID: "run-event-1", Seq: 101, Partition: 2, Offset: 11},
		{EventID: "run-event-2", Seq: 102, Partition: 2, Offset: 12},
	}

	result := compare("tenant", "conversation", "topic", expected, observed, false, true)
	if result.ExpectedCount != 2 {
		t.Fatalf("expected count = %d, want 2", result.ExpectedCount)
	}
	if result.ObservedCount != 2 {
		t.Fatalf("observed count = %d, want 2", result.ObservedCount)
	}
	if result.ScannedObservedCount != 3 {
		t.Fatalf("scanned observed count = %d, want 3", result.ScannedObservedCount)
	}
	if result.IgnoredObservedCount != 1 {
		t.Fatalf("ignored observed count = %d, want 1", result.IgnoredObservedCount)
	}
	if len(result.UnexpectedIDs) != 0 {
		t.Fatalf("unexpected ids = %v, want empty", result.UnexpectedIDs)
	}
	if len(result.MissingEventIDs) != 0 {
		t.Fatalf("missing ids = %v, want empty", result.MissingEventIDs)
	}
}

func TestCompareStrictModeReportsUnexpectedObservedEvents(t *testing.T) {
	expected := []expectedEvent{{EventID: "run-event", Seq: 101}}
	observed := []observedEvent{
		{EventID: "historical-event", Seq: 100, Partition: 2, Offset: 10},
		{EventID: "run-event", Seq: 101, Partition: 2, Offset: 11},
	}

	result := compare("tenant", "conversation", "topic", expected, observed, false, false)
	if result.ObservedCount != 2 {
		t.Fatalf("observed count = %d, want 2", result.ObservedCount)
	}
	if len(result.UnexpectedIDs) != 1 || result.UnexpectedIDs[0] != "historical-event" {
		t.Fatalf("unexpected ids = %v, want historical-event", result.UnexpectedIDs)
	}
}
