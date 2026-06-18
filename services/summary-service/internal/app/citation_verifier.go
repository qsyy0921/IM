package app

import "github.com/qsyy0921/IM/services/summary-service/internal/types"

func verifySummaryCitations(pack types.EvidencePack, summary types.SummaryGenerationResult) error {
	if summary.Status == types.SummaryStatusInsufficientEvidence {
		if len(summary.Citations) > 0 {
			return types.ErrCitationVerification
		}
		return nil
	}
	if summary.Status != types.SummaryStatusGrounded {
		return types.ErrCitationVerification
	}
	if summary.SummaryText == "" || len(summary.Citations) == 0 {
		return types.ErrCitationVerification
	}

	itemsByID := make(map[string]types.EvidenceItem, len(pack.Items))
	for _, item := range pack.Items {
		itemsByID[item.EvidenceID] = item
	}
	for _, citation := range summary.Citations {
		item, ok := itemsByID[citation.EvidenceID]
		if !ok || item.SourceType != citation.SourceType {
			return types.ErrCitationVerification
		}
		if len(item.SourceRefs) == 0 {
			if !citationMatchesItem(citation, item) {
				return types.ErrCitationVerification
			}
			continue
		}
		if !citationMatchesAnyRef(citation, item.SourceRefs) {
			return types.ErrCitationVerification
		}
	}
	return nil
}

func citationMatchesItem(citation types.Citation, item types.EvidenceItem) bool {
	return citation.SourceID == item.SourceID &&
		citation.SourceEventID == "" &&
		citation.ConversationID == item.ConversationID &&
		citation.ConversationSeq == item.ConversationSeq
}

func citationMatchesAnyRef(citation types.Citation, refs []types.EvidenceSourceRef) bool {
	for _, ref := range refs {
		if citation.SourceID == ref.SourceID &&
			citation.SourceEventID == ref.SourceEventID &&
			citation.ConversationID == ref.ConversationID &&
			citation.ConversationSeq == ref.ConversationSeq {
			return true
		}
	}
	return false
}
