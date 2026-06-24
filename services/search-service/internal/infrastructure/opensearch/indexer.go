package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

type Indexer struct {
	endpoint   *url.URL
	index      string
	username   string
	password   string
	apiKey     string
	httpClient *http.Client
}

func NewIndexer(config Config) (*Indexer, error) {
	endpoint, err := parseEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	index := strings.TrimSpace(config.Index)
	if err := validateIndexName(index); err != nil {
		return nil, err
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(config.Username) != "" && strings.TrimSpace(config.Password) == "" {
		return nil, fmt.Errorf("NEXUSIM_SEARCH_OPENSEARCH_PASSWORD is required when username is set")
	}
	return &Indexer{
		endpoint:   endpoint,
		index:      index,
		username:   strings.TrimSpace(config.Username),
		password:   config.Password,
		apiKey:     strings.TrimSpace(config.APIKey),
		httpClient: httpClient,
	}, nil
}

func (indexer *Indexer) EnsureSearchIndex(ctx context.Context) error {
	body := map[string]any{
		"settings": map[string]any{
			"index": map[string]any{
				"number_of_shards":   1,
				"number_of_replicas": 0,
			},
		},
		"mappings": map[string]any{
			"properties": map[string]any{
				"tenant_id":        map[string]any{"type": "keyword"},
				"conversation_id":  map[string]any{"type": "keyword"},
				"message_id":       map[string]any{"type": "keyword"},
				"conversation_seq": map[string]any{"type": "long"},
				"searchable_text":  map[string]any{"type": "text"},
			},
		},
	}
	status, responseBody, err := indexer.doJSON(ctx, http.MethodPut, "/"+url.PathEscape(indexer.index), body)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusCreated {
		return nil
	}
	if status == http.StatusBadRequest && strings.Contains(responseBody, "resource_already_exists_exception") {
		return nil
	}
	return types.NewSearchUnavailable(fmt.Sprintf("opensearch create index returned status %d", status))
}

func (indexer *Indexer) IndexSearchDocuments(ctx context.Context, documents []types.SearchIndexDocument) error {
	if len(documents) == 0 {
		return nil
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	for _, document := range documents {
		documentID := string(document.TenantID) + ":" + string(document.ConversationID) + ":" + document.MessageID
		action := map[string]any{
			"index": map[string]any{
				"_index": indexer.index,
				"_id":    documentID,
			},
		}
		if err := encoder.Encode(action); err != nil {
			return types.NewSearchUnavailable("encode opensearch bulk action failed")
		}
		payload := map[string]any{
			"tenant_id":        document.TenantID,
			"conversation_id":  document.ConversationID,
			"message_id":       document.MessageID,
			"conversation_seq": document.ConversationSeq,
			"searchable_text":  document.SearchableText,
		}
		if err := encoder.Encode(payload); err != nil {
			return types.NewSearchUnavailable("encode opensearch bulk document failed")
		}
	}
	status, responseBody, err := indexer.do(ctx, http.MethodPost, "/_bulk", "application/x-ndjson", &body)
	if err != nil {
		return err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return types.NewSearchUnavailable(fmt.Sprintf("opensearch bulk returned status %d", status))
	}
	var decoded struct {
		Errors bool `json:"errors"`
	}
	if err := json.Unmarshal([]byte(responseBody), &decoded); err != nil {
		return types.NewSearchUnavailable("decode opensearch bulk response failed")
	}
	if decoded.Errors {
		return types.NewSearchUnavailable("opensearch bulk response contains item errors")
	}
	return nil
}

func (indexer *Indexer) RefreshSearchIndex(ctx context.Context) error {
	status, _, err := indexer.do(ctx, http.MethodPost, "/"+url.PathEscape(indexer.index)+"/_refresh", "", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return types.NewSearchUnavailable(fmt.Sprintf("opensearch refresh returned status %d", status))
	}
	return nil
}

func (indexer *Indexer) doJSON(ctx context.Context, method string, path string, body any) (int, string, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, "", types.NewSearchUnavailable("encode opensearch request failed")
	}
	return indexer.do(ctx, method, path, "application/json", bytes.NewReader(encoded))
}

func (indexer *Indexer) do(ctx context.Context, method string, path string, contentType string, body io.Reader) (int, string, error) {
	request, err := http.NewRequestWithContext(ctx, method, indexer.url(path), body)
	if err != nil {
		return 0, "", types.NewSearchUnavailable("build opensearch request failed")
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if indexer.apiKey != "" {
		request.Header.Set("Authorization", "ApiKey "+indexer.apiKey)
	} else if indexer.username != "" {
		request.SetBasicAuth(indexer.username, indexer.password)
	}
	response, err := indexer.httpClient.Do(request)
	if err != nil {
		return 0, "", types.NewSearchUnavailable("opensearch request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return response.StatusCode, "", types.NewSearchUnavailable("read opensearch response failed")
	}
	return response.StatusCode, string(responseBody), nil
}

func (indexer *Indexer) url(path string) string {
	endpoint := *indexer.endpoint
	basePath := strings.TrimRight(endpoint.Path, "/")
	if strings.HasPrefix(path, "/") {
		endpoint.Path = basePath + path
	} else {
		endpoint.Path = basePath + "/" + path
	}
	return endpoint.String()
}
