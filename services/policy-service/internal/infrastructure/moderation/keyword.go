package moderation

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type KeywordConfig struct {
	DenyTerms         []string
	PermissionVersion int64
	Classification    string
	Reason            string
}

type KeywordModerator struct {
	terms             []string
	permissionVersion int64
	classification    string
	reason            string
}

func NewKeywordModerator(config KeywordConfig) KeywordModerator {
	terms := make([]string, 0, len(config.DenyTerms))
	seen := make(map[string]struct{}, len(config.DenyTerms))
	for _, term := range config.DenyTerms {
		normalized := strings.ToLower(strings.TrimSpace(term))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		terms = append(terms, normalized)
	}
	permissionVersion := config.PermissionVersion
	if permissionVersion <= 0 {
		permissionVersion = 1
	}
	classification := strings.TrimSpace(config.Classification)
	if classification == "" {
		classification = "CONTENT_MODERATION_DENIED"
	}
	reason := strings.TrimSpace(config.Reason)
	if reason == "" {
		reason = "content moderation policy denied"
	}
	return KeywordModerator{
		terms:             terms,
		permissionVersion: permissionVersion,
		classification:    classification,
		reason:            reason,
	}
}

func (m KeywordModerator) ModerateMessageContent(
	_ context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	if len(m.terms) == 0 {
		return types.MessageActionDecision{}, false, nil
	}
	text := strings.ToLower(strings.TrimSpace(command.MessageText))
	if text == "" {
		return types.MessageActionDecision{}, false, nil
	}
	for _, term := range m.terms {
		if strings.Contains(text, term) {
			return types.MessageActionDecision{
				TenantID:          command.AuthContext.TenantID,
				UserID:            command.AuthContext.UserID,
				ConversationID:    command.ConversationID,
				MessageID:         command.MessageID,
				Action:            command.Action,
				Allowed:           false,
				PermissionVersion: m.permissionVersion,
				Classification:    m.classification,
				Reason:            m.reason,
				DecisionSource:    types.PolicyDecisionSourceContentModeration,
			}, true, nil
		}
	}
	return types.MessageActionDecision{}, false, nil
}
