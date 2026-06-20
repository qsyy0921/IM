package main

import (
	"path/filepath"
	"testing"
)

func TestActionExternalMCPFallbackSmokeParseConfig(t *testing.T) {
	output := filepath.Join(t.TempDir(), "summary.json")
	cfg, err := parseConfig([]string{"--output", output})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.outputPath != output {
		t.Fatalf("unexpected output path: %q", cfg.outputPath)
	}
}
