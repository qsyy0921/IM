package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	providerReadinessPGVector         = "pgvector"
	providerReadinessOpenSearchVector = "opensearch-vector"
	providerReadinessMilvus           = "milvus"

	providerReadinessReady  = "READY"
	providerReadinessFailed = "FAILED"
)

type providerReadinessSummary struct {
	Provider   string `json:"provider"`
	Requested  bool   `json:"requested"`
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

func preflightProviderReadiness(ctx context.Context, cfg config, result *summary) error {
	providers, err := parseProviderReadiness(cfg.providerReadiness)
	if err != nil {
		return err
	}
	result.ProviderReadiness = make([]providerReadinessSummary, 0, len(providers))
	var failures []string
	for _, provider := range providers {
		entry := providerReadinessSummary{
			Provider:  provider,
			Requested: true,
			Status:    providerReadinessReady,
		}
		var err error
		switch provider {
		case providerReadinessPGVector:
			entry.Configured = strings.TrimSpace(cfg.pgVectorDSN) != ""
			err = preflightPGVector(ctx, cfg, result)
			entry.Available = result.PGVectorAvailable
		case providerReadinessOpenSearchVector:
			entry.Configured = strings.TrimSpace(cfg.openSearchVectorEndpoint) != ""
			err = preflightOpenSearchVector(ctx, cfg, result)
			entry.Available = result.OpenSearchVectorAvailable && result.OpenSearchVectorIndexExists && result.OpenSearchVectorMappingVerified
		case providerReadinessMilvus:
			entry.Configured = strings.TrimSpace(cfg.milvusEndpoint) != ""
			err = preflightMilvusVector(ctx, cfg, result)
			entry.Available = result.MilvusAvailable && result.MilvusCollectionExists && result.MilvusSchemaVerified
		default:
			err = fmt.Errorf("unsupported provider %q", provider)
		}
		if err != nil {
			entry.Status = providerReadinessFailed
			entry.Error = err.Error()
			failures = append(failures, provider+": "+err.Error())
		}
		result.ProviderReadiness = append(result.ProviderReadiness, entry)
	}
	if len(failures) > 0 {
		return fmt.Errorf("vector provider readiness failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func parseProviderReadiness(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("provider-readiness is required")
	}
	seen := map[string]bool{}
	providers := []string{}
	for _, part := range strings.Split(value, ",") {
		provider := strings.ToLower(strings.TrimSpace(part))
		if provider == "" {
			continue
		}
		switch provider {
		case providerReadinessPGVector, providerReadinessOpenSearchVector, providerReadinessMilvus:
		default:
			return nil, fmt.Errorf("unsupported provider-readiness provider %q", provider)
		}
		if seen[provider] {
			continue
		}
		seen[provider] = true
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return nil, errors.New("provider-readiness is required")
	}
	return providers, nil
}
