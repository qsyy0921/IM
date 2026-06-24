package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const openSearchMappingReadLimit = 1 << 20

func preflightOpenSearchVector(ctx context.Context, cfg config, result *summary) error {
	base, err := parseOpenSearchEndpoint(cfg.openSearchVectorEndpoint)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: cfg.requestTimeout}
	if client.Timeout <= 0 {
		client.Timeout = 5 * time.Second
	}

	if err := checkOpenSearchRoot(ctx, client, base); err != nil {
		return err
	}
	result.OpenSearchVectorAvailable = true

	exists, err := checkOpenSearchIndexExists(ctx, client, base, cfg.openSearchVectorIndex)
	if err != nil {
		return err
	}
	result.OpenSearchVectorIndexExists = exists
	if !exists {
		return fmt.Errorf("opensearch vector index %q does not exist", cfg.openSearchVectorIndex)
	}

	fieldType, dimension, err := readOpenSearchVectorField(ctx, client, base, cfg.openSearchVectorIndex, cfg.openSearchVectorField)
	if err != nil {
		return err
	}
	result.OpenSearchVectorFieldType = fieldType
	result.OpenSearchVectorDimension = dimension
	if fieldType != "knn_vector" {
		return fmt.Errorf("opensearch vector field %q has type %q, expected knn_vector", cfg.openSearchVectorField, fieldType)
	}
	if dimension != cfg.openSearchVectorDimension {
		return fmt.Errorf("opensearch vector field %q dimension=%d, expected %d", cfg.openSearchVectorField, dimension, cfg.openSearchVectorDimension)
	}
	result.OpenSearchVectorMappingVerified = true
	return nil
}

func checkOpenSearchRoot(ctx context.Context, client *http.Client, base *url.URL) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nexusim-vector-preflight")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect opensearch vector endpoint: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, openSearchMappingReadLimit))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("opensearch vector endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

func checkOpenSearchIndexExists(ctx context.Context, client *http.Client, base *url.URL, index string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, openSearchURL(base, index), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "nexusim-vector-preflight")
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("check opensearch vector index: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, openSearchMappingReadLimit))
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("opensearch vector index check returned status %d", resp.StatusCode)
	}
}

func readOpenSearchVectorField(ctx context.Context, client *http.Client, base *url.URL, index string, fieldPath string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openSearchURL(base, index, "_mapping"), nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nexusim-vector-preflight")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("read opensearch vector mapping: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, openSearchMappingReadLimit))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("opensearch vector mapping returned status %d", resp.StatusCode)
	}
	if readErr != nil {
		return "", 0, fmt.Errorf("read opensearch vector mapping body: %w", readErr)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", 0, fmt.Errorf("decode opensearch vector mapping: %w", err)
	}
	field, err := findOpenSearchFieldMapping(payload, index, fieldPath)
	if err != nil {
		return "", 0, err
	}
	fieldType, _ := field["type"].(string)
	dimension, err := parseOpenSearchDimension(field["dimension"])
	if err != nil {
		return fieldType, 0, err
	}
	return fieldType, dimension, nil
}

func findOpenSearchFieldMapping(payload map[string]any, index string, fieldPath string) (map[string]any, error) {
	indexPayload, ok := payload[index].(map[string]any)
	if !ok {
		if len(payload) == 1 {
			for _, value := range payload {
				indexPayload, ok = value.(map[string]any)
				break
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("opensearch mapping missing index %q", index)
	}
	mappings, ok := indexPayload["mappings"].(map[string]any)
	if !ok {
		return nil, errors.New("opensearch mapping missing mappings object")
	}
	properties, ok := mappings["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("opensearch mapping missing properties object")
	}
	current := properties
	parts := strings.Split(fieldPath, ".")
	for index, part := range parts {
		next, ok := current[part].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("opensearch mapping missing vector field %q", fieldPath)
		}
		if index == len(parts)-1 {
			return next, nil
		}
		nested, ok := next["properties"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("opensearch mapping path %q has no nested properties", strings.Join(parts[:index+1], "."))
		}
		current = nested
	}
	return nil, fmt.Errorf("opensearch mapping missing vector field %q", fieldPath)
}

func parseOpenSearchDimension(value any) (int, error) {
	switch typed := value.(type) {
	case float64:
		if typed <= 0 || typed != float64(int(typed)) {
			return 0, fmt.Errorf("invalid opensearch vector dimension %v", value)
		}
		return int(typed), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("invalid opensearch vector dimension %v", value)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid opensearch vector dimension %v", value)
	}
}

func validateOpenSearchEndpoint(value string) error {
	_, err := parseOpenSearchEndpoint(value)
	return err
}

func parseOpenSearchEndpoint(value string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("opensearch vector endpoint is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid opensearch vector endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("opensearch vector endpoint must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("opensearch vector endpoint must include host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("opensearch vector endpoint must not include credentials, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed, nil
}

func safeOpenSearchEndpointForSummary(value string) string {
	parsed, err := parseOpenSearchEndpoint(value)
	if err != nil {
		return ""
	}
	return parsed.String()
}

func openSearchURL(base *url.URL, parts ...string) string {
	next := *base
	path := strings.TrimRight(next.Path, "/")
	for _, part := range parts {
		path += "/" + url.PathEscape(part)
	}
	next.Path = path
	next.RawPath = ""
	return next.String()
}

func isSafeOpenSearchIndexName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || value == "." || value == ".." {
		return false
	}
	first := value[0]
	if first == '_' || first == '-' || first == '+' {
		return false
	}
	if strings.Contains(value, "..") {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func isSafeOpenSearchFieldPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if !isSafeSQLIdentifier(part) {
			return false
		}
	}
	return true
}
