package llmboundary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClientPostsPromptAndDecodesCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization header not set")
		}
		var prompt Prompt
		if err := json.NewDecoder(request.Body).Decode(&prompt); err != nil {
			t.Fatalf("decode prompt: %v", err)
		}
		if prompt.Task != "answer" || len(prompt.Evidence) != 1 {
			t.Fatalf("unexpected prompt: %+v", prompt)
		}
		_ = json.NewEncoder(writer).Encode(Candidate{
			Text:                "grounded",
			CitationEvidenceIDs: []string{"e1"},
			Confidence:          0.7,
		})
	}))
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientOptions{
		Endpoint:    server.URL,
		BearerToken: "test-token",
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}
	candidate, err := client.GenerateCandidate(context.Background(), Prompt{
		Task:        "answer",
		Query:       "query",
		TokenBudget: 100,
		Evidence:    []Evidence{{EvidenceID: "e1", Text: "text"}},
	})
	if err != nil {
		t.Fatalf("GenerateCandidate returned error: %v", err)
	}
	if candidate.Text != "grounded" || candidate.CitationEvidenceIDs[0] != "e1" {
		t.Fatalf("unexpected candidate: %+v", candidate)
	}
}

func TestHTTPClientClassifiesProviderErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   error
	}{
		{name: "permission", statusCode: http.StatusForbidden, expected: ErrProviderPermissionDenied},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, expected: ErrProviderRateLimited},
		{name: "unavailable", statusCode: http.StatusBadGateway, expected: ErrProviderUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(tt.statusCode)
			}))
			defer server.Close()
			client, err := NewHTTPClient(HTTPClientOptions{Endpoint: server.URL})
			if err != nil {
				t.Fatalf("NewHTTPClient returned error: %v", err)
			}
			_, err = client.GenerateCandidate(context.Background(), Prompt{Task: "answer"})
			if !errors.Is(err, tt.expected) {
				t.Fatalf("expected %v, got %v", tt.expected, err)
			}
		})
	}
}

func TestHTTPClientRejectsPublicPlainHTTP(t *testing.T) {
	_, err := NewHTTPClient(HTTPClientOptions{Endpoint: "http://example.com/llm"})
	if err == nil {
		t.Fatal("expected public plain HTTP endpoint rejection")
	}
}

func TestHTTPClientRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("{"))
	}))
	defer server.Close()
	client, err := NewHTTPClient(HTTPClientOptions{Endpoint: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}
	_, err = client.GenerateCandidate(context.Background(), Prompt{Task: "answer"})
	if !errors.Is(err, ErrMalformedOutput) {
		t.Fatalf("expected malformed output, got %v", err)
	}
}
