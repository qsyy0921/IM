package app

import (
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/summary-service/internal/types"
)

func groundableEvidencePack(pack types.EvidencePack) (types.EvidencePack, error) {
	seenEvidenceIDs := make(map[string]struct{}, len(pack.Items))
	for _, item := range pack.Items {
		evidenceID := strings.TrimSpace(item.EvidenceID)
		if evidenceID == "" {
			return types.EvidencePack{}, fmt.Errorf("%w: evidence_id is required", types.ErrCitationVerification)
		}
		if _, exists := seenEvidenceIDs[evidenceID]; exists {
			return types.EvidencePack{}, fmt.Errorf("%w: duplicate evidence_id", types.ErrCitationVerification)
		}
		seenEvidenceIDs[evidenceID] = struct{}{}
	}
	items := make([]types.EvidenceItem, 0, len(pack.Items))
	for _, item := range pack.Items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		if err := validateGroundingEvidenceItem(item); err != nil {
			return types.EvidencePack{}, err
		}
		items = append(items, item)
	}
	pack.Items = items
	return pack, nil
}

func validateGroundingEvidenceItem(item types.EvidenceItem) error {
	if !isGroundableSourceType(item.SourceType) {
		return fmt.Errorf("%w: unsupported evidence source type", types.ErrCitationVerification)
	}
	if strings.TrimSpace(item.SourceID) == "" {
		return fmt.Errorf("%w: source_id is required", types.ErrCitationVerification)
	}
	if len(item.SourceRefs) == 0 {
		if item.ConversationSeq > 0 && strings.TrimSpace(string(item.ConversationID)) == "" {
			return fmt.Errorf("%w: conversation_id is required for sequenced evidence", types.ErrCitationVerification)
		}
		if strings.TrimSpace(string(item.ConversationID)) != "" && item.ConversationSeq <= 0 {
			return fmt.Errorf("%w: conversation_seq is required for conversation evidence", types.ErrCitationVerification)
		}
		return nil
	}
	for _, ref := range item.SourceRefs {
		if isValidGroundingSourceRef(ref) {
			return nil
		}
	}
	return fmt.Errorf("%w: source_ref anchor is required", types.ErrCitationVerification)
}

func isGroundableSourceType(sourceType string) bool {
	switch sourceType {
	case types.EvidenceSourceSearchMessage,
		types.EvidenceSourceMemoryEvent,
		types.EvidenceSourceProfileAggregate:
		return true
	default:
		return false
	}
}

func isValidGroundingSourceRef(ref types.EvidenceSourceRef) bool {
	if strings.TrimSpace(ref.SourceType) == "" || strings.TrimSpace(ref.SourceID) == "" {
		return false
	}
	conversationID := strings.TrimSpace(string(ref.ConversationID))
	if conversationID == "" {
		return ref.ConversationSeq == 0
	}
	return ref.ConversationSeq > 0
}
