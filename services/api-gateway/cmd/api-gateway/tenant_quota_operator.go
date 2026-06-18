package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

type apiGatewayTenantQuotaAuditOptions struct {
	TenantID string
	Enabled  *bool
	Limit    int
}

type apiGatewayTenantQuotaSetOptions struct {
	TenantID          string
	RequestsPerSecond float64
	Burst             int
	Enabled           bool
	Source            string
	DryRun            bool
}

type apiGatewayTenantQuotaRow struct {
	TenantID          string
	RequestsPerSecond float64
	Burst             int
	Enabled           bool
	Source            string
	UpdatedAt         time.Time
}

type apiGatewayTenantQuotaAuditOutput struct {
	GeneratedAt string                      `json:"generated_at"`
	Rows        []apiGatewayTenantQuotaJSON `json:"rows"`
}

type apiGatewayTenantQuotaSetOutput struct {
	GeneratedAt string                         `json:"generated_at"`
	DryRun      bool                           `json:"dry_run"`
	Row         apiGatewayTenantQuotaJSON      `json:"row"`
	Approval    *apiGatewayTenantQuotaApproval `json:"approval,omitempty"`
}

type apiGatewayTenantQuotaJSON struct {
	TenantID          string  `json:"tenant_id"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
	Enabled           bool    `json:"enabled"`
	Source            string  `json:"source"`
	UpdatedAt         string  `json:"updated_at"`
}

type apiGatewayTenantQuotaApproval struct {
	SchemaVersion     string                                   `json:"schema_version"`
	Service           string                                   `json:"service"`
	ApprovalType      string                                   `json:"approval_type"`
	Status            string                                   `json:"status"`
	ChangeID          string                                   `json:"change_id"`
	TargetEnvironment string                                   `json:"target_environment"`
	Operator          string                                   `json:"operator"`
	Approver          string                                   `json:"approver"`
	GeneratedAtUnixMS int64                                    `json:"generated_at_unix_ms"`
	ApprovedAtUnixMS  int64                                    `json:"approved_at_unix_ms"`
	ExpiresAtUnixMS   int64                                    `json:"expires_at_unix_ms"`
	DesiredPlan       apiGatewayTenantQuotaApprovalDesiredPlan `json:"desired_plan"`
}

type apiGatewayTenantQuotaApprovalDesiredPlan struct {
	TenantID          string  `json:"tenant_id"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
	Enabled           bool    `json:"enabled"`
	Source            string  `json:"source"`
}

func runTenantQuotaAudit() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := apiGatewayTenantQuotaDSNFromEnv()
	if dsn == "" {
		return errors.New("NEXUSIM_API_GATEWAY_TENANT_QUOTA_DB_DSN, NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_DB_DSN or NEXUSIM_PG_DSN is required for api-gateway tenant quota audit")
	}
	pool, err := openAPIGatewayPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	enabled, enabledConfigured, err := envOptionalBool("NEXUSIM_API_GATEWAY_TENANT_QUOTA_AUDIT_ENABLED")
	if err != nil {
		return err
	}
	var enabledFilter *bool
	if enabledConfigured {
		enabledFilter = &enabled
	}
	rows, err := auditAPIGatewayTenantQuotas(ctx, pool, apiGatewayTenantQuotaAuditOptions{
		TenantID: envString("NEXUSIM_API_GATEWAY_TENANT_QUOTA_AUDIT_TENANT_ID", ""),
		Enabled:  enabledFilter,
		Limit:    envInt("NEXUSIM_API_GATEWAY_TENANT_QUOTA_AUDIT_LIMIT", 20),
	})
	if err != nil {
		return err
	}
	log.Printf("api-gateway tenant quota audit completed rows=%d", len(rows))
	for _, row := range rows {
		log.Printf(
			"api_gateway_tenant_quota tenant_id=%s requests_per_second=%.3f burst=%d enabled=%t source=%s updated_at=%s",
			row.TenantID,
			row.RequestsPerSecond,
			row.Burst,
			row.Enabled,
			row.Source,
			row.UpdatedAt.UTC().Format(time.RFC3339),
		)
	}
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_TENANT_QUOTA_AUDIT_OUTPUT")); outputPath != "" {
		if err := writeAPIGatewayTenantQuotaAuditOutput(outputPath, rows); err != nil {
			return err
		}
	}
	return nil
}

