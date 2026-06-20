package objectstore

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

type FakeStore struct {
	BaseURL string
}

func NewFakeStore(baseURL string) FakeStore {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:19080/media"
	}
	return FakeStore{BaseURL: baseURL}
}

func (store FakeStore) PresignPut(_ context.Context, objectKey string, _ types.ObjectMetadata, expiresAt time.Time) (types.PresignedURL, error) {
	return types.PresignedURL{
		URL:             store.objectURL(objectKey, "put"),
		RequiredHeaders: map[string]string{"x-nexusim-media-mode": "fake"},
		ExpiresAt:       expiresAt,
	}, nil
}

func (store FakeStore) VerifyUploadedObject(_ context.Context, _ string, expected types.ObjectMetadata) (types.ObjectMetadata, error) {
	if expected.SizeBytes <= 0 || expected.SHA256 == "" {
		return types.ObjectMetadata{}, types.NewProviderUnavailable("fake object metadata is incomplete")
	}
	return expected, nil
}

func (store FakeStore) PresignGet(_ context.Context, objectKey string, variant string, expiresAt time.Time) (types.PresignedURL, error) {
	return types.PresignedURL{
		URL:             store.objectURL(objectKey, strings.ToLower(variant)),
		RequiredHeaders: map[string]string{},
		ExpiresAt:       expiresAt,
	}, nil
}

func (store FakeStore) objectURL(objectKey string, operation string) string {
	values := url.Values{}
	values.Set("op", operation)
	values.Set("key", objectKey)
	return store.BaseURL + "?" + values.Encode()
}
