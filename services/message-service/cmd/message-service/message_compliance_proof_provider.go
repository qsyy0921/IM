package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

const (
	messageComplianceProofProviderModeManual   = "manual"
	messageComplianceProofProviderModeManifest = "manifest"
)

type messageComplianceProofManifest struct {
	Proofs []messageComplianceProofManifestEntry `json:"proofs"`
}

type messageComplianceProofManifestEntry struct {
	ExternalProofRef string `json:"external_proof_ref"`
	Provider         string `json:"provider"`
	ProofHash        string `json:"proof_hash"`
	Status           string `json:"status"`
}

func messageComplianceProofRegisterOptionsFromEnv() (postgresinfra.MessageComplianceExternalProofMutationOptions, error) {
	options := postgresinfra.MessageComplianceExternalProofMutationOptions{
		TenantID:         envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_TENANT_ID", ""),
		ExternalProofRef: envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_EXTERNAL_PROOF_REF", ""),
		Provider:         envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_PROVIDER", ""),
		ProofHash:        envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_HASH", ""),
		OperatorID:       envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_OPERATOR_ID", ""),
	}
	return resolveMessageComplianceProofRegisterOptions(
		options,
		envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_PROVIDER_MODE", messageComplianceProofProviderModeManual),
		envString("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_MANIFEST_PATH", ""),
	)
}

func resolveMessageComplianceProofRegisterOptions(
	options postgresinfra.MessageComplianceExternalProofMutationOptions,
	providerMode string,
	manifestPath string,
) (postgresinfra.MessageComplianceExternalProofMutationOptions, error) {
	options.TenantID = strings.TrimSpace(options.TenantID)
	options.ExternalProofRef = strings.TrimSpace(options.ExternalProofRef)
	options.Provider = strings.TrimSpace(options.Provider)
	options.ProofHash = strings.TrimSpace(options.ProofHash)
	options.OperatorID = strings.TrimSpace(options.OperatorID)
	mode := strings.ToLower(strings.TrimSpace(providerMode))
	if mode == "" {
		mode = messageComplianceProofProviderModeManual
	}
	switch mode {
	case messageComplianceProofProviderModeManual:
		return options, nil
	case messageComplianceProofProviderModeManifest:
		return resolveMessageComplianceProofRegisterOptionsFromManifest(options, manifestPath)
	default:
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("unsupported NEXUSIM_MESSAGE_COMPLIANCE_PROOF_PROVIDER_MODE")
	}
}

func resolveMessageComplianceProofRegisterOptionsFromManifest(
	options postgresinfra.MessageComplianceExternalProofMutationOptions,
	manifestPath string,
) (postgresinfra.MessageComplianceExternalProofMutationOptions, error) {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("NEXUSIM_MESSAGE_COMPLIANCE_PROOF_MANIFEST_PATH is required when provider mode is manifest")
	}
	if strings.TrimSpace(options.ExternalProofRef) == "" {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("external_proof_ref is required")
	}
	entries, err := loadMessageComplianceProofManifest(manifestPath)
	if err != nil {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, err
	}
	var matched *messageComplianceProofManifestEntry
	for index := range entries {
		entry := normalizeMessageComplianceProofManifestEntry(entries[index])
		if entry.ExternalProofRef == options.ExternalProofRef {
			matched = &entry
			break
		}
	}
	if matched == nil {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("external proof ref not found in manifest")
	}
	if matched.Status != postgresinfra.MessageComplianceExternalProofStatusVerified {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("external proof ref is not verified in manifest")
	}
	if matched.Provider == "" {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("manifest proof provider is required")
	}
	if matched.ProofHash == "" {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("manifest proof_hash is required")
	}
	if options.Provider != "" && options.Provider != matched.Provider {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("manifest proof provider does not match request")
	}
	if options.ProofHash != "" && options.ProofHash != matched.ProofHash {
		return postgresinfra.MessageComplianceExternalProofMutationOptions{}, errors.New("manifest proof_hash does not match request")
	}
	options.Provider = matched.Provider
	options.ProofHash = matched.ProofHash
	return options, nil
}

func loadMessageComplianceProofManifest(path string) ([]messageComplianceProofManifestEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read compliance proof manifest: %w", err)
	}
	var envelope messageComplianceProofManifest
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Proofs) > 0 {
		return envelope.Proofs, nil
	}
	var entries []messageComplianceProofManifestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, errors.New("decode compliance proof manifest")
	}
	return entries, nil
}

func normalizeMessageComplianceProofManifestEntry(entry messageComplianceProofManifestEntry) messageComplianceProofManifestEntry {
	entry.ExternalProofRef = strings.TrimSpace(entry.ExternalProofRef)
	entry.Provider = strings.TrimSpace(entry.Provider)
	entry.ProofHash = strings.TrimSpace(entry.ProofHash)
	entry.Status = strings.ToUpper(strings.TrimSpace(entry.Status))
	return entry
}
