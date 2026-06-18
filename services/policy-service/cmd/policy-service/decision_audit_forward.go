package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

const policyDecisionAuditForwardSchema = "nexusim.policy.decision_audit_forward.v1"

type decisionAuditForwardConfig struct {
	Endpoint      string
	BearerToken   string
	Timeout       time.Duration
	AllowInsecure bool
	DryRun        bool
	OutputPath    string
	Limit         int
	Client        *http.Client
}

type decisionAuditForwardPayload struct {
	Schema      string                         `json:"schema"`
	GeneratedAt string                         `json:"generated_at"`
	Filters     map[string]string              `json:"filters,omitempty"`
	RowCount    int                            `json:"row_count"`
	Rows        []decisionAuditExportOutputRow `json:"rows"`
}

type decisionAuditForwardSummary struct {
	GeneratedAt    string            `json:"generated_at"`
	Schema         string            `json:"schema"`
	DryRun         bool              `json:"dry_run"`
	EndpointScheme string            `json:"endpoint_scheme"`
	EndpointHost   string            `json:"endpoint_host"`
	RowCount       int               `json:"row_count"`
	StatusCode     int               `json:"status_code,omitempty"`
	StatusFamily   string            `json:"status_family,omitempty"`
	Success        bool              `json:"success"`
	ErrorClass     string            `json:"error_class,omitempty"`
	Filters        map[string]string `json:"filters,omitempty"`
}

func runDecisionAuditForward() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config, err := policyDecisionAuditForwardConfigFromEnv()
	if err != nil {
		return err
	}
	dsn := envString("NEXUSIM_PG_DSN", "")
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required for policy decision audit forward")
	}
	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	options, filters, err := policyDecisionAuditForwardOptionsFromEnv(config.Limit)
	if err != nil {
		return err
	}
	rows, err := postgresinfra.NewOutboxStore(pool).ExportDecisionAudit(ctx, options)
	if err != nil {
		return err
	}
	summary, forwardErr := forwardDecisionAudit(ctx, config, rows, filters)
	if config.OutputPath != "" {
		if err := writeDecisionAuditForwardSummary(config.OutputPath, summary); err != nil {
			return err
		}
	}
	return forwardErr
}

func policyDecisionAuditForwardConfigFromEnv() (decisionAuditForwardConfig, error) {
	allowInsecure, allowConfigured, err := envOptionalBool("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ALLOW_INSECURE")
	if err != nil {
		return decisionAuditForwardConfig{}, err
	}
	dryRun, _, err := envOptionalBool("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_DRY_RUN")
	if err != nil {
		return decisionAuditForwardConfig{}, err
	}
	timeout, err := envPositiveDuration("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_TIMEOUT", 5*time.Second)
	if err != nil {
		return decisionAuditForwardConfig{}, err
	}
	endpoint := strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT"))
	if endpoint == "" && !dryRun {
		return decisionAuditForwardConfig{}, errors.New("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT is required")
	}
	if endpoint != "" {
		parsed, err := validateDecisionAuditForwardEndpoint(endpoint, allowConfigured && allowInsecure)
		if err != nil {
			return decisionAuditForwardConfig{}, err
		}
		endpoint = parsed.String()
	}
	client := &http.Client{Timeout: timeout}
	return decisionAuditForwardConfig{
		Endpoint:      endpoint,
		BearerToken:   strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_BEARER_TOKEN")),
		Timeout:       timeout,
		AllowInsecure: allowConfigured && allowInsecure,
		DryRun:        dryRun,
		OutputPath:    strings.TrimSpace(os.Getenv("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_OUTPUT")),
		Limit:         envInt("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_LIMIT", 100),
		Client:        client,
	}, nil
}

func validateDecisionAuditForwardEndpoint(endpoint string, allowInsecure bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT must be an absolute URL")
	}
	switch parsed.Scheme {
	case "https":
		return parsed, nil
	case "http":
		if allowInsecure {
			return parsed, nil
		}
		return nil, errors.New("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT must use https unless NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ALLOW_INSECURE=true")
	default:
		return nil, errors.New("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ENDPOINT must use http or https")
	}
}

