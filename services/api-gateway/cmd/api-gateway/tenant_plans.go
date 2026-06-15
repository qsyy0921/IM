package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	ratelimitinfra "github.com/qsyy0921/IM/services/api-gateway/internal/infrastructure/ratelimit"
)

type tenantRateLimitPlanSnapshot struct {
	Plans             map[string]ratelimitinfra.Plan
	Source            string
	Version           string
	GeneratedAtUnixMS int64
	ChecksumPresent   bool
}

type tenantRateLimitPlanPayload struct {
	RequestsPerSecond float64 `json:"requests_per_second"`
	RPS               float64 `json:"rps"`
	Burst             int     `json:"burst"`
}

type tenantRateLimitPlanSnapshotPayload struct {
	Version           string                                `json:"version"`
	GeneratedAtUnixMS int64                                 `json:"generated_at_unix_ms"`
	Checksum          string                                `json:"checksum"`
	Plans             map[string]tenantRateLimitPlanPayload `json:"plans"`
}

const (
	tenantPlanSnapshotMaxBytes      = 1 << 20
	tenantPlanSnapshotVersionPrefix = "quota-v1"
)

func tenantRateLimitPlansFromEnv(ctx context.Context) (tenantRateLimitPlanSnapshot, error) {
	source := strings.ToLower(strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE")))
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON"))
	path := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE"))
	endpoint := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL"))
	maxAge, err := tenantPlanMaxAgeFromEnv()
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	requireChecksum, err := tenantPlanRequireChecksumFromEnv()
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if source == "" || source == "auto" {
		switch {
		case raw != "":
			source = "inline"
		case path != "":
			source = "file"
		case endpoint != "":
			source = "url"
		default:
			source = "none"
		}
	}
	switch source {
	case "none":
		if raw != "" || path != "" || endpoint != "" {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_SOURCE=none cannot be used with tenant plan JSON, file or URL")
		}
		return tenantRateLimitPlanSnapshot{Source: source}, nil
	case "inline", "json":
		if raw == "" {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_JSON is required when tenant plan source is inline")
		}
		source = "inline"
	case "file":
		if path == "" {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE is required when tenant plan source is file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return tenantRateLimitPlanSnapshot{}, err
		}
		raw = string(data)
	case "url", "http", "https", "config-url", "config_url":
		if endpoint == "" {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL is required when tenant plan source is url")
		}
		source = "url"
		snapshot, err := tenantRateLimitPlansFromURL(ctx, endpoint, maxAge, requireChecksum)
		if err != nil {
			return tenantRateLimitPlanSnapshot{}, err
		}
		snapshot.Source = source
		return snapshot, nil
	case "db", "database", "config", "config-center", "config_center":
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan source " + source + " is not supported yet; use inline, file or url")
	default:
		return tenantRateLimitPlanSnapshot{}, errors.New("unsupported api-gateway tenant plan source")
	}
	snapshot, err := parseTenantRateLimitPlanSnapshot(raw)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	snapshot.Source = source
	if err := validateTenantPlanSnapshotPolicy(snapshot, maxAge, requireChecksum); err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	return snapshot, nil
}

func tenantRateLimitPlansFromURL(ctx context.Context, endpoint string, maxAge time.Duration, requireChecksum bool) (tenantRateLimitPlanSnapshot, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL is required when tenant plan source is url")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source is invalid")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source requires http or https")
	}
	if parsed.Host == "" {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source requires a host")
	}
	if parsed.User != nil {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source must not include user info")
	}
	requireHTTPS, err := tenantPlanURLRequireHTTPSFromEnv()
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source requires https")
	}
	bearerToken := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_BEARER_TOKEN"))
	if bearerToken != "" {
		if parsed.Scheme != "https" {
			return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL bearer token requires https")
		}
		if strings.ContainsAny(bearerToken, "\r\n") {
			return tenantRateLimitPlanSnapshot{}, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_BEARER_TOKEN must not contain line breaks")
		}
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source request is invalid")
	}
	request.Header.Set("Accept", "application/json")
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	client, err := tenantPlanURLHTTPClient(parsed)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source returned non-200 status")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, tenantPlanSnapshotMaxBytes+1))
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source response read failed")
	}
	if len(data) > tenantPlanSnapshotMaxBytes {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source response is too large")
	}
	snapshot, err := parseTenantRateLimitPlanSnapshot(string(data))
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if snapshot.Version == "" {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan URL source requires a versioned snapshot")
	}
	if err := validateTenantPlanSnapshotPolicy(snapshot, maxAge, requireChecksum); err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	snapshot.Source = "url"
	return snapshot, nil
}

