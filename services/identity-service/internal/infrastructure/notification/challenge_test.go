package notification

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestWebhookChallengeNotifierPostsChallenge(t *testing.T) {
	var got types.ChallengeNotification
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content type %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-NexusIM-Request-ID") != "request-1" {
			t.Fatalf("unexpected request id header %q", r.Header.Get("X-NexusIM-Request-ID"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	notifier, err := NewWebhookChallengeNotifier(server.URL, "provider-token", time.Second)
	if err != nil {
		t.Fatalf("new notifier: %v", err)
	}
	err = notifier.SendChallenge(context.Background(), types.ChallengeNotification{
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ChallengeID:     "challenge-1",
		Type:            types.ChallengeTypePasswordReset,
		Channel:         types.VerificationChannelEmail,
		Destination:     "user1@example.com",
		Token:           "challenge-token",
		ExpiresAtUnixMS: 1_800_000_900_000,
		RequestID:       "request-1",
	})
	if err != nil {
		t.Fatalf("send challenge: %v", err)
	}
	if gotAuthorization != "Bearer provider-token" {
		t.Fatalf("unexpected authorization header %q", gotAuthorization)
	}
	if got.Token != "challenge-token" || got.Type != types.ChallengeTypePasswordReset || got.Destination != "user1@example.com" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestWebhookChallengeNotifierReturnsStableErrorOnNonSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	notifier, err := NewWebhookChallengeNotifier(server.URL, "", time.Second)
	if err != nil {
		t.Fatalf("new notifier: %v", err)
	}
	err = notifier.SendChallenge(context.Background(), types.ChallengeNotification{
		TenantID:    "tenant-1",
		UserID:      "user-1",
		ChallengeID: "challenge-1",
		Type:        types.ChallengeTypeEmailVerification,
		Channel:     types.VerificationChannelEmail,
		Destination: "user1@example.com",
		Token:       "challenge-token",
	})
	if !errors.Is(err, types.ErrChallengeDeliveryFailed) {
		t.Fatalf("expected challenge delivery failure, got %v", err)
	}
}
