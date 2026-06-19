package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

const DefaultExternalHTTPMaxResponseBytes int64 = 16 * 1024

type ExternalHTTPExecutorOptions struct {
	Endpoint         string
	BearerToken      string
	AllowedTools     []string
	Timeout          time.Duration
	MaxResponseBytes int64
}

type ExternalHTTPExecutor struct {
	endpoint         string
	bearerToken      string
	allowedTools     map[string]struct{}
	client           *http.Client
	maxResponseBytes int64
}

func NewExternalHTTPExecutor(options ExternalHTTPExecutorOptions) (ExternalHTTPExecutor, error) {
	endpoint, err := validateExternalHTTPEndpoint(options.Endpoint)
	if err != nil {
		return ExternalHTTPExecutor{}, err
	}
	allowedTools := normalizeAllowedTools(options.AllowedTools)
	if len(allowedTools) == 0 {
		return ExternalHTTPExecutor{}, errors.New("external HTTP executor requires at least one allowed tool")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = DefaultExternalHTTPMaxResponseBytes
	}
	return ExternalHTTPExecutor{
		endpoint:         endpoint,
		bearerToken:      strings.TrimSpace(options.BearerToken),
		allowedTools:     allowedTools,
		client:           &http.Client{Timeout: timeout},
		maxResponseBytes: maxResponseBytes,
	}, nil
}

func (executor ExternalHTTPExecutor) ExecuteTool(
	ctx context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	if executor.client == nil || executor.endpoint == "" {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if strings.TrimSpace(command.ToolName) == "" || strings.TrimSpace(command.ToolName) == types.LocalSafeEchoToolName {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if _, ok := executor.allowedTools[strings.TrimSpace(command.ToolName)]; !ok {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if strings.ToUpper(strings.TrimSpace(command.RiskLevel)) != "LOW" {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	payload, err := json.Marshal(externalToolRequest{
		SchemaVersion: 1,
		ToolName:      command.ToolName,
		Action:        types.ToolActionExecute,
		ResourceType:  command.ResourceType,
		ResourceID:    command.ResourceID,
		RiskLevel:     command.RiskLevel,
		IntentPresent: strings.TrimSpace(command.Intent) != "",
		InputSHA256:   command.InputSHA256,
	})
	if err != nil {
		return types.ToolExecutionResult{}, types.ErrToolExecutionFailed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, executor.endpoint, bytes.NewReader(payload))
	if err != nil {
		return types.ToolExecutionResult{}, types.ErrToolProviderUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	if executor.bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+executor.bearerToken)
	}
	response, err := executor.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return types.ToolExecutionResult{}, types.ErrToolExecutionTimeout
		}
		return types.ToolExecutionResult{}, types.ErrToolProviderUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return types.ToolExecutionResult{}, types.ErrToolProviderPermissionDenied
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return types.ToolExecutionResult{}, types.ErrToolProviderRateLimited
	}
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusGatewayTimeout {
		return types.ToolExecutionResult{}, types.ErrToolExecutionTimeout
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return types.ToolExecutionResult{}, types.ErrToolProviderUnavailable
	}
	body, err := readLimited(response.Body, executor.maxResponseBytes)
	if err != nil {
		return types.ToolExecutionResult{}, err
	}
	return types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: string(body),
	}, nil
}

type externalToolRequest struct {
	SchemaVersion int    `json:"schema_version"`
	ToolName      string `json:"tool_name"`
	Action        string `json:"action"`
	ResourceType  string `json:"resource_type"`
	ResourceID    string `json:"resource_id"`
	RiskLevel     string `json:"risk_level"`
	IntentPresent bool   `json:"intent_present"`
	InputSHA256   string `json:"input_sha256,omitempty"`
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultExternalHTTPMaxResponseBytes
	}
	limited := io.LimitReader(reader, maxBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, types.ErrToolProviderUnavailable
	}
	if int64(len(payload)) > maxBytes {
		return nil, types.ErrToolOutputUnsafe
	}
	return payload, nil
}

func normalizeAllowedTools(tools []string) map[string]struct{} {
	result := make(map[string]struct{}, len(tools))
	for _, raw := range tools {
		toolName := strings.TrimSpace(raw)
		if toolName == "" || toolName == types.LocalSafeEchoToolName {
			continue
		}
		result[toolName] = struct{}{}
	}
	return result
}

func validateExternalHTTPEndpoint(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("external HTTP executor endpoint is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("external HTTP executor endpoint must use http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("external HTTP executor endpoint host is required")
	}
	if parsed.Scheme == "http" && !isLocalOrPrivateHost(parsed.Hostname()) {
		return "", errors.New("plain HTTP external executor endpoint must be loopback or private")
	}
	return parsed.String(), nil
}

func isLocalOrPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func sortedAllowedTools(tools map[string]struct{}) []string {
	result := make([]string, 0, len(tools))
	for toolName := range tools {
		result = append(result, toolName)
	}
	sort.Strings(result)
	return result
}

func (executor ExternalHTTPExecutor) String() string {
	return fmt.Sprintf("external-http tools=%v", sortedAllowedTools(executor.allowedTools))
}