func tenantPlanURLBearerTokenConfigured() bool {
	return strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_BEARER_TOKEN")) != ""
}

func tenantPlanURLRequireHTTPSFromEnv() (bool, error) {
	value, _, err := envOptionalBool("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_REQUIRE_HTTPS")
	return value, err
}

type tenantPlanURLTLSConfig struct {
	CAFile         string
	ServerName     string
	ClientCertFile string
	ClientKeyFile  string
}

func tenantPlanURLTLSConfigFromEnv() tenantPlanURLTLSConfig {
	return tenantPlanURLTLSConfig{
		CAFile:         envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CA_FILE", ""),
		ServerName:     envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_SERVER_NAME", ""),
		ClientCertFile: envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CLIENT_CERT_FILE", ""),
		ClientKeyFile:  envString("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CLIENT_KEY_FILE", ""),
	}
}

func (config tenantPlanURLTLSConfig) Enabled() bool {
	return strings.TrimSpace(config.CAFile) != "" ||
		strings.TrimSpace(config.ServerName) != "" ||
		strings.TrimSpace(config.ClientCertFile) != "" ||
		strings.TrimSpace(config.ClientKeyFile) != ""
}

func (config tenantPlanURLTLSConfig) ClientCertConfigured() bool {
	return strings.TrimSpace(config.ClientCertFile) != "" && strings.TrimSpace(config.ClientKeyFile) != ""
}

func tenantPlanURLHTTPClient(parsed *url.URL) (*http.Client, error) {
	config := tenantPlanURLTLSConfigFromEnv()
	if !config.Enabled() {
		return &http.Client{Transport: http.DefaultTransport, CheckRedirect: tenantPlanURLCheckRedirect}, nil
	}
	if parsed == nil || parsed.Scheme != "https" {
		return nil, errors.New("api-gateway tenant plan URL TLS config requires https")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: strings.TrimSpace(config.ServerName),
	}
	if caFile := strings.TrimSpace(config.CAFile); caFile != "" {
		pemBytes, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pemBytes) {
			return nil, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CA_FILE does not contain a valid PEM certificate")
		}
		tlsConfig.RootCAs = roots
	}

	clientCertFile := strings.TrimSpace(config.ClientCertFile)
	clientKeyFile := strings.TrimSpace(config.ClientKeyFile)
	if (clientCertFile == "") != (clientKeyFile == "") {
		return nil, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CLIENT_CERT_FILE and NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL_CLIENT_KEY_FILE must be configured together")
	}
	if clientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		clone := transport.Clone()
		clone.TLSClientConfig = tlsConfig
		return &http.Client{Transport: clone, CheckRedirect: tenantPlanURLCheckRedirect}, nil
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}, CheckRedirect: tenantPlanURLCheckRedirect}, nil
}

func tenantPlanURLCheckRedirect(_ *http.Request, _ []*http.Request) error {
	return errors.New("api-gateway tenant plan URL source must not redirect")
}

func parseTenantRateLimitPlans(raw string) (map[string]ratelimitinfra.Plan, error) {
	snapshot, err := parseTenantRateLimitPlanSnapshot(raw)
	if err != nil {
		return nil, err
	}
	return snapshot.Plans, nil
}

