package moderation

import (
	"context"
	"testing"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestKeywordModeratorDeniesMatchingMessageText(t *testing.T) {
	moderator := NewKeywordModerator(KeywordConfig{
		DenyTerms:         []string{"secret", "SECRET"},
		PermissionVersion: 12,
		Classification:    "CONTENT_REVIEW",
		Reason:            "content rejected",
	})
	decision, handled, err := moderator.ModerateMessageContent(context.Background(), types.CheckMessageActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
		},
		ConversationID: "conv-1",
		Action:         types.MessageActionSend,
		MessageText:    "contains Secret marker",
	})
	if err != nil {
		t.Fatalf("moderate message content: %v", err)
	}
	if !handled || decision.Allowed || decision.PermissionVersion != 12 || decision.Classification != "CONTENT_REVIEW" || decision.Reason != "content rejected" || decision.DecisionSource != types.PolicyDecisionSourceContentModeration {
		t.Fatalf("unexpected moderation decision: handled=%v decision=%+v", handled, decision)
	}
}

func TestKeywordModeratorSkipsEmptyTermsAndText(t *testing.T) {
	moderator := NewKeywordModerator(KeywordConfig{DenyTerms: []string{"", "   "}})
	if decision, handled, err := moderator.ModerateMessageContent(context.Background(), types.CheckMessageActionCommand{
		Action:      types.MessageActionSend,
		MessageText: "secret",
	}); err != nil || handled || decision.Classification != "" {
		t.Fatalf("expected empty deny terms to skip, handled=%v decision=%+v err=%v", handled, decision, err)
	}

	moderator = NewKeywordModerator(KeywordConfig{DenyTerms: []string{"secret"}})
	if _, handled, err := moderator.ModerateMessageContent(context.Background(), types.CheckMessageActionCommand{
		Action: types.MessageActionSend,
	}); err != nil || handled {
		t.Fatalf("expected empty text to skip, handled=%v err=%v", handled, err)
	}
}
