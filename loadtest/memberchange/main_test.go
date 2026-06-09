package main

import (
	"testing"

	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
)

func TestParseMemberChangeType(t *testing.T) {
	tests := []struct {
		name string
		want conversationv1.MemberChangeType
	}{
		{name: "join", want: conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN},
		{name: "LEAVE", want: conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_LEAVE},
		{name: "remove", want: conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_REMOVE},
		{name: "role-changed", want: conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_ROLE_CHANGED},
	}
	for _, test := range tests {
		got, _, err := parseMemberChangeType(test.name)
		if err != nil {
			t.Fatalf("parseMemberChangeType(%q) returned error: %v", test.name, err)
		}
		if got != test.want {
			t.Fatalf("parseMemberChangeType(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestIsOwnerTransferChange(t *testing.T) {
	tests := []string{"owner-transfer", "OWNER_TRANSFER", "owner transfer"}
	for _, test := range tests {
		if !isOwnerTransferChange(test) {
			t.Fatalf("isOwnerTransferChange(%q) = false, want true", test)
		}
	}
	if isOwnerTransferChange("role-changed") {
		t.Fatal("isOwnerTransferChange(role-changed) = true, want false")
	}
}

func TestParseConfigDefaultsListUserToOperator(t *testing.T) {
	cfg := normalizeConfigDefaults(config{operatorUserID: "operator-1"})
	if cfg.listUserID != cfg.operatorUserID {
		t.Fatalf("listUserID = %q, want operator user %q", cfg.listUserID, cfg.operatorUserID)
	}
}

func TestParseMemberRole(t *testing.T) {
	tests := []struct {
		name string
		want conversationv1.MemberRole
	}{
		{name: "owner", want: conversationv1.MemberRole_MEMBER_ROLE_OWNER},
		{name: "ADMIN", want: conversationv1.MemberRole_MEMBER_ROLE_ADMIN},
		{name: "member", want: conversationv1.MemberRole_MEMBER_ROLE_MEMBER},
	}
	for _, test := range tests {
		got, _, err := parseMemberRole(test.name)
		if err != nil {
			t.Fatalf("parseMemberRole(%q) returned error: %v", test.name, err)
		}
		if got != test.want {
			t.Fatalf("parseMemberRole(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}