func parseTenantRateLimitPlanSnapshot(raw string) (tenantRateLimitPlanSnapshot, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tenantRateLimitPlanSnapshot{}, nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	if _, hasVersion := probe["version"]; hasVersion || probe["checksum"] != nil || probe["generated_at_unix_ms"] != nil {
		return parseVersionedTenantRateLimitPlanSnapshot(raw)
	}
	plans, err := parseTenantRateLimitPlanPayloads(raw)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	return tenantRateLimitPlanSnapshot{Plans: plans}, nil
}

func parseVersionedTenantRateLimitPlanSnapshot(raw string) (tenantRateLimitPlanSnapshot, error) {
	var payload tenantRateLimitPlanSnapshotPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	payload.Version = strings.TrimSpace(payload.Version)
	if payload.Version == "" {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan snapshot version is required")
	}
	if payload.Version != tenantPlanSnapshotVersionPrefix && !strings.HasPrefix(payload.Version, tenantPlanSnapshotVersionPrefix+".") {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan snapshot version is not supported")
	}
	if payload.GeneratedAtUnixMS <= 0 {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan snapshot generated_at_unix_ms must be greater than 0")
	}
	if payload.Plans == nil {
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan snapshot plans is required")
	}
	plans := tenantRateLimitPlansFromPayload(payload.Plans)
	checksumPresent, err := validateTenantPlanSnapshotChecksum(payload.Checksum, plans)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	return tenantRateLimitPlanSnapshot{
		Plans:             plans,
		Version:           payload.Version,
		GeneratedAtUnixMS: payload.GeneratedAtUnixMS,
		ChecksumPresent:   checksumPresent,
	}, nil
}

func parseTenantRateLimitPlanPayloads(raw string) (map[string]ratelimitinfra.Plan, error) {
	var payload map[string]tenantRateLimitPlanPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return tenantRateLimitPlansFromPayload(payload), nil
}

func tenantRateLimitPlansFromPayload(payload map[string]tenantRateLimitPlanPayload) map[string]ratelimitinfra.Plan {
	plans := make(map[string]ratelimitinfra.Plan, len(payload))
	for tenantID, item := range payload {
		rps := item.RequestsPerSecond
		if rps <= 0 {
			rps = item.RPS
		}
		plans[tenantID] = ratelimitinfra.Plan{RequestsPerSecond: rps, Burst: item.Burst}
	}
	return plans
}

func validateTenantPlanSnapshotChecksum(expected string, plans map[string]ratelimitinfra.Plan) (bool, error) {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return false, nil
	}
	if !strings.HasPrefix(expected, "sha256:") {
		return true, errors.New("api-gateway tenant plan snapshot checksum must use sha256:<hex>")
	}
	actual, err := tenantPlanSnapshotChecksum(plans)
	if err != nil {
		return true, err
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return true, errors.New("api-gateway tenant plan snapshot checksum mismatch")
	}
	return true, nil
}

func tenantPlanSnapshotChecksum(plans map[string]ratelimitinfra.Plan) (string, error) {
	type normalizedPlan struct {
		RequestsPerSecond float64 `json:"requests_per_second"`
		Burst             int     `json:"burst,omitempty"`
	}
	normalized := make(map[string]normalizedPlan, len(plans))
	for tenantID, plan := range plans {
		normalized[tenantID] = normalizedPlan{RequestsPerSecond: plan.RequestsPerSecond, Burst: plan.Burst}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func tenantPlanReloadIntervalFromEnv() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_RELOAD_INTERVAL"))
	if raw == "" || raw == "0" {
		return 0, nil
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		return 0, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_RELOAD_INTERVAL must be a positive duration")
	}
	return interval, nil
}

func tenantPlanMaxAgeFromEnv() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE"))
	if raw == "" || raw == "0" {
		return 0, nil
	}
	maxAge, err := time.ParseDuration(raw)
	if err != nil || maxAge <= 0 {
		return 0, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_MAX_AGE must be a positive duration")
	}
	return maxAge, nil
}

