package llmboundary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultMaxResponseBytes int64 = 64 * 1024

var (
	ErrProviderUnavailable      = errors.New("llm provider unavailable")
	ErrProviderRateLimited      = errors.New("llm provider rate limited")
	ErrProviderPermissionDenied = errors.New("llm provider permission denied")
)

type HTTPClientOptions struct {
	Endpoint         string
	BearerToken      string
	Timeout          time.Duration
	MaxResponseBytes int64
}

type HTTPClient struct {
	endpoint         string
	bearerToken      string
	client           *http.Client
	maxResponseBytes int64
}

func NewHTTPClient(options HTTPClientOptions) (HTTPClient, error) {
	endpoint, err := validateEndpoint(options.Endpoint)
	if err != nil {
		return HTTPClient{}, err
	}
	if ContainsSensitiveText(endpoint) {
		return HTTPClient{}, fmt.Errorf("%w: endpoint contains sensitive value", ErrUnsafeInput)
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = DefaultMaxResponseBytes
	}
	return HTTPClient{
		endpoint:         endpoint,
		bearerToken:      strings.TrimSpace(options.BearerToken),
		client:           &http.Client{Timeout: timeout},
		maxResponseBytes: maxResponseBytes,
	}, nil
}

func (client HTTPClient) GenerateCandidate(ctx context.Context, prompt Prompt) (Candidate, error) {
	if client.client == nil || strings.TrimSpace(client.endpoint) == "" {
		return Candidate{}, ErrProviderUnavailable
	}
	body, err := json.Marshal(prompt)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: encode request", ErrProviderUnavailable)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: build request", ErrProviderUnavailable)
	}
	request.Header.Set("Content-Type", "application/json")
	if client.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+client.bearerToken)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: request failed", ErrProviderUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return Candidate{}, ErrProviderPermissionDenied
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return Candidate{}, ErrProviderRateLimited
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Candidate{}, ErrProviderUnavailable
	}
	limited := io.LimitReader(response.Body, client.maxResponseBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: read response", ErrProviderUnavailable)
	}
	if int64(len(payload)) > client.maxResponseBytes {
		return Candidate{}, fmt.Errorf("%w: response too large", ErrMalformedOutput)
	}
	var candidate Candidate
	if err := json.Unmarshal(payload, &candidate); err != nil {
		return Candidate{}, fmt.Errorf("%w: decode response", ErrMalformedOutput)
	}
	return candidate, nil
}

func validateEndpoint(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("llm endpoint is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("llm endpoint must use http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("llm endpoint host is required")
	}
	if parsed.Scheme == "http" && !isLocalOrPrivateHost(parsed.Hostname()) {
		return "", errors.New("http llm endpoint must be loopback or private")
	}
	return parsed.String(), nil
}

func isLocalOrPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