func runTenantQuotaSet() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rps, err := envPositiveFloat64("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_REQUESTS_PER_SECOND", 0)
	if err != nil {
		return err
	}
	burst, err := envPositiveInt("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_BURST", 0)
	if err != nil {
		return err
	}
	enabled, enabledConfigured, err := envOptionalBool("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_ENABLED")
	if err != nil {
		return err
	}
	if !enabledConfigured {
		enabled = true
	}
	dryRun, _, err := envOptionalBool("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_DRY_RUN")
	if err != nil {
		return err
	}
	options := apiGatewayTenantQuotaSetOptions{
		TenantID:          envString("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_TENANT_ID", ""),
		RequestsPerSecond: rps,
		Burst:             burst,
		Enabled:           enabled,
		Source:            envString("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_SOURCE", "operator"),
		DryRun:            dryRun,
	}
	if err := validateAPIGatewayTenantQuotaSetOptions(options); err != nil {
		return err
	}
	requireApproval, _, err := envOptionalBool("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_REQUIRE_APPROVAL")
	if err != nil {
		return err
	}
	approvalPath := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_APPROVAL_PATH"))
	var approval *apiGatewayTenantQuotaApproval
	if !dryRun && (requireApproval || approvalPath != "") {
		if approvalPath == "" {
			return errors.New("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_APPROVAL_PATH is required when tenant quota approval is required")
		}
		approval, err = readAndValidateAPIGatewayTenantQuotaApproval(approvalPath, options, time.Now)
		if err != nil {
			return err
		}
	}
	var row apiGatewayTenantQuotaRow
	if dryRun {
		row = apiGatewayTenantQuotaRow{
			TenantID:          strings.TrimSpace(options.TenantID),
			RequestsPerSecond: options.RequestsPerSecond,
			Burst:             options.Burst,
			Enabled:           options.Enabled,
			Source:            strings.TrimSpace(options.Source),
			UpdatedAt:         time.Now().UTC(),
		}
	} else {
		dsn := apiGatewayTenantQuotaDSNFromEnv()
		if dsn == "" {
			return errors.New("NEXUSIM_API_GATEWAY_TENANT_QUOTA_DB_DSN, NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_DB_DSN or NEXUSIM_PG_DSN is required for api-gateway tenant quota set")
		}
		pool, err := openAPIGatewayPGPool(ctx, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()

		row, err = setAPIGatewayTenantQuota(ctx, pool, options)
		if err != nil {
			return err
		}
	}
	log.Printf(
		"api-gateway tenant quota set tenant_id=%s requests_per_second=%.3f burst=%d enabled=%t source=%s dry_run=%t",
		row.TenantID,
		row.RequestsPerSecond,
		row.Burst,
		row.Enabled,
		row.Source,
		dryRun,
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_OUTPUT")); outputPath != "" {
		if err := writeAPIGatewayTenantQuotaSetOutput(outputPath, row, dryRun, approval); err != nil {
			return err
		}
	}
	return nil
}

func apiGatewayTenantQuotaDSNFromEnv() string {
	if dsn := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_TENANT_QUOTA_DB_DSN")); dsn != "" {
		return dsn
	}
	return tenantPlanDBDSNFromEnv()
}

func openAPIGatewayPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_API_GATEWAY_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
}

