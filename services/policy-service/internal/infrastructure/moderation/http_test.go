package moderation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestHTTPModeratorDeniesProviderDecision(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing provider authorization header: %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		_, _ = w.Write([]byte(`{
			"allowed": false,
			"permission_version": 41,
			"classification": "PROVIDER_RISK",
			"reason": "provider denied"
		}`))
	}))
	defer server.Close()

	moderator, err := NewHTTPModerator(HTTPConfig{
		Endpoint:    server.URL,
		BearerToken: "test-token",
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new http moderator: %v", err)
	}
	decision, handled, err := moderator.ModerateMessageContent(context.Background(), samplePolicyModerationCommand("hello provider"))
	if err != nil {
		t.Fatalf("moderate message content: %v", err)
	}
	if !handled || decision.Allowed || decision.PermissionVersion != 41 ||
		decision.Classification != "PROVIDER_RISK" || decision.Reason != "provider denied" {
		t.Fatalf("unexpected provider decision: handled=%v decision=%+v", handled, decision)
	}
	if captured["message_text"] != "hello provider" ||
		captured["tenant_id"] != "tenant-1" ||
		captured["user_id"] != "user-1" ||
		captured["conversation_id"] != "conv-1" ||
		captured["action"] != string(types.MessageActionSend) {
		t.Fatalf("unexpected provider request: %+v", captured)
	}
}

func TestHTTPModeratorAllowsProviderDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"allowed": true}`))
	}))
	defer server.Close()

	moderator, err := NewHTTPModerator(HTTPConfig{Endpoint: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new http moderator: %v", err)
	}
	decision, handled, err := moderator.ModerateMessageContent(context.Background(), samplePolicyModerationCommand("allowed"))
	if err != nil {
		t.Fatalf("moderate message content: %v", err)
	}
	if handled || decision.Classification != "" {
		t.Fatalf("expected allowed provider decision to fall through, handled=%v decision=%+v", handled, decision)
	}
}

func TestHTTPModeratorProviderFailureUsesStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider body token=secret", http.StatusInternalServerError)
	}))
	defer server.Close()

	moderator, err := NewHTTPModerator(HTTPConfig{Endpoint: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new http moderator: %v", err)
	}
	_, _, err = moderator.ModerateMessageContent(context.Background(), samplePolicyModerationCommand("blocked"))
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "provider body") {
		t.Fatalf("provider error leaked raw body: %v", err)
	}
}

func TestHTTPModeratorSkipsEmptyText(t *testing.T) {
	moderator, err := NewHTTPModerator(HTTPConfig{Endpoint: "https://moderation.example.test"})
	if err != nil {
		t.Fatalf("new http moderator: %v", err)
	}
	_, handled, err := moderator.ModerateMessageContent(context.Background(), samplePolicyModerationCommand("   "))
	if err != nil || handled {
		t.Fatalf("expected empty text to skip provider call, handled=%v err=%v", handled, err)
	}
}

func samplePolicyModerationCommand(text string) types.CheckMessageActionCommand {
	return types.CheckMessageActionCommand{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		ConversationID: "conv-1",
		Action:         types.MessageActionSend,
		MessageText:    text,
	}
}
