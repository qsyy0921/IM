package main

import (
	"strings"
	"testing"
)

func TestValidateConfigRequiresPGVectorDSNForPreflight(t *testing.T) {
	cfg := validTestConfig()
	cfg.phase = "preflight-pgvector"
	cfg.pgVectorDSN = ""
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "pgvector-dsn") {
		t.Fatalf("expected pgvector-dsn validation error, got %v", err)
	}

	cfg.pgVectorDSN = "postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected valid pgvector preflight config: %v", err)
	}
}

func TestValidateConfigRejectsMissingPGVectorTable(t *testing.T) {
	cfg := validTestConfig()
	cfg.phase = "verify"
	cfg.vectorTarget = "127.0.0.1:10760"
	cfg.visibilityScope = "tenant:tenant-vector"
	cfg.policyVersion = "policy-v1"
	cfg.expectedCount = 1
	cfg.pgVectorDSN = "postgres://nexusim:nexusim@localhost:15432/nexusim?sslmode=disable"
	cfg.pgVectorTable = ""

	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "pgvector-table") {
		t.Fatalf("expected pgvector-table validation error, got %v", err)
	}
}

func TestQuoteSQLIdentifierRejectsUnsafePGVectorTable(t *testing.T) {
	quoted, err := quoteSQLIdentifier("vector_index.vector_embedding_items")
	if err != nil {
		t.Fatalf("expected safe schema-qualified identifier: %v", err)
	}
	if quoted != `"vector_index"."vector_embedding_items"` {
		t.Fatalf("unexpected quoted identifier: %s", quoted)
	}

	if _, err := quoteSQLIdentifier("vector_embedding_items;drop table vector_items"); err == nil {
		t.Fatal("expected unsafe identifier to fail")
	}
}

func validTestConfig() config {
	return config{
		phase:           "prepare",
		knowledgeTarget: "127.0.0.1:10740",
		vectorTarget:    "127.0.0.1:10760",
		pgDSN:           "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
		pgVectorTable:   "vector_embedding_items",
		tenantID:        "tenant-vector",
		userID:          "user-vector",
		expectedCount:   1,
	}
}
