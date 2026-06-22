package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReceiptOperatorReasonFromEnvDefault(t *testing.T) {
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON", "")
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON_FILE", "")
	reason, err := receiptOperatorReasonFromEnv("NEXUSIM_RECEIPT_TEST_REASON", "NEXUSIM_RECEIPT_TEST_REASON_FILE", "manual reason")
	if err != nil {
		t.Fatalf("resolve default reason: %v", err)
	}
	if reason != "manual reason" {
		t.Fatalf("unexpected default reason: %q", reason)
	}
}

func TestReceiptOperatorReasonFromEnvValue(t *testing.T) {
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON", " direct operator reason ")
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON_FILE", "")
	reason, err := receiptOperatorReasonFromEnv("NEXUSIM_RECEIPT_TEST_REASON", "NEXUSIM_RECEIPT_TEST_REASON_FILE", "manual reason")
	if err != nil {
		t.Fatalf("resolve env reason: %v", err)
	}
	if reason != "direct operator reason" {
		t.Fatalf("unexpected env reason: %q", reason)
	}
}

func TestReceiptOperatorReasonFromFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(reasonPath, []byte(" file-backed receipt reason \n"), 0o600); err != nil {
		t.Fatalf("write reason file: %v", err)
	}
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON", "")
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON_FILE", reasonPath)
	reason, err := receiptOperatorReasonFromEnv("NEXUSIM_RECEIPT_TEST_REASON", "NEXUSIM_RECEIPT_TEST_REASON_FILE", "manual reason")
	if err != nil {
		t.Fatalf("resolve file reason: %v", err)
	}
	if reason != "file-backed receipt reason" {
		t.Fatalf("unexpected file reason: %q", reason)
	}
}

func TestReceiptOperatorReasonRejectsAmbiguousEnvAndFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(reasonPath, []byte("file-backed reason"), 0o600); err != nil {
		t.Fatalf("write reason file: %v", err)
	}
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON", "direct reason")
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON_FILE", reasonPath)
	if _, err := receiptOperatorReasonFromEnv("NEXUSIM_RECEIPT_TEST_REASON", "NEXUSIM_RECEIPT_TEST_REASON_FILE", "manual reason"); err == nil {
		t.Fatal("expected ambiguous env/file reason to fail")
	}
}

func TestReceiptOperatorReasonRejectsEmptyFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	if err := os.WriteFile(reasonPath, []byte(" \n\t"), 0o600); err != nil {
		t.Fatalf("write reason file: %v", err)
	}
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON", "")
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON_FILE", reasonPath)
	if _, err := receiptOperatorReasonFromEnv("NEXUSIM_RECEIPT_TEST_REASON", "NEXUSIM_RECEIPT_TEST_REASON_FILE", "manual reason"); err == nil {
		t.Fatal("expected empty reason file to fail")
	}
}

func TestReceiptOperatorReasonRejectsLargeFile(t *testing.T) {
	reasonPath := filepath.Join(t.TempDir(), "reason.txt")
	payload := strings.Repeat("a", receiptOperatorReasonMaxBytes+1)
	if err := os.WriteFile(reasonPath, []byte(payload), 0o600); err != nil {
		t.Fatalf("write reason file: %v", err)
	}
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON", "")
	t.Setenv("NEXUSIM_RECEIPT_TEST_REASON_FILE", reasonPath)
	if _, err := receiptOperatorReasonFromEnv("NEXUSIM_RECEIPT_TEST_REASON", "NEXUSIM_RECEIPT_TEST_REASON_FILE", "manual reason"); err == nil {
		t.Fatal("expected oversized reason file to fail")
	}
}
