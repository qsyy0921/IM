package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

func TestWorkerRunOnceMarksSuccess(t *testing.T) {
	store := &fakeStore{
		requests: []types.DeliveryRequest{{
			NotificationRequest: types.NotificationRequest{
				TenantID:  "tenant-1",
				RequestID: "notif-1",
			},
			AttemptNumber: 1,
			ProviderID:    "provider-1",
		}},
	}
	worker := NewWorker(store, fakeProvider{}, nil, Config{ProviderID: "provider-1"})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Succeeded != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if store.succeeded != 1 || store.failed != 0 {
		t.Fatalf("unexpected store calls: %+v", store)
	}
}

func TestWorkerRunOnceRecordsProviderFailure(t *testing.T) {
	store := &fakeStore{
		requests: []types.DeliveryRequest{{
			NotificationRequest: types.NotificationRequest{
				TenantID:  "tenant-1",
				RequestID: "notif-1",
			},
			AttemptNumber: 1,
			ProviderID:    "provider-1",
		}},
	}
	worker := NewWorker(store, fakeProvider{err: errors.New("provider down")}, nil, Config{ProviderID: "provider-1", MaxAttempts: 3})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if store.failure.FailureClass != types.FailureClassProviderUnavailable {
		t.Fatalf("unexpected failure: %+v", store.failure)
	}
}

func TestWorkerRunOnceDeadLettersExpiredRequest(t *testing.T) {
	store := &fakeStore{
		deadLetter: true,
		requests: []types.DeliveryRequest{{
			NotificationRequest: types.NotificationRequest{
				TenantID:  "tenant-1",
				RequestID: "notif-1",
				ExpiresAt: time.Now().Add(-time.Minute),
			},
			AttemptNumber: 1,
			ProviderID:    "provider-1",
		}},
	}
	worker := NewWorker(store, fakeProvider{}, nil, Config{ProviderID: "provider-1", MaxAttempts: 3})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.DeadLettered != 1 || stats.Succeeded != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if store.failure.FailureClass != types.FailureClassExpired || !store.failure.Permanent {
		t.Fatalf("unexpected failure: %+v", store.failure)
	}
}

func TestWorkerRunOnceFailsWhenProviderMissing(t *testing.T) {
	worker := NewWorker(&fakeStore{}, nil, nil, Config{})
	if _, err := worker.RunOnce(context.Background()); err == nil {
		t.Fatalf("expected missing provider error")
	}
}

type fakeStore struct {
	requests   []types.DeliveryRequest
	deadLetter bool
	succeeded  int
	failed     int
	failure    types.DeliveryFailure
}

func (store *fakeStore) ClaimReadyDeliveryRequests(context.Context, int, string) ([]types.DeliveryRequest, error) {
	return store.requests, nil
}

func (store *fakeStore) MarkDeliverySucceeded(context.Context, types.DeliveryRequest, types.DeliveryResult) error {
	store.succeeded++
	return nil
}

func (store *fakeStore) MarkDeliveryFailed(_ context.Context, _ types.DeliveryRequest, failure types.DeliveryFailure, _ int, _ time.Duration) (bool, error) {
	store.failed++
	store.failure = failure
	return store.deadLetter || failure.Permanent, nil
}

type fakeProvider struct {
	err error
}

func (provider fakeProvider) Send(_ context.Context, request types.DeliveryRequest) (types.DeliveryResult, error) {
	if provider.err != nil {
		return types.DeliveryResult{}, provider.err
	}
	return types.DeliveryResult{
		ProviderID:            request.ProviderID,
		ProviderMessageIDHash: "provider-message-hash",
	}, nil
}
