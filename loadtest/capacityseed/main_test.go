package main

import (
	"strings"
	"testing"
)

func TestParseConfigRejectsMissingDSN(t *testing.T) {
	_, err := parseConfig([]string{})
	if err == nil || !strings.Contains(err.Error(), "--pg-dsn is required") {
		t.Fatalf("parseConfig error = %v, want missing dsn", err)
	}
}

func TestParseConfigDefaultsMatchCapacitySuite(t *testing.T) {
	cfg, err := parseConfig([]string{"--pg-dsn", "postgres://example"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.messageTenantID != "tenant-capacity-message" {
		t.Fatalf("message tenant = %q", cfg.messageTenantID)
	}
	if cfg.messageConversationPrefix != "conv-capacity-message" {
		t.Fatalf("message conversation prefix = %q", cfg.messageConversationPrefix)
	}
	if cfg.messageConversationCount != 10 {
		t.Fatalf("message conversation count = %d", cfg.messageConversationCount)
	}
	if cfg.messageVUs != 2 {
		t.Fatalf("message vus = %d", cfg.messageVUs)
	}
	if cfg.conversationTenantID != "tenant-capacity-conversation" {
		t.Fatalf("conversation tenant = %q", cfg.conversationTenantID)
	}
	if cfg.conversationID != "conv-capacity-memberchange" {
		t.Fatalf("conversation id = %q", cfg.conversationID)
	}
	if cfg.conversationOwnerUserID != "owner-1" {
		t.Fatalf("conversation owner = %q", cfg.conversationOwnerUserID)
	}
	if cfg.deliveryTenantID != "tenant-capacity-delivery" {
		t.Fatalf("delivery tenant = %q", cfg.deliveryTenantID)
	}
	if cfg.deliveryConversationID != "conv-capacity-delivery" {
		t.Fatalf("delivery conversation = %q", cfg.deliveryConversationID)
	}
	if cfg.deliveryUserID != "delivery-user-1" {
		t.Fatalf("delivery user = %q", cfg.deliveryUserID)
	}
	if cfg.deliveryMessageCount != 1 {
		t.Fatalf("delivery message count = %d", cfg.deliveryMessageCount)
	}
}

func TestParseConfigRejectsNonPositiveCounts(t *testing.T) {
	cases := [][]string{
		{"--pg-dsn", "postgres://example", "--message-conversation-count", "0"},
		{"--pg-dsn", "postgres://example", "--message-vus", "0"},
		{"--pg-dsn", "postgres://example", "--delivery-message-count", "0"},
	}
	for _, args := range cases {
		if _, err := parseConfig(args); err == nil {
			t.Fatalf("parseConfig(%v) succeeded, want error", args)
		}
	}
}
