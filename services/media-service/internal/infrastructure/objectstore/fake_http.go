package objectstore

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

func NewFakeHTTPHandler() http.Handler {
	server := &fakeObjectServer{objects: make(map[string]fakeObject)}
	mux := http.NewServeMux()
	mux.HandleFunc("/media", server.handleFakeObject)
	return mux
}

type fakeObjectServer struct {
	mu      sync.RWMutex
	objects map[string]fakeObject
}

type fakeObject struct {
	body        []byte
	contentType string
}

func (server *fakeObjectServer) handleFakeObject(response http.ResponseWriter, request *http.Request) {
	setFakeObjectCORS(response)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	token := strings.TrimSpace(request.URL.Query().Get("token"))
	if token == "" {
		http.Error(response, "missing fake object token", http.StatusBadRequest)
		return
	}
	switch request.Method {
	case http.MethodPut:
		server.handleFakePut(response, request, token)
	case http.MethodGet:
		server.handleFakeGet(response, token)
	default:
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (server *fakeObjectServer) handleFakePut(response http.ResponseWriter, request *http.Request, token string) {
	if request.URL.Query().Get("op") != "put" {
		http.Error(response, "invalid fake object upload url", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.Header.Get("x-nexusim-media-mode")) != "fake" {
		http.Error(response, "missing fake media upload header", http.StatusBadRequest)
		return
	}
	limited := http.MaxBytesReader(response, request.Body, types.DefaultMaxSizeBytes)
	defer limited.Close()
	var body bytes.Buffer
	if _, err := io.Copy(&body, limited); err != nil {
		http.Error(response, "fake media upload body is invalid", http.StatusRequestEntityTooLarge)
		return
	}
	contentType := strings.TrimSpace(request.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	server.mu.Lock()
	server.objects[token] = fakeObject{body: body.Bytes(), contentType: contentType}
	server.mu.Unlock()
	response.WriteHeader(http.StatusNoContent)
}

func (server *fakeObjectServer) handleFakeGet(response http.ResponseWriter, token string) {
	server.mu.RLock()
	object, ok := server.objects[token]
	server.mu.RUnlock()
	if !ok {
		http.Error(response, "fake media object is missing", http.StatusNotFound)
		return
	}
	response.Header().Set("Content-Type", object.contentType)
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(object.body)
}

func setFakeObjectCORS(response http.ResponseWriter) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "PUT, GET, OPTIONS")
	response.Header().Set("Access-Control-Allow-Headers", "x-nexusim-media-mode, content-type")
	response.Header().Set("Access-Control-Max-Age", "600")
}