func auditAPIGatewayTenantQuotas(ctx context.Context, pool *pgxpool.Pool, options apiGatewayTenantQuotaAuditOptions) ([]apiGatewayTenantQuotaRow, error) {
	if pool == nil {
		return nil, errors.New("api-gateway tenant quota DB is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var args []any
	var clauses []string
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		if err := validateAPIGatewayTenantID(tenantID); err != nil {
			return nil, err
		}
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if options.Enabled != nil {
		args = append(args, *options.Enabled)
		clauses = append(clauses, "enabled = $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := pool.Query(ctx, `
SELECT tenant_id, requests_per_second, burst, enabled, source, updated_at
FROM api_gateway_rate_limit_tenant_plans
`+where+`
ORDER BY updated_at DESC, tenant_id
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, errors.New("api-gateway tenant quota audit query failed")
	}
	defer rows.Close()

	result := make([]apiGatewayTenantQuotaRow, 0, limit)
	for rows.Next() {
		var row apiGatewayTenantQuotaRow
		if err := rows.Scan(
			&row.TenantID,
			&row.RequestsPerSecond,
			&row.Burst,
			&row.Enabled,
			&row.Source,
			&row.UpdatedAt,
		); err != nil {
			return nil, errors.New("api-gateway tenant quota audit scan failed")
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("api-gateway tenant quota audit query failed")
	}
	return result, nil
}

func setAPIGatewayTenantQuota(ctx context.Context, pool *pgxpool.Pool, options apiGatewayTenantQuotaSetOptions) (apiGatewayTenantQuotaRow, error) {
	if pool == nil {
		return apiGatewayTenantQuotaRow{}, errors.New("api-gateway tenant quota DB is not configured")
	}
	if err := validateAPIGatewayTenantQuotaSetOptions(options); err != nil {
		return apiGatewayTenantQuotaRow{}, err
	}
	var row apiGatewayTenantQuotaRow
	err := pool.QueryRow(ctx, `
INSERT INTO api_gateway_rate_limit_tenant_plans (
    tenant_id,
    requests_per_second,
    burst,
    enabled,
    source,
    updated_at
) VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (tenant_id) DO UPDATE
SET requests_per_second = EXCLUDED.requests_per_second,
    burst = EXCLUDED.burst,
    enabled = EXCLUDED.enabled,
    source = EXCLUDED.source,
    updated_at = now()
RETURNING tenant_id, requests_per_second, burst, enabled, source, updated_at
`,
		strings.TrimSpace(options.TenantID),
		options.RequestsPerSecond,
		options.Burst,
		options.Enabled,
		strings.TrimSpace(options.Source),
	).Scan(
		&row.TenantID,
		&row.RequestsPerSecond,
		&row.Burst,
		&row.Enabled,
		&row.Source,
		&row.UpdatedAt,
	)
	if err != nil {
		return apiGatewayTenantQuotaRow{}, errors.New("api-gateway tenant quota set failed")
	}
	return row, nil
}

func validateAPIGatewayTenantQuotaSetOptions(options apiGatewayTenantQuotaSetOptions) error {
	if err := validateAPIGatewayTenantID(options.TenantID); err != nil {
		return err
	}
	if options.RequestsPerSecond <= 0 {
		return errors.New("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_REQUESTS_PER_SECOND must be positive")
	}
	if options.Burst <= 0 {
		return errors.New("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_BURST must be a positive integer")
	}
	if strings.TrimSpace(options.Source) == "" {
		return errors.New("NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_SOURCE is required")
	}
	return nil
}

func readAndValidateAPIGatewayTenantQuotaApproval(path string, options apiGatewayTenantQuotaSetOptions, now func() time.Time) (*apiGatewayTenantQuotaApproval, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var approval apiGatewayTenantQuotaApproval
	if err := json.Unmarshal(raw, &approval); err != nil {
		return nil, errors.New("api-gateway tenant quota approval JSON is invalid")
	}
	if err := validateAPIGatewayTenantQuotaApproval(approval, options, now); err != nil {
		return nil, err
	}
	return &approval, nil
}

func validateAPIGatewayTenantQuotaApproval(approval apiGatewayTenantQuotaApproval, options apiGatewayTenantQuotaSetOptions, now func() time.Time) error {
	if approval.SchemaVersion != "nexusim.api_gateway.tenant_quota_approval.v1" {
		return errors.New("api-gateway tenant quota approval has unsupported schema_version")
	}
	if approval.Service != "api-gateway" {
		return errors.New("api-gateway tenant quota approval service must be api-gateway")
	}
	if approval.ApprovalType != "tenant_quota_change" {
		return errors.New("api-gateway tenant quota approval has unsupported approval_type")
	}
	if approval.Status != "APPROVED" {
		return errors.New("api-gateway tenant quota approval status must be APPROVED")
	}
	if err := validateLowSensitiveAPIGatewayApprovalLabel(approval.ChangeID, "change_id"); err != nil {
		return err
	}
	if err := validateLowSensitiveAPIGatewayApprovalLabel(approval.TargetEnvironment, "target_environment"); err != nil {
		return err
	}
	if err := validateLowSensitiveAPIGatewayApprovalLabel(approval.Operator, "operator"); err != nil {
		return err
	}
	if err := validateLowSensitiveAPIGatewayApprovalLabel(approval.Approver, "approver"); err != nil {
		return err
	}
	if approval.GeneratedAtUnixMS <= 0 || approval.ApprovedAtUnixMS <= 0 || approval.ExpiresAtUnixMS <= 0 {
		return errors.New("api-gateway tenant quota approval timestamps must be positive")
	}
	if now == nil {
		now = time.Now
	}
	nowMS := now().UnixMilli()
	if approval.ApprovedAtUnixMS > nowMS+60_000 {
		return errors.New("api-gateway tenant quota approval approved_at_unix_ms is in the future")
	}
	if approval.ExpiresAtUnixMS <= nowMS {
		return errors.New("api-gateway tenant quota approval is expired")
	}
	if err := validateAPIGatewayTenantID(approval.DesiredPlan.TenantID); err != nil {
		return err
	}
	if strings.TrimSpace(approval.DesiredPlan.TenantID) != strings.TrimSpace(options.TenantID) {
		return errors.New("api-gateway tenant quota approval tenant_id does not match requested change")
	}
	if math.Abs(approval.DesiredPlan.RequestsPerSecond-options.RequestsPerSecond) > 1e-9 {
		return errors.New("api-gateway tenant quota approval requests_per_second does not match requested change")
	}
	if approval.DesiredPlan.Burst != options.Burst {
		return errors.New("api-gateway tenant quota approval burst does not match requested change")
	}
	if approval.DesiredPlan.Enabled != options.Enabled {
		return errors.New("api-gateway tenant quota approval enabled does not match requested change")
	}
	if strings.TrimSpace(approval.DesiredPlan.Source) != strings.TrimSpace(options.Source) {
		return errors.New("api-gateway tenant quota approval source does not match requested change")
	}
	return nil
}

func validateLowSensitiveAPIGatewayApprovalLabel(value string, fieldName string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("api-gateway tenant quota approval " + fieldName + " is required")
	}
	if len(value) > 128 {
		return errors.New("api-gateway tenant quota approval " + fieldName + " is too long")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return errors.New("api-gateway tenant quota approval " + fieldName + " must not contain whitespace or control characters")
		}
	}
	lower := strings.ToLower(value)
	sensitiveMarkers := []string{"password", "passwd", "secret", "token", "bearer", "credential", "api_key", "apikey", "access_key", "refresh", "session", "cookie", "sk-", "eyj"}
	for _, marker := range sensitiveMarkers {
		if strings.Contains(lower, marker) {
			return errors.New("api-gateway tenant quota approval " + fieldName + " must be low-sensitive")
		}
	}
	if strings.Contains(value, "@") {
		return errors.New("api-gateway tenant quota approval " + fieldName + " must not contain email-like values")
	}
	return nil
}

func validateAPIGatewayTenantID(tenantID string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}
	if len(tenantID) > 128 {
		return errors.New("tenant_id is too long")
	}
	for _, r := range tenantID {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return errors.New("tenant_id must not contain whitespace or control characters")
		}
	}
	return nil
}

