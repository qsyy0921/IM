package main

import (
	"bytes"
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

const milvusResponseReadLimit = 1 << 20

func preflightMilvusVector(ctx context.Context, cfg config, result *summary) error {
	base, err := parseMilvusEndpoint(cfg.milvusEndpoint)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: cfg.requestTimeout}
	if client.Timeout <= 0 {
		client.Timeout = 5 * time.Second
	}

	exists, err := checkMilvusCollectionExists(ctx, client, base, cfg)
	if err != nil {
		return err
	}
	result.MilvusAvailable = true
	result.MilvusCollectionExists = exists
	if !exists {
		return fmt.Errorf("milvus collection %q does not exist", cfg.milvusCollection)
	}

	fieldType, dimension, err := readMilvusVectorField(ctx, client, base, cfg)
	if err != nil {
		return err
	}
	result.MilvusVectorFieldType = fieldType
	result.MilvusVectorDimension = dimension
	if !isMilvusDenseVectorType(fieldType) {
		return fmt.Errorf("milvus vector field %q has type %q, expected dense vector type", cfg.milvusVectorField, fieldType)
	}
	if dimension != cfg.milvusVectorDimension {
		return fmt.Errorf("milvus vector field %q dimension=%d, expected %d", cfg.milvusVectorField, dimension, cfg.milvusVectorDimension)
	}
	result.MilvusSchemaVerified = true
	return nil
}

func checkMilvusCollectionExists(ctx context.Context, client *http.Client, base *url.URL, cfg config) (bool, error) {
	payload := milvusCollectionRequest{
		DBName:         strings.TrimSpace(cfg.milvusDatabase),
		CollectionName: strings.TrimSpace(cfg.milvusCollection),
	}
	var response milvusHasCollectionResponse
	if err := callMilvusJSON(ctx, client, base, cfg, "/v2/vectordb/collections/has", payload, &response); err != nil {
		return false, err
	}
	return response.Data.Has, nil
}

func readMilvusVectorField(ctx context.Context, client *http.Client, base *url.URL, cfg config) (string, int, error) {
	payload := milvusCollectionRequest{
		DBName:         strings.TrimSpace(cfg.milvusDatabase),
		CollectionName: strings.TrimSpace(cfg.milvusCollection),
	}
	var response milvusDescribeCollectionResponse
	if err := callMilvusJSON(ctx, client, base, cfg, "/v2/vectordb/collections/describe", payload, &response); err != nil {
		return "", 0, err
	}
	fieldName := strings.TrimSpace(cfg.milvusVectorField)
	for _, field := range response.Data.Fields {
		if field.Name != fieldName {
			continue
		}
		dimension, err := milvusFieldDimension(field)
		if err != nil {
			return field.Type, 0, err
		}
		return field.Type, dimension, nil
	}
	return "", 0, fmt.Errorf("milvus collection %q missing vector field %q", cfg.milvusCollection, fieldName)
}

func callMilvusJSON(ctx context.Context, client *http.Client, base *url.URL, cfg config, path string, payload any, target any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, milvusURL(base, path), bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nexusim-vector-preflight")
	req.Header.Set("Request-Timeout", strconv.Itoa(int(client.Timeout.Seconds())))
	if token := strings.TrimSpace(cfg.milvusToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect milvus vector endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, milvusResponseReadLimit))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("milvus vector endpoint %s returned status %d", path, resp.StatusCode)
	}
	if readErr != nil {
		return fmt.Errorf("read milvus vector response body: %w", readErr)
	}
	var envelope struct {
		Code *int `json:"code"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode milvus vector response: %w", err)
	}
	if envelope.Code == nil {
		return fmt.Errorf("milvus vector endpoint %s response missing code", path)
	}
	if *envelope.Code != 0 {
		return fmt.Errorf("milvus vector endpoint %s failed with code %d", path, *envelope.Code)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode milvus vector response payload: %w", err)
	}
	return nil
}

type milvusCollectionRequest struct {
	DBName         string `json:"dbName,omitempty"`
	CollectionName string `json:"collectionName"`
}

type milvusHasCollectionResponse struct {
	Code int `json:"code"`
	Data struct {
		Has bool `json:"has"`
	} `json:"data"`
}

type milvusDescribeCollectionResponse struct {
	Code int `json:"code"`
	Data struct {
		Fields []milvusField `json:"fields"`
	} `json:"data"`
}

type milvusField struct {
	Name              string         `json:"name"`
	Type              string         `json:"type"`
	Params            []milvusParam  `json:"params"`
	ElementTypeParams map[string]any `json:"elementTypeParams"`
}

type milvusParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func milvusFieldDimension(field milvusField) (int, error) {
	for _, param := range field.Params {
		if strings.EqualFold(strings.TrimSpace(param.Key), "dim") {
			return parseMilvusDimension(param.Value)
		}
	}
	if value, ok := field.ElementTypeParams["dim"]; ok {
		return parseMilvusDimension(value)
	}
	return 0, fmt.Errorf("milvus vector field %q missing dim parameter", field.Name)
}

func parseMilvusDimension(value any) (int, error) {
	switch typed := value.(type) {
	case float64:
		if typed <= 0 || typed != float64(int(typed)) {
			return 0, fmt.Errorf("invalid milvus vector dimension %v", value)
		}
		return int(typed), nil
	case int:
		if typed <= 0 {
			return 0, fmt.Errorf("invalid milvus vector dimension %v", value)
		}
		return typed, nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("invalid milvus vector dimension %v", value)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid milvus vector dimension %v", value)
	}
}

func isMilvusDenseVectorType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "FLOATVECTOR", "FLOAT_VECTOR", "FLOAT16VECTOR", "FLOAT16_VECTOR", "BFLOAT16VECTOR", "BFLOAT16_VECTOR":
		return true
	default:
		return false
	}
}

func validateMilvusConfig(cfg config, context string) error {
	if strings.TrimSpace(cfg.milvusEndpoint) == "" {
		return fmt.Errorf("milvus-endpoint is required for %s", context)
	}
	if err := validateMilvusEndpoint(cfg.milvusEndpoint); err != nil {
		return err
	}
	if !isSafeSQLIdentifier(cfg.milvusDatabase) {
		return fmt.Errorf("unsafe milvus database %q", cfg.milvusDatabase)
	}
	if !isSafeSQLIdentifier(cfg.milvusCollection) {
		return fmt.Errorf("unsafe milvus collection %q", cfg.milvusCollection)
	}
	if !isSafeSQLIdentifier(cfg.milvusVectorField) {
		return fmt.Errorf("unsafe milvus vector field %q", cfg.milvusVectorField)
	}
	if cfg.milvusVectorDimension <= 0 {
		return errors.New("milvus-vector-dimension must be positive")
	}
	return nil
}

func validateMilvusEndpoint(value string) error {
	_, err := parseMilvusEndpoint(value)
	return err
}

func parseMilvusEndpoint(value string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("milvus endpoint is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid milvus endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("milvus endpoint must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("milvus endpoint must include host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("milvus endpoint must not include credentials, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed, nil
}

func safeMilvusEndpointForSummary(value string) string {
	parsed, err := parseMilvusEndpoint(value)
	if err != nil {
		return ""
	}
	return parsed.String()
}

func milvusURL(base *url.URL, path string) string {
	next := *base
	next.Path = strings.TrimRight(next.Path, "/") + "/" + strings.TrimLeft(path, "/")
	next.RawPath = ""
	return next.String()
}
