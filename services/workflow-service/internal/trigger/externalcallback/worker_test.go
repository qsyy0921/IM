package externalcallback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestWorkerMarksDeliveredOnProviderSuccess(t *testing.T) {
	store := &fakeStore{
		deliveries: []types.WorkflowExternalCallbackDelivery{testDelivery(1, 3)},
	}
	provider := fakeProvider{
		result: types.WorkflowExternalCallbackDeliveryResult{DeliveryResultRef: "provider-status:202"},
	}
	worker := NewWorker(store, provider, Config{Now: fixedNow})

	count, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if count != 1 || len(store.delivered) != 1 || len(store.failed) != 0 {
		t.Fatalf("unexpected worker result count=%d delivered=%d failed=%d", count, len(store.delivered), len(store.failed))
	}
	if store.delivered[0].DeliveryResultRef != "provider-status:202" {
		t.Fatalf("unexpected result ref: %+v", store.delivered[0])
	}
}

func TestWorkerMarksFailedOnProviderError(t *testing.T) {
	store := &fakeStore{
		deliveries: []types.WorkflowExternalCallbackDelivery{testDelivery(1, 3)},
	}
	provider := fakeProvider{
		result: types.WorkflowExternalCallbackDeliveryResult{FailureClass: "provider timeout"},
		err:    errors.New("raw provider timeout"),
	}
	worker := NewWorker(store, provider, Config{Now: fixedNow, RetryBaseDelay: time.Second})

	count, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if count != 1 || len(store.failed) != 1 || len(store.delivered) != 0 {
		t.Fatalf("unexpected worker result count=%d delivered=%d failed=%d", count, len(store.delivered), len(store.failed))
	}
	if store.failed[0].FailureClass != "provider_timeout" {
		t.Fatalf("unexpected failure class: %+v", store.failed[0])
	}
	if !store.nextRetryAt[0].Equal(fixedNow().Add(time.Second)) {
		t.Fatalf("unexpected next retry: %s", store.nextRetryAt[0])
	}
}

func TestWorkerTreatsMissingProviderResultAsFailure(t *testing.T) {
	store := &fakeStore{
		deliveries: []types.WorkflowExternalCallbackDelivery{testDelivery(2, 2)},
	}
	worker := NewWorker(store, fakeProvider{}, Config{Now: fixedNow, RetryBaseDelay: time.Second})

	count, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if count != 1 || len(store.failed) != 1 || len(store.delivered) != 0 {
		t.Fatalf("unexpected worker result count=%d delivered=%d failed=%d", count, len(store.delivered), len(store.failed))
	}
	if store.failed[0].FailureClass != "provider_result_missing" {
		t.Fatalf("missing provider result must be explicit failure: %+v", store.failed[0])
	}
	if !store.nextRetryAt[0].Equal(fixedNow()) {
		t.Fatalf("last attempt should be eligible for DLQ immediately, got %s", store.nextRetryAt[0])
	}
}

type fakeStore struct {
	deliveries  []types.WorkflowExternalCallbackDelivery
	delivered   []types.WorkflowExternalCallbackDeliveryResult
	failed      []types.WorkflowExternalCallbackDeliveryResult
	nextRetryAt []time.Time
}

func (store *fakeStore) ClaimReadyExternalCallbackDeliveries(
	context.Context,
	time.Time,
	int,
	time.Duration,
) ([]types.WorkflowExternalCallbackDelivery, error) {
	return store.deliveries, nil
}

func (store *fakeStore) MarkExternalCallbackDeliveryDelivered(
	_ context.Context,
	_ types.WorkflowExternalCallbackDelivery,
	result types.WorkflowExternalCallbackDeliveryResult,
) (types.WorkflowExternalCallbackDelivery, error) {
	store.delivered = append(store.delivered, result)
	return types.WorkflowExternalCallbackDelivery{}, nil
}

func (store *fakeStore) MarkExternalCallbackDeliveryFailed(
	_ context.Context,
	_ types.WorkflowExternalCallbackDelivery,
	result types.WorkflowExternalCallbackDeliveryResult,
	nextRetryAt time.Time,
) (types.WorkflowExternalCallbackDelivery, error) {
	store.failed = append(store.failed, result)
	store.nextRetryAt = append(store.nextRetryAt, nextRetryAt)
	return types.WorkflowExternalCallbackDelivery{}, nil
}

type fakeProvider struct {
	result types.WorkflowExternalCallbackDeliveryResult
	err    error
}

func (provider fakeProvider) DeliverExternalCallback(context.Context, types.WorkflowExternalCallbackDelivery) (types.WorkflowExternalCallbackDeliveryResult, error) {
	return provider.result, provider.err
}

func testDelivery(attemptCount int, maxAttempts int) types.WorkflowExternalCallbackDelivery {
	return types.WorkflowExternalCallbackDelivery{
		TenantID:     "tenant-workflow-test",
		WorkflowID:   "wf_external_callback_delivery",
		DeliveryID:   "wfecd_test",
		AttemptCount: attemptCount,
		MaxAttempts:  maxAttempts,
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
}
