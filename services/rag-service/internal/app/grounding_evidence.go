package app

import (
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/rag-service/internal/types"
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
	if err := validateEvidenceState(item); err != nil {
		return err
	}
	if len(item.SourceRefs) == 0 {
		return fmt.Errorf("%w: source_ref anchor is required", types.ErrCitationVerification)
	}
	for _, ref := range item.SourceRefs {
		if isValidGroundingSourceRef(item, ref) {
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

func validateEvidenceState(item types.EvidenceItem) error {
	switch item.SourceType {
	case types.EvidenceSourceSearchMessage:
		if item.VisibilityVersion <= 0 {
			return fmt.Errorf("%w: search evidence visibility_version is required", types.ErrCitationVerification)
		}
	case types.EvidenceSourceMemoryEvent:
		if item.VisibilityVersion <= 0 {
			return fmt.Errorf("%w: memory evidence visibility_version is required", types.ErrCitationVerification)
		}
		if item.TemporalStatus != "" && item.TemporalStatus != types.MemoryStatusActive {
			return fmt.Errorf("%w: memory evidence must be active", types.ErrCitationVerification)
		}
		if item.ReviewState != "" && item.ReviewState != "APPROVED" {
			return fmt.Errorf("%w: memory evidence must be approved", types.ErrCitationVerification)
		}
	case types.EvidenceSourceProfileAggregate:
		if item.TemporalStatus != "" && item.TemporalStatus != types.MemoryStatusActive {
			return fmt.Errorf("%w: profile evidence must be active", types.ErrCitationVerification)
		}
		if item.ReviewState != "" && item.ReviewState != "APPROVED" {
			return fmt.Errorf("%w: profile evidence must be approved", types.ErrCitationVerification)
		}
	}
	return nil
}

func isValidGroundingSourceRef(item types.EvidenceItem, ref types.EvidenceSourceRef) bool {
	if strings.TrimSpace(ref.SourceType) == "" || strings.TrimSpace(ref.SourceID) == "" {
		return false
	}
	conversationID := strings.TrimSpace(string(ref.ConversationID))
	if item.SourceType == types.EvidenceSourceSearchMessage || item.SourceType == types.EvidenceSourceMemoryEvent {
		return conversationID != "" && ref.ConversationSeq > 0
	}
	if conversationID == "" {
		return ref.ConversationSeq == 0
	}
	return ref.ConversationSeq > 0
}
