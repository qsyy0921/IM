package objectstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

func TestFakeStoreDoesNotExposeObjectKey(t *testing.T) {
	store := NewFakeStore("http://media.local")
	objectKey := "tenant-1/conv-1/object-secret-key"

	putURL, err := store.PresignPut(context.Background(), objectKey, types.ObjectMetadata{
		SizeBytes: 12,
		SHA256:    strings.Repeat("a", 64),
	}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("presign put: %v", err)
	}
	getURL, err := store.PresignGet(context.Background(), objectKey, types.VariantOriginal, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}

	for _, rawURL := range []string{putURL.URL, getURL.URL} {
		if strings.Contains(rawURL, objectKey) || strings.Contains(rawURL, "object-secret-key") || strings.Contains(rawURL, "key=") {
			t.Fatalf("presigned URL leaked object key: %s", rawURL)
		}
		if !strings.Contains(rawURL, "token=") {
			t.Fatalf("presigned URL should carry opaque token: %s", rawURL)
		}
	}
}

func TestFakeHTTPHandlerAcceptsBrowserPut(t *testing.T) {
	handler := NewFakeHTTPHandler()
	request := httptest.NewRequest(http.MethodPut, "/media?op=put&token=opaque", strings.NewReader("avatar-bytes"))
	request.Header.Set("x-nexusim-media-mode", "fake")
	request.Header.Set("Content-Type", "image/png")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected CORS header, got %q", response.Header().Get("Access-Control-Allow-Origin"))
	}

	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/media?op=original&token=opaque", nil))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
	if getResponse.Body.String() != "avatar-bytes" {
		t.Fatalf("expected uploaded bytes, got %q", getResponse.Body.String())
	}
	if getResponse.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png content-type, got %q", getResponse.Header().Get("Content-Type"))
	}
}

func TestFakeHTTPHandlerRejectsMissingUploadHeader(t *testing.T) {
	handler := NewFakeHTTPHandler()
	request := httptest.NewRequest(http.MethodPut, "/media?op=put&token=opaque", strings.NewReader("avatar-bytes"))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got status=%d body=%s", response.Code, response.Body.String())
	}
}
