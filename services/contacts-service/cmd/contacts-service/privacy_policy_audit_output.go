package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type tenantPrivacyAuditOutput struct {
	GeneratedAt                string `json:"generated_at"`
	TenantID                   string `json:"tenant_id"`
	AllowContactRequests       bool   `json:"allow_contact_requests"`
	AllowSearchContactRequests bool   `json:"allow_search_contact_requests"`
	AllowProfileVisibility     bool   `json:"allow_profile_visibility"`
	Version                    int64  `json:"version"`
	PolicySource               string `json:"policy_source"`
	UpdatedAtUnixMS            int64  `json:"updated_at_unix_ms"`
}

type tenantPrivacySetOutput struct {
	GeneratedAt                string `json:"generated_at"`
	TenantID                   string `json:"tenant_id"`
	AllowContactRequests       bool   `json:"allow_contact_requests"`
	AllowSearchContactRequests bool   `json:"allow_search_contact_requests"`
	AllowProfileVisibility     bool   `json:"allow_profile_visibility"`
	Version                    int64  `json:"version"`
	PolicySource               string `json:"policy_source"`
	Changed                    bool   `json:"changed"`
	UpdatedAtUnixMS            int64  `json:"updated_at_unix_ms"`
}

type sourcePolicyAuditOutput struct {
	GeneratedAt          string `json:"generated_at"`
	TenantID             string `json:"tenant_id"`
	SourceType           string `json:"source_type"`
	AllowContactRequests bool   `json:"allow_contact_requests"`
	Version              int64  `json:"version"`
	UpdatedAtUnixMS      int64  `json:"updated_at_unix_ms"`
}

type sourcePolicySetOutput struct {
	GeneratedAt          string `json:"generated_at"`
	TenantID             string `json:"tenant_id"`
	SourceType           string `json:"source_type"`
	AllowContactRequests bool   `json:"allow_contact_requests"`
	Version              int64  `json:"version"`
	Changed              bool   `json:"changed"`
	UpdatedAtUnixMS      int64  `json:"updated_at_unix_ms"`
}

func writeTenantPrivacyAuditOutput(path string, result types.GetTenantContactPrivacyDefaultResult) error {
	return writeJSONFile(path, tenantPrivacyAuditOutput{
		GeneratedAt:                time.Now().UTC().Format(time.RFC3339Nano),
		TenantID:                   string(result.TenantID),
		AllowContactRequests:       result.Settings.AllowContactRequests,
		AllowSearchContactRequests: result.Settings.AllowSearchContactRequests,
		AllowProfileVisibility:     result.Settings.AllowProfileVisibility,
		Version:                    result.Settings.Version,
		PolicySource:               string(result.Settings.PolicySource),
		UpdatedAtUnixMS:            result.Settings.UpdatedAtUnixMS,
	})
}

func writeTenantPrivacySetOutput(path string, result types.SetTenantContactPrivacyDefaultResult) error {
	return writeJSONFile(path, tenantPrivacySetOutput{
		GeneratedAt:                time.Now().UTC().Format(time.RFC3339Nano),
		TenantID:                   string(result.TenantID),
		AllowContactRequests:       result.Settings.AllowContactRequests,
		AllowSearchContactRequests: result.Settings.AllowSearchContactRequests,
		AllowProfileVisibility:     result.Settings.AllowProfileVisibility,
		Version:                    result.Settings.Version,
		PolicySource:               string(result.Settings.PolicySource),
		Changed:                    result.Changed,
		UpdatedAtUnixMS:            result.Settings.UpdatedAtUnixMS,
	})
}

func writeSourcePolicyAuditOutput(path string, result types.GetTenantContactRequestSourcePolicyResult) error {
	return writeJSONFile(path, sourcePolicyAuditOutput{
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339Nano),
		TenantID:             string(result.TenantID),
		SourceType:           string(result.Policy.SourceType),
		AllowContactRequests: result.Policy.AllowContactRequests,
		Version:              result.Policy.Version,
		UpdatedAtUnixMS:      result.Policy.UpdatedAtUnixMS,
	})
}

func writeSourcePolicySetOutput(path string, result types.SetTenantContactRequestSourcePolicyResult) error {
	return writeJSONFile(path, sourcePolicySetOutput{
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339Nano),
		TenantID:             string(result.TenantID),
		SourceType:           string(result.Policy.SourceType),
		AllowContactRequests: result.Policy.AllowContactRequests,
		Version:              result.Policy.Version,
		Changed:              result.Changed,
		UpdatedAtUnixMS:      result.Policy.UpdatedAtUnixMS,
	})
}

func writeJSONFile(path string, value any) error {
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
	return encoder.Encode(value)
}
