package app

import (
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

func verifyAnswerCitations(pack types.EvidencePack, answer types.AnswerGenerationResult) error {
	switch answer.Status {
	case types.AnswerStatusInsufficientEvidence:
		if len(answer.Citations) > 0 {
			return fmt.Errorf("%w: insufficient answer must not include citations", types.ErrCitationVerification)
		}
		return nil
	case types.AnswerStatusGrounded:
		if strings.TrimSpace(answer.AnswerText) == "" {
			return fmt.Errorf("%w: grounded answer text is empty", types.ErrCitationVerification)
		}
		if len(answer.Citations) == 0 {
			return fmt.Errorf("%w: grounded answer has no citations", types.ErrCitationVerification)
		}
	default:
		return fmt.Errorf("%w: unsupported answer status", types.ErrCitationVerification)
	}

	for _, citation := range answer.Citations {
		if !citationMatchesEvidence(pack.Items, citation) {
			return fmt.Errorf("%w: citation does not match evidence", types.ErrCitationVerification)
		}
	}
	return nil
}

func citationMatchesEvidence(items []types.EvidenceItem, citation types.Citation) bool {
	for _, item := range items {
		if citation.EvidenceID != item.EvidenceID || citation.SourceType != item.SourceType {
			continue
		}
		if len(item.SourceRefs) == 0 {
			if citation.SourceID != item.SourceID {
				continue
			}
			if citation.ConversationID != item.ConversationID || citation.ConversationSeq != item.ConversationSeq {
				continue
			}
			if citation.SourceEventID != "" {
				continue
			}
			return true
		}
		for _, ref := range item.SourceRefs {
			if citation.SourceID == ref.SourceID &&
				citation.SourceEventID == ref.SourceEventID &&
				citation.ConversationID == ref.ConversationID &&
				citation.ConversationSeq == ref.ConversationSeq {
				return true
			}
		}
	}
	return false
}
