package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type gatewayTokenKeyRingRotateOutput struct {
	GeneratedAt       string   `json:"generated_at"`
	Algorithm         string   `json:"algorithm"`
	CurrentKeyID      string   `json:"current_kid"`
	OldPublicKeyCount int      `json:"old_public_key_count"`
	OldPublicKeyIDs   []string `json:"old_public_kids,omitempty"`
	RSABits           int      `json:"rsa_bits"`
	OldKeyLimit       int      `json:"old_key_limit"`
}

func writeGatewayTokenKeyRingRotateOutput(path string, rotated gatewayTokenRS256KeyRingConfig, options gatewayTokenKeyRingRotateOptions) error {
	oldPublicKeyIDs := make([]string, 0, len(rotated.OldPublicKeys))
	for _, key := range rotated.OldPublicKeys {
		if key.KeyID != "" {
			oldPublicKeyIDs = append(oldPublicKeyIDs, key.KeyID)
		}
	}
	output := gatewayTokenKeyRingRotateOutput{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Algorithm:         "RS256",
		CurrentKeyID:      rotated.Current.KeyID,
		OldPublicKeyCount: len(rotated.OldPublicKeys),
		OldPublicKeyIDs:   oldPublicKeyIDs,
		RSABits:           options.RSABits,
		OldKeyLimit:       options.OldKeyLimit,
	}
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
