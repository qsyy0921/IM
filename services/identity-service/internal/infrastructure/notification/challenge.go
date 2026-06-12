package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type NoopChallengeNotifier struct{}

func NewNoopChallengeNotifier() NoopChallengeNotifier {
	return NoopChallengeNotifier{}
}

func (NoopChallengeNotifier) SendChallenge(context.Context, types.ChallengeNotification) error {
	return nil
}

type WebhookChallengeNotifier struct {
	url         string
	bearerToken string
	client      *http.Client
}

func NewWebhookChallengeNotifier(url string, bearerToken string, timeout time.Duration) (*WebhookChallengeNotifier, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("identity challenge webhook url is required")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookChallengeNotifier{
		url:         url,
		bearerToken: strings.TrimSpace(bearerToken),
		client:      &http.Client{Timeout: timeout},
	}, nil
}

func (notifier *WebhookChallengeNotifier) SendChallenge(ctx context.Context, notification types.ChallengeNotification) error {
	if notifier == nil || notifier.client == nil || notifier.url == "" {
		return types.NewChallengeDeliveryFailed("identity challenge notifier is not configured")
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		return types.NewChallengeDeliveryFailed(err.Error())
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, notifier.url, bytes.NewReader(payload))
	if err != nil {
		return types.NewChallengeDeliveryFailed(err.Error())
	}
	request.Header.Set("Content-Type", "application/json")
	if notification.RequestID != "" {
		request.Header.Set("X-NexusIM-Request-ID", notification.RequestID)
	}
	if notifier.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+notifier.bearerToken)
	}
	response, err := notifier.client.Do(request)
	if err != nil {
		return types.NewChallengeDeliveryFailed(err.Error())
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return types.NewChallengeDeliveryFailed("identity challenge webhook returned non-success status")
	}
	return nil
}
