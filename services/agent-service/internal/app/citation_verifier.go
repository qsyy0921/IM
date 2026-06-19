package app

import (
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func verifyProposalCitations(
	pack types.EvidencePack,
	generation types.AgentProposalGenerationResult,
) error {
	if len(generation.Citations) == 0 && len(pack.Items) > 0 {
		return fmt.Errorf("%w: missing citations", types.ErrCitationVerification)
	}
	known := make(map[string]types.EvidenceItem, len(pack.Items))
	for _, item := range pack.Items {
		if strings.TrimSpace(item.EvidenceID) != "" {
			known[item.EvidenceID] = item
		}
	}
	for _, citation := range generation.Citations {
		if strings.TrimSpace(citation.EvidenceID) == "" {
			return fmt.Errorf("%w: citation missing evidence_id", types.ErrCitationVerification)
		}
		if _, ok := known[citation.EvidenceID]; !ok {
			return fmt.Errorf("%w: citation references unknown evidence", types.ErrCitationVerification)
		}
	}
	return nil
}