func tenantPlanRequireChecksumFromEnv() (bool, error) {
	value, _, err := envOptionalBool("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_REQUIRE_CHECKSUM")
	return value, err
}

func validateTenantPlanSnapshotPolicy(snapshot tenantRateLimitPlanSnapshot, maxAge time.Duration, requireChecksum bool) error {
	if err := validateTenantPlanMaxAge(snapshot, maxAge); err != nil {
		return err
	}
	if requireChecksum && !snapshot.ChecksumPresent {
		return errors.New("api-gateway tenant plan snapshot checksum is required")
	}
	return nil
}

func validateTenantPlanMaxAge(snapshot tenantRateLimitPlanSnapshot, maxAge time.Duration) error {
	if maxAge <= 0 || snapshot.GeneratedAtUnixMS <= 0 {
		return nil
	}
	generatedAt := time.UnixMilli(snapshot.GeneratedAtUnixMS)
	if time.Since(generatedAt) > maxAge {
		return errors.New("api-gateway tenant plan snapshot is stale")
	}
	return nil
}

func tenantPlanReloadLocationFromEnv(source string) string {
	switch source {
	case "file":
		return strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE"))
	case "url":
		return strings.TrimSpace(os.Getenv("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL"))
	default:
		return ""
	}
}

func startTenantPlanReloader(ctx context.Context, limiter *ratelimitinfra.Limiter, source string, location string, maxAge time.Duration, requireChecksum bool, interval time.Duration) (func() error, error) {
	source = strings.TrimSpace(source)
	location = strings.TrimSpace(location)
	if location == "" {
		switch source {
		case "file":
			return nil, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_FILE is required when tenant plan reload is enabled")
		case "url":
			return nil, errors.New("NEXUSIM_API_GATEWAY_RATE_LIMIT_TENANT_PLANS_URL is required when tenant plan reload is enabled")
		default:
			return nil, errors.New("api-gateway tenant plan reload requires file or url source")
		}
	}
	if interval <= 0 {
		return func() error { return nil }, nil
	}
	reloadCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-reloadCtx.Done():
				return
			case <-ticker.C:
				snapshot, err := tenantRateLimitPlansFromSource(reloadCtx, source, location, maxAge, requireChecksum)
				if err != nil {
					limiter.RecordTenantPlanReloadError()
					log.Printf("api-gateway tenant rate limit plan reload failed: %v", err)
					continue
				}
				if err := limiter.UpdateTenantPlanSnapshot(snapshot.Plans, snapshot.Version, snapshot.GeneratedAtUnixMS, snapshot.ChecksumPresent); err != nil {
					log.Printf("api-gateway tenant rate limit plan reload rejected: %v", err)
				}
			}
		}
	}()
	return func() error {
		cancel()
		<-done
		return nil
	}, nil
}

func tenantRateLimitPlansFromSource(ctx context.Context, source string, location string, maxAge time.Duration, requireChecksum bool) (tenantRateLimitPlanSnapshot, error) {
	switch source {
	case "file":
		snapshot, err := tenantRateLimitPlansFromFile(location)
		if err != nil {
			return tenantRateLimitPlanSnapshot{}, err
		}
		if err := validateTenantPlanSnapshotPolicy(snapshot, maxAge, requireChecksum); err != nil {
			return tenantRateLimitPlanSnapshot{}, err
		}
		return snapshot, nil
	case "url":
		return tenantRateLimitPlansFromURL(ctx, location, maxAge, requireChecksum)
	default:
		return tenantRateLimitPlanSnapshot{}, errors.New("api-gateway tenant plan reload requires file or url source")
	}
}

func tenantRateLimitPlansFromFile(path string) (tenantRateLimitPlanSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	snapshot, err := parseTenantRateLimitPlanSnapshot(string(data))
	if err != nil {
		return tenantRateLimitPlanSnapshot{}, err
	}
	snapshot.Source = "file"
	return snapshot, nil
}
