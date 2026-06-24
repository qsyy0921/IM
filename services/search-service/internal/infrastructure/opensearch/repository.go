package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/search-service/internal/types"
)

const (
	defaultCandidateOverfetchFactor = 5
	defaultMaxCandidateFetch        = 500
	defaultRequestTimeout           = 2 * time.Second
)

type CandidateHydrator interface {
	SearchMessagesByCandidates(
		ctx context.Context,
		command types.SearchMessagesCommand,
		candidates []types.SearchMessageCandidate,
		fetchLimit int,
	) ([]types.SearchMessageHit, int64, error)
}

type Config struct {
	Endpoint                 string
	Index                    string
	Username                 string
	Password                 string
	APIKey                   string
	Timeout                  time.Duration
	CandidateOverfetchFactor int
	MaxCandidateFetch        int
	HTTPClient               *http.Client
}

type Repository struct {
	endpoint                 *url.URL
	index                    string
	username                 string
	password                 string
	apiKey                   string
	timeout                  time.Duration
	candidateOverfetchFactor int
	maxCandidateFetch        int
	httpClient               *http.Client
	hydrator                 CandidateHydrator
}

func NewRepository(config Config, hydrator CandidateHydrator) (*Repository, error) {
	if hydrator == nil {
		return nil, errors.New("opensearch candidate hydrator is required")
	}
	endpoint, err := parseEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	index := strings.TrimSpace(config.Index)
	if err := validateIndexName(index); err != nil {
		return nil, err
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	overfetchFactor := config.CandidateOverfetchFactor
	if overfetchFactor <= 0 {
		overfetchFactor = defaultCandidateOverfetchFactor
	}
	maxCandidateFetch := config.MaxCandidateFetch
	if maxCandidateFetch <= 0 {
		maxCandidateFetch = defaultMaxCandidateFetch
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(config.Username) != "" && strings.TrimSpace(config.Password) == "" {
		return nil, errors.New("NEXUSIM_SEARCH_OPENSEARCH_PASSWORD is required when username is set")
	}
	return &Repository{
		endpoint:                 endpoint,
		index:                    index,
		username:                 strings.TrimSpace(config.Username),
		password:                 config.Password,
		apiKey:                   strings.TrimSpace(config.APIKey),
		timeout:                  timeout,
		candidateOverfetchFactor: overfetchFactor,
		maxCandidateFetch:        maxCandidateFetch,
		httpClient:               httpClient,
		hydrator:                 hydrator,
	}, nil
}

func (repository *Repository) SearchMessages(
	ctx context.Context,
	command types.SearchMessagesCommand,
	fetchLimit int,
) ([]types.SearchMessageHit, int64, error) {
	if fetchLimit <= 0 {
		return nil, 0, nil
	}
	candidateLimit := fetchLimit * repository.candidateOverfetchFactor
	if candidateLimit < fetchLimit {
		candidateLimit = fetchLimit
	}
	if candidateLimit > repository.maxCandidateFetch {
		candidateLimit = repository.maxCandidateFetch
	}
	candidates, err := repository.searchCandidates(ctx, command, candidateLimit)
	if err != nil {
		return nil, 0, err
	}
	if len(candidates) == 0 {
		return nil, 0, nil
	}
	return repository.hydrator.SearchMessagesByCandidates(ctx, command, candidates, fetchLimit)
}

func (repository *Repository) searchCandidates(
	ctx context.Context,
	command types.SearchMessagesCommand,
	candidateLimit int,
) ([]types.SearchMessageCandidate, error) {
	requestCtx := ctx
	cancel := func() {}
	if repository.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, repository.timeout)
	}
	defer cancel()

	body, err := json.Marshal(buildSearchRequest(command, candidateLimit))
	if err != nil {
		return nil, types.NewSearchUnavailable("build opensearch request failed")
	}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, repository.searchURL(), bytes.NewReader(body))
	if err != nil {
		return nil, types.NewSearchUnavailable("build opensearch request failed")
	}
	request.Header.Set("Content-Type", "application/json")
	if repository.apiKey != "" {
		request.Header.Set("Authorization", "ApiKey "+repository.apiKey)
	} else if repository.username != "" {
		request.SetBasicAuth(repository.username, repository.password)
	}
	response, err := repository.httpClient.Do(request)
	if err != nil {
		return nil, types.NewSearchUnavailable("opensearch request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return nil, types.NewSearchUnavailable(fmt.Sprintf("opensearch returned status %d", response.StatusCode))
	}

	var decoded searchResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4*1024*1024))
	if err := decoder.Decode(&decoded); err != nil {
		return nil, types.NewSearchUnavailable("decode opensearch response failed")
	}
	candidates := make([]types.SearchMessageCandidate, 0, len(decoded.Hits.Hits))
	seen := make(map[string]struct{}, len(decoded.Hits.Hits))
	for _, hit := range decoded.Hits.Hits {
		conversationID := strings.TrimSpace(hit.Source.ConversationID)
		messageID := strings.TrimSpace(hit.Source.MessageID)
		if conversationID == "" || messageID == "" {
			return nil, types.NewSearchUnavailable("malformed opensearch hit source")
		}
		key := conversationID + "\x00" + messageID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, types.SearchMessageCandidate{
			ConversationID: types.ConversationID(conversationID),
			MessageID:      messageID,
		})
	}
	return candidates, nil
}

func buildSearchRequest(command types.SearchMessagesCommand, candidateLimit int) map[string]any {
	filters := []any{
		map[string]any{"term": map[string]any{"tenant_id": string(command.AuthContext.TenantID)}},
		map[string]any{"range": map[string]any{"conversation_seq": map[string]any{"gt": command.AfterSeq}}},
	}
	if command.ConversationID != "" {
		filters = append(filters, map[string]any{"term": map[string]any{"conversation_id": string(command.ConversationID)}})
	}
	return map[string]any{
		"size":             candidateLimit,
		"track_total_hits": false,
		"_source":          []string{"conversation_id", "message_id"},
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{
					map[string]any{
						"match": map[string]any{
							"searchable_text": map[string]any{
								"query":    command.NormalizedQuery(),
								"operator": "and",
							},
						},
					},
				},
				"filter": filters,
			},
		},
	}
}

func (repository *Repository) searchURL() string {
	endpoint := *repository.endpoint
	basePath := strings.TrimRight(endpoint.Path, "/")
	endpoint.Path = basePath + "/" + url.PathEscape(repository.index) + "/_search"
	query := endpoint.Query()
	query.Set("allow_partial_search_results", "false")
	endpoint.RawQuery = query.Encode()
	return endpoint.String()
}

type searchResponse struct {
	Hits struct {
		Hits []struct {
			Source struct {
				ConversationID string `json:"conversation_id"`
				MessageID      string `json:"message_id"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func parseEndpoint(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT host is required")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT must not include credentials, query, or fragment")
	}
	return parsed, nil
}

func validateIndexName(index string) error {
	if index == "" {
		return errors.New("NEXUSIM_SEARCH_OPENSEARCH_INDEX is required")
	}
	if strings.ContainsAny(index, "/*?,#\\ \t\r\n") {
		return errors.New("NEXUSIM_SEARCH_OPENSEARCH_INDEX contains unsupported characters")
	}
	return nil
}
