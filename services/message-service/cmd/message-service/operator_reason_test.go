package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessageOperatorReasonFromEnvDefault(t *testing.T) {
	t.Setenv("NEXUSIM_TEST_REASON", "")
	t.Setenv("NEXUSIM_TEST_REASON_FILE", "")
	reason, err := messageOperatorReasonFromEnv("NEXUSIM_TEST_REASON", "NEXUSIM_TEST_REASON_FILE", "manual fallback")
	if err != nil {
		t.Fatalf("resolve default reason: %v", err)
	}
	if reason != "manual fallback" {
		t.Fatalf("unexpected default reason: %q", reason)
	}
}

func TestMessageOperatorReasonFromEnvValue(t *testing.T) {
	t.Setenv("NEXUSIM_TEST_REASON", " direct operator reason ")
	t.Setenv("NEXUSIM_TEST_REASON_FILE", "")
	reason, err := messageOperatorReasonFromEnv("NEXUSIM_TEST_REASON", "NEXUSIM_TEST_REASON_FILE", "manual fallback")
	if err != nil {
		t.Fatalf("resolve env reason: %v", err)
	}
	if reason != "direct operator reason" {
		t.Fatalf("unexpected env reason: %q", reason)
	}
}

func TestMessageOperatorReasonFromFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(reasonPath, []byte(" file-backed reason \n"), 0o600); err != nil {
		t.Fatalf("write reason file: %v", err)
	}
	t.Setenv("NEXUSIM_TEST_REASON", "")
	t.Setenv("NEXUSIM_TEST_REASON_FILE", reasonPath)
	reason, err := messageOperatorReasonFromEnv("NEXUSIM_TEST_REASON", "NEXUSIM_TEST_REASON_FILE", "manual fallback")
	if err != nil {
		t.Fatalf("resolve file reason: %v", err)
	}
	if reason != "file-backed reason" {
		t.Fatalf("unexpected file reason: %q", reason)
	}
}

func TestMessageOperatorReasonRejectsAmbiguousEnvAndFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(reasonPath, []byte("file-backed reason"), 0o600); err != nil {
		t.Fatalf("write reason file: %v", err)
	}
	t.Setenv("NEXUSIM_TEST_REASON", "direct reason")
	t.Setenv("NEXUSIM_TEST_REASON_FILE", reasonPath)
	if _, err := messageOperatorReasonFromEnv("NEXUSIM_TEST_REASON", "NEXUSIM_TEST_REASON_FILE", "manual fallback"); err == nil {
		t.Fatal("expected ambiguous env/file reason to fail")
	}
}

func TestMessageOperatorReasonRejectsEmptyFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(reasonPath, []byte(" \n\t"), 0o600); err != nil {
		t.Fatalf("write reason file: %v", err)
	}
	t.Setenv("NEXUSIM_TEST_REASON", "")
	t.Setenv("NEXUSIM_TEST_REASON_FILE", reasonPath)
	if _, err := messageOperatorReasonFromEnv("NEXUSIM_TEST_REASON", "NEXUSIM_TEST_REASON_FILE", "manual fallback"); err == nil {
		t.Fatal("expected empty reason file to fail")
	}
}

func TestMessageOperatorReasonRejectsLargeFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	payload := strings.Repeat("a", messageOperatorReasonMaxBytes+1)
	if err := os.WriteFile(reasonPath, []byte(payload), 0o600); err != nil {
		t.Fatalf("write reason file: %v", err)
	}
	t.Setenv("NEXUSIM_TEST_REASON", "")
	t.Setenv("NEXUSIM_TEST_REASON_FILE", reasonPath)
	if _, err := messageOperatorReasonFromEnv("NEXUSIM_TEST_REASON", "NEXUSIM_TEST_REASON_FILE", "manual fallback"); err == nil {
		t.Fatal("expected oversized reason file to fail")
	}
}
