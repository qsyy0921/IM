package objectstore

import (
	"io"
	"net/http"
	"strings"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

func NewFakeHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/media", handleFakeObject)
	return mux
}

func handleFakeObject(response http.ResponseWriter, request *http.Request) {
	setFakeObjectCORS(response)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPut {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.URL.Query().Get("op") != "put" || strings.TrimSpace(request.URL.Query().Get("token")) == "" {
		http.Error(response, "invalid fake object upload url", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.Header.Get("x-nexusim-media-mode")) != "fake" {
		http.Error(response, "missing fake media upload header", http.StatusBadRequest)
		return
	}
	limited := http.MaxBytesReader(response, request.Body, types.DefaultMaxSizeBytes)
	defer limited.Close()
	if _, err := io.Copy(io.Discard, limited); err != nil {
		http.Error(response, "fake media upload body is invalid", http.StatusRequestEntityTooLarge)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func setFakeObjectCORS(response http.ResponseWriter) {
	response.Header().Set("Access-Control-Allow-Origin", "*")
	response.Header().Set("Access-Control-Allow-Methods", "PUT, OPTIONS")
	response.Header().Set("Access-Control-Allow-Headers", "x-nexusim-media-mode, content-type")
	response.Header().Set("Access-Control-Max-Age", "600")
}
