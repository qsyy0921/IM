package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/notification-service/internal/types"
)

type WebhookConfig struct {
	URL         string
	BearerToken string
	ProviderID  string
	Timeout     time.Duration
}

type WebhookProvider struct {
	url         string
	bearerToken string
	providerID  string
	client      *http.Client
}

type WebhookFailureClassifier struct{}

type providerError struct {
	failure types.DeliveryFailure
}

func (err providerError) Error() string {
	if strings.TrimSpace(err.failure.PublicError) != "" {
		return err.failure.PublicError
	}
	return types.PublicErrorProviderUnavailable
}

func NewWebhookProvider(config WebhookConfig) (*WebhookProvider, error) {
	endpoint := strings.TrimSpace(config.URL)
	if endpoint == "" {
		return nil, errors.New("notification webhook provider url is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("notification webhook provider url is invalid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return nil, errors.New("notification webhook provider url must be http or https")
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	providerID := strings.TrimSpace(config.ProviderID)
	if providerID == "" {
		providerID = "webhook"
	}
	return &WebhookProvider{
		url:         endpoint,
		bearerToken: strings.TrimSpace(config.BearerToken),
		providerID:  providerID,
		client:      &http.Client{Timeout: timeout},
	}, nil
}

func NewWebhookFailureClassifier() WebhookFailureClassifier {
	return WebhookFailureClassifier{}
}

func (classifier WebhookFailureClassifier) ClassifyProviderError(err error) types.DeliveryFailure {
	var providerErr providerError
	if errors.As(err, &providerErr) {
		return providerErr.failure
	}
	return types.NewProviderUnavailableFailure()
}

func (provider *WebhookProvider) Send(ctx context.Context, request types.DeliveryRequest) (types.DeliveryResult, error) {
	if provider == nil || provider.client == nil || strings.TrimSpace(provider.url) == "" {
		return types.DeliveryResult{}, providerError{failure: types.NewProviderUnavailableFailure()}
	}
	payload, err := json.Marshal(webhookPayloadFromRequest(request))
	if err != nil {
		return types.DeliveryResult{}, providerError{failure: types.NewProviderUnavailableFailure()}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.url, bytes.NewReader(payload))
	if err != nil {
		return types.DeliveryResult{}, providerError{failure: types.NewProviderUnavailableFailure()}
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-NexusIM-Request-ID", request.RequestID)
	httpRequest.Header.Set("X-NexusIM-Provider-ID", provider.providerID)
	httpRequest.Header.Set("X-NexusIM-Provider-Idempotency-Key", request.ProviderIdempotencyKey)
	if provider.bearerToken != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+provider.bearerToken)
	}
	response, err := provider.client.Do(httpRequest)
	if err != nil {
		return types.DeliveryResult{}, providerError{failure: types.NewProviderUnavailableFailure()}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return types.DeliveryResult{}, providerError{failure: providerFailureFromStatus(response.StatusCode, response.Header.Get("Retry-After"))}
	}
	messageID := providerMessageIDFromBody(body)
	if messageID == "" {
		messageID = provider.providerID + ":" + string(request.TenantID) + ":" + request.RequestID + ":" + request.ProviderIdempotencyKey
	}
	return types.DeliveryResult{
		ProviderID:            provider.providerID,
		ProviderMessageIDHash: hashProviderMessageID(provider.providerID, messageID),
	}, nil
}

type webhookPayload struct {
	TenantID               string          `json:"tenant_id"`
	RequestID              string          `json:"request_id"`
	RequesterService       string          `json:"requester_service"`
	Channel                string          `json:"channel"`
	RecipientRef           string          `json:"recipient_ref"`
	DestinationMasked      string          `json:"destination_masked"`
	TemplateKey            string          `json:"template_key"`
	TemplateVersion        string          `json:"template_version"`
	Locale                 string          `json:"locale"`
	Priority               string          `json:"priority"`
	TemplateVariables      json.RawMessage `json:"template_variables"`
	ProviderID             string          `json:"provider_id"`
	ProviderIdempotencyKey string          `json:"provider_idempotency_key"`
	AttemptNumber          int             `json:"attempt_number"`
	CorrelationID          string          `json:"correlation_id,omitempty"`
	CausationID            string          `json:"causation_id,omitempty"`
	TraceID                string          `json:"trace_id,omitempty"`
}

func webhookPayloadFromRequest(request types.DeliveryRequest) webhookPayload {
	var variables json.RawMessage = []byte("{}")
	if raw := strings.TrimSpace(request.TemplateVariablesJSON); raw != "" {
		var decoded map[string]any
		if json.Unmarshal([]byte(raw), &decoded) == nil {
			variables = json.RawMessage(raw)
		}
	}
	return webhookPayload{
		TenantID:               string(request.TenantID),
		RequestID:              request.RequestID,
		RequesterService:       request.RequesterService,
		Channel:                request.Channel,
		RecipientRef:           request.RecipientRef,
		DestinationMasked:      request.DestinationMasked,
		TemplateKey:            request.TemplateKey,
		TemplateVersion:        request.TemplateVersion,
		Locale:                 request.Locale,
		Priority:               request.Priority,
		TemplateVariables:      variables,
		ProviderID:             request.ProviderID,
		ProviderIdempotencyKey: request.ProviderIdempotencyKey,
		AttemptNumber:          request.AttemptNumber,
		CorrelationID:          request.CorrelationID,
		CausationID:            request.CausationID,
		TraceID:                request.TraceID,
	}
}

func providerFailureFromStatus(statusCode int, retryAfter string) types.DeliveryFailure {
	failure := types.DeliveryFailure{
		FailureClass: types.FailureClassProviderRejected,
		PublicError:  types.PublicErrorProviderRejected,
		Permanent:    statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError && statusCode != http.StatusTooManyRequests,
	}
	if statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError {
		failure.FailureClass = types.FailureClassProviderUnavailable
		failure.PublicError = types.PublicErrorProviderUnavailable
		failure.Permanent = false
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		failure.RetryAfter = time.Now().UTC().Add(time.Duration(seconds) * time.Second)
	}
	return failure
}

func providerMessageIDFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var decoded struct {
		ProviderMessageID string `json:"provider_message_id"`
		MessageID         string `json:"message_id"`
		ID                string `json:"id"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ""
	}
	return firstNonEmpty(decoded.ProviderMessageID, decoded.MessageID, decoded.ID)
}

func hashProviderMessageID(providerID string, messageID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(providerID) + ":" + strings.TrimSpace(messageID)))
	return hex.EncodeToString(digest[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