func envPositiveFloat64(name string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		if fallback > 0 {
			return fallback, nil
		}
		return 0, errors.New(name + " is required")
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return 0, errors.New(name + " must be positive")
	}
	return value, nil
}

func envPositiveInt(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		if fallback > 0 {
			return fallback, nil
		}
		return 0, errors.New(name + " is required")
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, errors.New(name + " must be a positive integer")
	}
	return value, nil
}

func writeAPIGatewayTenantQuotaAuditOutput(path string, rows []apiGatewayTenantQuotaRow) error {
	output := apiGatewayTenantQuotaAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]apiGatewayTenantQuotaJSON, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, apiGatewayTenantQuotaJSONFromRow(row))
	}
	return writeAPIGatewayTenantQuotaJSON(path, output)
}

func writeAPIGatewayTenantQuotaSetOutput(path string, row apiGatewayTenantQuotaRow, dryRun bool, approval *apiGatewayTenantQuotaApproval) error {
	return writeAPIGatewayTenantQuotaJSON(path, apiGatewayTenantQuotaSetOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		DryRun:      dryRun,
		Row:         apiGatewayTenantQuotaJSONFromRow(row),
		Approval:    approval,
	})
}

func apiGatewayTenantQuotaJSONFromRow(row apiGatewayTenantQuotaRow) apiGatewayTenantQuotaJSON {
	updatedAt := ""
	if !row.UpdatedAt.IsZero() {
		updatedAt = row.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return apiGatewayTenantQuotaJSON{
		TenantID:          row.TenantID,
		RequestsPerSecond: row.RequestsPerSecond,
		Burst:             row.Burst,
		Enabled:           row.Enabled,
		Source:            row.Source,
		UpdatedAt:         updatedAt,
	}
}

func writeAPIGatewayTenantQuotaJSON(path string, output any) error {
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
	return encoder.Encode(output)
}
