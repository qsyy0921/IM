package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tokeninfra "github.com/qsyy0921/IM/services/identity-service/internal/infrastructure/token"
)

func TestWriteGatewayTokenKeyRingRotateOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "rotate.json")
	rotated := gatewayTokenRS256KeyRingConfig{
		Current: gatewayTokenRS256CurrentKey{
			KeyID:         "current-new",
			PrivateKeyPEM: "must-not-be-written",
		},
		OldPublicKeys: []tokeninfra.JWK{
			{KeyID: "old-current", Modulus: "must-not-be-written", Exponent: "AQAB"},
			{KeyID: "old-previous", Modulus: "must-not-be-written", Exponent: "AQAB"},
		},
	}

	if err := writeGatewayTokenKeyRingRotateOutput(outputPath, rotated, gatewayTokenKeyRingRotateOptions{
		RSABits:     3072,
		OldKeyLimit: 2,
	}); err != nil {
		t.Fatalf("writeGatewayTokenKeyRingRotateOutput() error = %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(raw), "must-not-be-written") {
		t.Fatalf("rotate output leaked key material: %s", string(raw))
	}
	var output gatewayTokenKeyRingRotateOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" ||
		output.Algorithm != "RS256" ||
		output.CurrentKeyID != "current-new" ||
		output.OldPublicKeyCount != 2 ||
		output.RSABits != 3072 ||
		output.OldKeyLimit != 2 {
		t.Fatalf("unexpected rotate output: %+v", output)
	}
	if len(output.OldPublicKeyIDs) != 2 ||
		output.OldPublicKeyIDs[0] != "old-current" ||
		output.OldPublicKeyIDs[1] != "old-previous" {
		t.Fatalf("unexpected old public kid list: %+v", output.OldPublicKeyIDs)
	}
}