func policyDecisionAuditForwardOptionsFromEnv(limit int) (postgresinfra.DecisionAuditExportOptions, map[string]string, error) {
	allowed, allowedConfigured, err := envOptionalBool("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ALLOWED")
	if err != nil {
		return postgresinfra.DecisionAuditExportOptions{}, nil, err
	}
	var allowedFilter *bool
	if allowedConfigured {
		allowedFilter = &allowed
	}
	createdAfter, err := envOptionalRFC3339Time("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_CREATED_AFTER")
	if err != nil {
		return postgresinfra.DecisionAuditExportOptions{}, nil, err
	}
	createdBefore, err := envOptionalRFC3339Time("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_CREATED_BEFORE")
	if err != nil {
		return postgresinfra.DecisionAuditExportOptions{}, nil, err
	}
	filters := map[string]string{
		"event_id":        envString("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_EVENT_ID", ""),
		"tenant_id":       envString("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_TENANT_ID", ""),
		"action":          envString("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ACTION", ""),
		"allowed":         envString("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_ALLOWED", ""),
		"classification":  envString("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_CLASSIFICATION", ""),
		"reason_code":     envString("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_REASON_CODE", ""),
		"decision_source": envString("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_DECISION_SOURCE", ""),
		"status":          envString("NEXUSIM_POLICY_DECISION_AUDIT_FORWARD_STATUS", ""),
		"created_after":   formatOptionalFilterTime(createdAfter),
		"created_before":  formatOptionalFilterTime(createdBefore),
	}
	return postgresinfra.DecisionAuditExportOptions{
		EventID:        filters["event_id"],
		TenantID:       filters["tenant_id"],
		Action:         filters["action"],
		Allowed:        allowedFilter,
		Classification: filters["classification"],
		ReasonCode:     filters["reason_code"],
		DecisionSource: filters["decision_source"],
		Status:         filters["status"],
		CreatedAfter:   createdAfter,
		CreatedBefore:  createdBefore,
		Limit:          limit,
	}, filters, nil
}

func forwardDecisionAudit(ctx context.Context, config decisionAuditForwardConfig, rows []postgresinfra.DecisionAuditExportRow, filters map[string]string) (decisionAuditForwardSummary, error) {
	parsed, _ := url.Parse(config.Endpoint)
	summary := decisionAuditForwardSummary{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Schema:         policyDecisionAuditForwardSchema,
		DryRun:         config.DryRun,
		EndpointScheme: endpointScheme(parsed),
		EndpointHost:   endpointHost(parsed),
		RowCount:       len(rows),
		Filters:        compactCleanupFilters(filters),
	}
	if config.DryRun {
		summary.StatusFamily = "DRY_RUN"
		summary.Success = true
		return summary, nil
	}
	payload := decisionAuditForwardPayload{
		Schema:      policyDecisionAuditForwardSchema,
		GeneratedAt: summary.GeneratedAt,
		Filters:     summary.Filters,
		RowCount:    len(rows),
		Rows:        decisionAuditRowsForForward(rows),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		summary.ErrorClass = "ENCODE_FAILED"
		return summary, errors.New("policy decision audit forward failed")
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.Endpoint, bytes.NewReader(body))
	if err != nil {
		summary.ErrorClass = "REQUEST_FAILED"
		return summary, errors.New("policy decision audit forward failed")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if config.BearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+config.BearerToken)
	}
	response, err := client.Do(request)
	if err != nil {
		summary.ErrorClass = "SINK_UNAVAILABLE"
		return summary, errors.New("policy decision audit forward sink unavailable")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	summary.StatusCode = response.StatusCode
	summary.StatusFamily = statusFamily(response.StatusCode)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		summary.ErrorClass = "SINK_REJECTED"
		return summary, errors.New("policy decision audit forward sink rejected")
	}
	summary.Success = true
	return summary, nil
}

func decisionAuditRowsForForward(rows []postgresinfra.DecisionAuditExportRow) []decisionAuditExportOutputRow {
	output := decisionAuditExportOutput{
		Rows: make([]decisionAuditExportOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		outputRow := decisionAuditExportOutputRow{
			EventID:                  row.EventID,
			TenantID:                 row.TenantID,
			ActorUserKey:             row.ActorUserKey,
			DeviceKey:                row.DeviceKey,
			ConversationKey:          row.ConversationKey,
			MessageKey:               row.MessageKey,
			Action:                   row.Action,
			MessageIDPresent:         row.MessageIDPresent,
			DirectPeerContextPresent: row.DirectPeerContextPresent,
			DirectPeerKey:            row.DirectPeerKey,
			Allowed:                  row.Allowed,
			PermissionVersion:        row.PermissionVersion,
			Classification:           row.Classification,
			ReasonCode:               row.ReasonCode,
			DecisionSource:           row.DecisionSource,
			Status:                   row.Status,
			EventType:                row.EventType,
			EventVersion:             row.EventVersion,
			Producer:                 row.Producer,
			PartitionKey:             row.PartitionKey,
			CorrelationID:            row.CorrelationID,
			TraceID:                  row.TraceID,
			CreatedAt:                formatOutboxAuditTime(row.CreatedAt),
		}
		if row.PublishedAt != nil {
			outputRow.PublishedAt = formatOutboxAuditTime(*row.PublishedAt)
		}
		output.Rows = append(output.Rows, outputRow)
	}
	return output.Rows
}

func writeDecisionAuditForwardSummary(path string, summary decisionAuditForwardSummary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

func endpointScheme(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	return parsed.Scheme
}

func endpointHost(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	return parsed.Host
}

func statusFamily(statusCode int) string {
	if statusCode <= 0 {
		return ""
	}
	return string(rune('0'+statusCode/100)) + "xx"
}
