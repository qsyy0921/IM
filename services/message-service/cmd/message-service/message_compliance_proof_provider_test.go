package main

import (
	"os"
	"path/filepath"
	"testing"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

func TestResolveMessageComplianceProofRegisterOptionsManual(t *testing.T) {
	options := postgresinfra.MessageComplianceExternalProofMutationOptions{
		TenantID:         "tenant-a",
		ExternalProofRef: "proof://case/a",
		Provider:         "manual-legal-system",
		ProofHash:        "sha256:manual",
		OperatorID:       "legal-ops",
	}
	result, err := resolveMessageComplianceProofRegisterOptions(options, "manual", "")
	if err != nil {
		t.Fatalf("resolve manual proof options: %v", err)
	}
	if result.Provider != "manual-legal-system" || result.ProofHash != "sha256:manual" {
		t.Fatalf("manual mode changed provider/hash: %+v", result)
	}
}

func TestResolveMessageComplianceProofRegisterOptionsFromManifest(t *testing.T) {
	manifestPath := writeComplianceProofManifest(t, `{
  "proofs": [
    {
      "external_proof_ref": "proof://case/a",
      "provider": "legal-archive",
      "proof_hash": "sha256:abc123",
      "status": "VERIFIED",
      "proof_body": "must not be parsed or persisted"
    }
  ]
}`)
	options := postgresinfra.MessageComplianceExternalProofMutationOptions{
		TenantID:         "tenant-a",
		ExternalProofRef: " proof://case/a ",
		OperatorID:       "legal-ops",
	}
	result, err := resolveMessageComplianceProofRegisterOptions(options, "manifest", manifestPath)
	if err != nil {
		t.Fatalf("resolve manifest proof options: %v", err)
	}
	if result.ExternalProofRef != "proof://case/a" ||
		result.Provider != "legal-archive" ||
		result.ProofHash != "sha256:abc123" {
		t.Fatalf("unexpected manifest-resolved options: %+v", result)
	}
}

func TestResolveMessageComplianceProofRegisterOptionsFromManifestArray(t *testing.T) {
	manifestPath := writeComplianceProofManifest(t, `[
  {
    "external_proof_ref": "proof://case/a",
    "provider": "legal-archive",
    "proof_hash": "sha256:abc123",
    "status": "VERIFIED"
  }
]`)
	options := postgresinfra.MessageComplianceExternalProofMutationOptions{
		TenantID:         "tenant-a",
		ExternalProofRef: "proof://case/a",
		OperatorID:       "legal-ops",
	}
	result, err := resolveMessageComplianceProofRegisterOptions(options, "manifest", manifestPath)
	if err != nil {
		t.Fatalf("resolve array manifest proof options: %v", err)
	}
	if result.Provider != "legal-archive" || result.ProofHash != "sha256:abc123" {
		t.Fatalf("unexpected array manifest options: %+v", result)
	}
}

func TestResolveMessageComplianceProofRegisterOptionsManifestRequiresVerified(t *testing.T) {
	manifestPath := writeComplianceProofManifest(t, `{
  "proofs": [
    {
      "external_proof_ref": "proof://case/a",
      "provider": "legal-archive",
      "proof_hash": "sha256:abc123",
      "status": "REVOKED"
    }
  ]
}`)
	options := postgresinfra.MessageComplianceExternalProofMutationOptions{
		TenantID:         "tenant-a",
		ExternalProofRef: "proof://case/a",
		OperatorID:       "legal-ops",
	}
	if _, err := resolveMessageComplianceProofRegisterOptions(options, "manifest", manifestPath); err == nil {
		t.Fatal("expected revoked manifest proof to fail")
	}
}

func TestResolveMessageComplianceProofRegisterOptionsManifestRejectsMismatch(t *testing.T) {
	manifestPath := writeComplianceProofManifest(t, `{
  "proofs": [
    {
      "external_proof_ref": "proof://case/a",
      "provider": "legal-archive",
      "proof_hash": "sha256:abc123",
      "status": "VERIFIED"
    }
  ]
}`)
	options := postgresinfra.MessageComplianceExternalProofMutationOptions{
		TenantID:         "tenant-a",
		ExternalProofRef: "proof://case/a",
		Provider:         "other-provider",
		OperatorID:       "legal-ops",
	}
	if _, err := resolveMessageComplianceProofRegisterOptions(options, "manifest", manifestPath); err == nil {
		t.Fatal("expected provider mismatch to fail")
	}
}

func writeComplianceProofManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "proof-manifest.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write proof manifest: %v", err)
	}
	return path
}
