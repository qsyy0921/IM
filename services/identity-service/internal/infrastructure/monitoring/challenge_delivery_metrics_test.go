package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestInstrumentedChallengeNotifierRecordsSuccessAndFailure(t *testing.T) {
	metrics := NewChallengeDeliveryMetrics("webhook")
	notifier := NewInstrumentedChallengeNotifier(&fakeChallengeSender{}, metrics)
	times := []time.Time{
		time.Unix(1_800_000_000, 0).UTC(),
		time.Unix(1_800_000_000, 25_000_000).UTC(),
		time.Unix(1_800_000_001, 0).UTC(),
		time.Unix(1_800_000_001, 50_000_000).UTC(),
	}
	notifier.now = func() time.Time {
		next := times[0]
		times = times[1:]
		return next
	}

	if err := notifier.SendChallenge(context.Background(), types.ChallengeNotification{Token: "secret-token"}); err != nil {
		t.Fatalf("send success: %v", err)
	}
	notifier.notifier = &fakeChallengeSender{err: types.NewChallengeDeliveryFailed("provider failed")}
	if err := notifier.SendChallenge(context.Background(), types.ChallengeNotification{Token: "secret-token"}); !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected delivery failure, got %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.Mode != "webhook" ||
		snapshot.TotalRequests != 2 ||
		snapshot.SuccessCount != 1 ||
		snapshot.FailureCount != 1 ||
		snapshot.LatencyAvgMS != 37 ||
		snapshot.LatencyMaxMS != 50 ||
		snapshot.LastSuccessUnixMS == 0 ||
		snapshot.LastFailureUnixMS == 0 {
		t.Fatalf("unexpected challenge delivery snapshot: %+v", snapshot)
	}
}

func TestHandlerMetricsIncludesChallengeDeliverySnapshot(t *testing.T) {
	metrics := NewChallengeDeliveryMetrics("webhook")
	metrics.record(12, nil, time.Unix(1_800_000_000, 0).UTC())
	handler := NewHandler(nil).WithChallengeDeliveryMetrics(metrics)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	bodyRaw := response.Body.String()
	if strings.Contains(bodyRaw, "secret-token") ||
		strings.Contains(bodyRaw, "user1@example.com") ||
		strings.Contains(bodyRaw, "user-1") {
		t.Fatalf("challenge delivery metrics leaked per-user delivery data: %s", bodyRaw)
	}
	var body Snapshot
	if err := json.Unmarshal([]byte(bodyRaw), &body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.ChallengeDelivery == nil ||
		body.ChallengeDelivery.Mode != "webhook" ||
		body.ChallengeDelivery.TotalRequests != 1 ||
		body.ChallengeDelivery.SuccessCount != 1 {
		t.Fatalf("expected challenge delivery metrics, got %+v", body.ChallengeDelivery)
	}
}

type fakeChallengeSender struct {
	err error
}

func (sender *fakeChallengeSender) SendChallenge(context.Context, types.ChallengeNotification) error {
	return sender.err
}
