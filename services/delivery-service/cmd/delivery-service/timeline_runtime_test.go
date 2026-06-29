package main

import "testing"

func TestDeliveryTimelineConsumerWorkerCountFromEnvDefaults(t *testing.T) {
	t.Setenv("NEXUSIM_DELIVERY_TIMELINE_CONSUMER_WORKERS", "")

	workerCount, err := deliveryTimelineConsumerWorkerCountFromEnv()
	if err != nil {
		t.Fatalf("expected default worker count to pass: %v", err)
	}
	if workerCount != 1 {
		t.Fatalf("expected default worker count 1, got %d", workerCount)
	}
}

func TestDeliveryTimelineConsumerWorkerCountFromEnvLoadsCustomValue(t *testing.T) {
	t.Setenv("NEXUSIM_DELIVERY_TIMELINE_CONSUMER_WORKERS", "4")

	workerCount, err := deliveryTimelineConsumerWorkerCountFromEnv()
	if err != nil {
		t.Fatalf("expected custom worker count to pass: %v", err)
	}
	if workerCount != 4 {
		t.Fatalf("expected custom worker count 4, got %d", workerCount)
	}
}

func TestDeliveryTimelineConsumerWorkerCountFromEnvRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"0", "-1", "many"} {
		t.Setenv("NEXUSIM_DELIVERY_TIMELINE_CONSUMER_WORKERS", value)
		if _, err := deliveryTimelineConsumerWorkerCountFromEnv(); err == nil {
			t.Fatalf("expected worker count %q to fail", value)
		}
	}
}
