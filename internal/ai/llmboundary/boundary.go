package llmboundary

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	DefaultTokenBudget      = 1200
	DefaultMaxEvidenceItems = 8
	DefaultMaxTextRunes     = 600
)

var (
	ErrUnsafeInput     = errors.New("llm input rejected")
	ErrUnsafeOutput    = errors.New("llm output rejected")
	ErrMalformedOutput = errors.New("llm output malformed")
)

type Options struct {
	TokenBudget      int
	MaxEvidenceItems int
	MaxTextRunes     int
}

type Evidence struct {
	EvidenceID string
	Text       string
}

type Prompt struct {
	Task        string
	Query       string
	TokenBudget int
	Evidence    []Evidence
}

type Candidate struct {
	Text                string
	CitationEvidenceIDs []string
	Confidence          float64
}

var sensitiveTextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`),
	regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._-]{12,}`),
	regexp.MustCompile(`-----BEGIN\s+(RSA\s+)?PRIVATE KEY-----`),
	regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`),
	regexp.MustCompile(`\b1[3-9]\d{9}\b`),
}

func DefaultOptions() Options {
	return Options{
		TokenBudget:      DefaultTokenBudget,
		MaxEvidenceItems: DefaultMaxEvidenceItems,
		MaxTextRunes:     DefaultMaxTextRunes,
	}
}

func NormalizeOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.TokenBudget <= 0 {
		options.TokenBudget = defaults.TokenBudget
	}
	if options.MaxEvidenceItems <= 0 {
		options.MaxEvidenceItems = defaults.MaxEvidenceItems
	}
	if options.MaxTextRunes <= 0 {
		options.MaxTextRunes = defaults.MaxTextRunes
	}
	return options
}

func BuildPrompt(task string, query string, evidence []Evidence, options Options) (Prompt, error) {
	options = NormalizeOptions(options)
	task = strings.TrimSpace(task)
	query = strings.TrimSpace(query)
	if task == "" {
		return Prompt{}, fmt.Errorf("%w: task is required", ErrMalformedOutput)
	}
	if ContainsSensitiveText(query) {
		return Prompt{}, fmt.Errorf("%w: query contains sensitive text", ErrUnsafeInput)
	}
	prompt := Prompt{
		Task:        task,
		Query:       truncateRunes(query, options.MaxTextRunes),
		TokenBudget: options.TokenBudget,
		Evidence:    make([]Evidence, 0, min(len(evidence), options.MaxEvidenceItems)),
	}
	for _, item := range evidence {
		if len(prompt.Evidence) >= options.MaxEvidenceItems {
			break
		}
		evidenceID := strings.TrimSpace(item.EvidenceID)
		text := strings.TrimSpace(item.Text)
		if evidenceID == "" || text == "" {
			continue
		}
		if ContainsSensitiveText(evidenceID) || ContainsSensitiveText(text) {
			return Prompt{}, fmt.Errorf("%w: evidence contains sensitive text", ErrUnsafeInput)
		}
		prompt.Evidence = append(prompt.Evidence, Evidence{
			EvidenceID: evidenceID,
			Text:       truncateRunes(text, options.MaxTextRunes),
		})
	}
	if len(prompt.Evidence) == 0 {
		return Prompt{}, fmt.Errorf("%w: evidence is required", ErrMalformedOutput)
	}
	return prompt, nil
}

func ValidateCandidate(candidate Candidate, allowedEvidenceIDs map[string]struct{}) error {
	text := strings.TrimSpace(candidate.Text)
	if text == "" {
		return fmt.Errorf("%w: text is required", ErrMalformedOutput)
	}
	if ContainsSensitiveText(text) {
		return fmt.Errorf("%w: text contains sensitive value", ErrUnsafeOutput)
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 {
		return fmt.Errorf("%w: confidence must be in [0,1]", ErrMalformedOutput)
	}
	if len(candidate.CitationEvidenceIDs) == 0 {
		return fmt.Errorf("%w: citation evidence ids are required", ErrMalformedOutput)
	}
	seen := make(map[string]struct{}, len(candidate.CitationEvidenceIDs))
	for _, rawID := range candidate.CitationEvidenceIDs {
		evidenceID := strings.TrimSpace(rawID)
		if evidenceID == "" {
			return fmt.Errorf("%w: empty citation evidence id", ErrMalformedOutput)
		}
		if ContainsSensitiveText(evidenceID) {
			return fmt.Errorf("%w: citation contains sensitive value", ErrUnsafeOutput)
		}
		if _, ok := allowedEvidenceIDs[evidenceID]; !ok {
			return fmt.Errorf("%w: citation is not in EvidencePack", ErrMalformedOutput)
		}
		if _, duplicated := seen[evidenceID]; duplicated {
			return fmt.Errorf("%w: duplicated citation evidence id", ErrMalformedOutput)
		}
		seen[evidenceID] = struct{}{}
	}
	return nil
}

func ContainsSensitiveText(value string) bool {
	for _, pattern := range sensitiveTextPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func truncateRunes(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}
