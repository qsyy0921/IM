package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

func TestWebhookProviderPostsLowSensitivePayload(t *testing.T) {
	var gotAuthorization string
	var gotIdempotency string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAuthorization = request.Header.Get("Authorization")
		gotIdempotency = request.Header.Get("X-NexusIM-Provider-Idempotency-Key")
		if err := json.NewDecoder(request.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"provider_message_id":"provider-message-1","provider_body":"ignored"}`))
	}))
	defer server.Close()

	provider, err := NewWebhookProvider(WebhookConfig{
		URL:         server.URL,
		BearerToken: "provider-token",
		ProviderID:  "webhook-fixture",
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new webhook provider: %v", err)
	}
	result, err := provider.Send(context.Background(), webhookDeliveryRequest())
	if err != nil {
		t.Fatalf("send webhook: %v", err)
	}
	if result.ProviderID != "webhook-fixture" {
		t.Fatalf("unexpected provider id %q", result.ProviderID)
	}
	if result.ProviderMessageIDHash == "" || strings.Contains(result.ProviderMessageIDHash, "provider-message-1") {
		t.Fatalf("provider message id should be hashed, got %q", result.ProviderMessageIDHash)
	}
	if gotAuthorization != "Bearer provider-token" {
		t.Fatalf("unexpected authorization header %q", gotAuthorization)
	}
	if gotIdempotency != "provider-idem-1" {
		t.Fatalf("unexpected idempotency header %q", gotIdempotency)
	}
	encoded, _ := json.Marshal(gotPayload)
	payload := strings.ToLower(string(encoded))
	for _, marker := range []string{"destination_ref", "destination_hash", "secret_payload", "ciphertext", "provider_body", "authorization", "raw@example.com"} {
		if strings.Contains(payload, marker) {
			t.Fatalf("webhook payload leaked forbidden marker %q: %s", marker, payload)
		}
	}
	if gotPayload["destination_masked"] != "r***@example.com" {
		t.Fatalf("masked destination missing from payload: %+v", gotPayload)
	}
}

func TestWebhookProviderClassifiesNonSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "provider body must not persist", http.StatusBadRequest)
	}))
	defer server.Close()
	provider, err := NewWebhookProvider(WebhookConfig{URL: server.URL, ProviderID: "webhook-fixture"})
	if err != nil {
		t.Fatalf("new webhook provider: %v", err)
	}
	_, err = provider.Send(context.Background(), webhookDeliveryRequest())
	if err == nil {
		t.Fatal("expected provider error")
	}
	failure := NewWebhookFailureClassifier().ClassifyProviderError(err)
	if failure.FailureClass != types.FailureClassProviderRejected || failure.PublicError != types.PublicErrorProviderRejected || !failure.Permanent {
		t.Fatalf("unexpected failure classification: %+v", failure)
	}
}

func TestWebhookProviderClassifiesRetryableStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "10")
		http.Error(writer, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()
	provider, err := NewWebhookProvider(WebhookConfig{URL: server.URL, ProviderID: "webhook-fixture"})
	if err != nil {
		t.Fatalf("new webhook provider: %v", err)
	}
	_, err = provider.Send(context.Background(), webhookDeliveryRequest())
	if err == nil {
		t.Fatal("expected provider error")
	}
	failure := NewWebhookFailureClassifier().ClassifyProviderError(err)
	if failure.FailureClass != types.FailureClassProviderUnavailable || failure.PublicError != types.PublicErrorProviderUnavailable || failure.Permanent {
		t.Fatalf("unexpected failure classification: %+v", failure)
	}
	if failure.RetryAfter.IsZero() {
		t.Fatal("retry-after should be propagated as retry time")
	}
}

func TestWebhookProviderRequiresHTTPURL(t *testing.T) {
	if _, err := NewWebhookProvider(WebhookConfig{}); err == nil {
		t.Fatal("empty URL should fail")
	}
	if _, err := NewWebhookProvider(WebhookConfig{URL: "file:///tmp/provider"}); err == nil {
		t.Fatal("unsupported URL scheme should fail")
	}
}

func webhookDeliveryRequest() types.DeliveryRequest {
	return types.DeliveryRequest{
		NotificationRequest: types.NotificationRequest{
			TenantID:              "tenant-provider-test",
			RequestID:             "notif-provider-1",
			RequesterService:      "identity-service",
			Channel:               types.ChannelEmail,
			RecipientRef:          "user:user-1",
			DestinationHash:       "hash-raw@example.com",
			DestinationMasked:     "r***@example.com",
			TemplateKey:           "identity.challenge",
			TemplateVersion:       "v1",
			Locale:                "zh-CN",
			Priority:              types.PriorityNormal,
			TemplateVariablesJSON: `{"purpose":"smoke"}`,
			CorrelationID:         "corr-1",
			CausationID:           "cause-1",
			TraceID:               "trace-1",
		},
		AttemptNumber:          1,
		ProviderID:             "webhook-fixture",
		ProviderIdempotencyKey: "provider-idem-1",
	}
}
