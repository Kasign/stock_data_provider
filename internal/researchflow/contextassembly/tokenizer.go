package contextassembly

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrTokenizerUnavailable = errors.New("model tokenizer is unavailable")
	ErrInvalidTokenEstimate = errors.New("invalid model token estimate")
)

// Tokenizer is the model-specific counting port. Production implementations
// must use the exact tokenizer/version associated with the target model.
type Tokenizer interface {
	ModelID() string
	Version() string
	CountTokens(text string) (int, error)
}

// SectionContent is authorized section text supplied by its resource Owner.
// The estimator does not fetch, persist, log or return this content.
type SectionContent struct {
	SectionID string
	Text      string
}

// TokenizerProvenance identifies the exact model/tokenizer used for estimates.
type TokenizerProvenance struct {
	ModelID string
	Version string
}

// EstimateSectionTokens returns a copy of candidates with token counts
// measured by the supplied model tokenizer. It rejects missing, duplicate or
// extra content so every estimate has an unambiguous section identity.
func EstimateSectionTokens(
	tokenizer Tokenizer,
	expectedModelID string,
	candidates []SectionCandidate,
	contents []SectionContent,
) ([]SectionCandidate, TokenizerProvenance, error) {
	if tokenizer == nil {
		return nil, TokenizerProvenance{}, ErrTokenizerUnavailable
	}
	if expectedModelID == "" || expectedModelID != strings.TrimSpace(expectedModelID) ||
		tokenizer.ModelID() != expectedModelID || tokenizer.Version() == "" ||
		tokenizer.Version() != strings.TrimSpace(tokenizer.Version()) {
		return nil, TokenizerProvenance{}, fmt.Errorf("%w: tokenizer identity does not match target model", ErrTokenizerUnavailable)
	}
	if len(candidates) == 0 || len(contents) != len(candidates) {
		return nil, TokenizerProvenance{}, fmt.Errorf("%w: section content does not match candidates", ErrInvalidTokenEstimate)
	}

	contentByID := make(map[string]string, len(contents))
	for _, content := range contents {
		if content.SectionID == "" || content.SectionID != strings.TrimSpace(content.SectionID) ||
			strings.TrimSpace(content.Text) == "" {
			return nil, TokenizerProvenance{}, fmt.Errorf("%w: invalid section content", ErrInvalidTokenEstimate)
		}
		if _, exists := contentByID[content.SectionID]; exists {
			return nil, TokenizerProvenance{}, fmt.Errorf("%w: duplicate content for section %q", ErrInvalidTokenEstimate, content.SectionID)
		}
		contentByID[content.SectionID] = content.Text
	}

	estimated := make([]SectionCandidate, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for i, candidate := range candidates {
		if candidate.ID == "" || candidate.ID != strings.TrimSpace(candidate.ID) {
			return nil, TokenizerProvenance{}, fmt.Errorf("%w: invalid candidate section id", ErrInvalidTokenEstimate)
		}
		if _, exists := seen[candidate.ID]; exists {
			return nil, TokenizerProvenance{}, fmt.Errorf("%w: duplicate candidate section %q", ErrInvalidTokenEstimate, candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
		content, exists := contentByID[candidate.ID]
		if !exists {
			return nil, TokenizerProvenance{}, fmt.Errorf("%w: missing content for section %q", ErrInvalidTokenEstimate, candidate.ID)
		}
		count, err := tokenizer.CountTokens(content)
		if err != nil {
			return nil, TokenizerProvenance{}, fmt.Errorf("count tokens for section %q: %w", candidate.ID, err)
		}
		if count <= 0 {
			return nil, TokenizerProvenance{}, fmt.Errorf("%w: section %q returned non-positive token count", ErrInvalidTokenEstimate, candidate.ID)
		}
		estimated[i] = candidate
		estimated[i].EstimatedTokens = count
		estimated[i].SourceRefs = append([]string(nil), candidate.SourceRefs...)
		estimated[i].EvidenceRefs = append([]string(nil), candidate.EvidenceRefs...)
		estimated[i].ProtectedEvidenceRefs = append([]string(nil), candidate.ProtectedEvidenceRefs...)
	}
	return estimated, TokenizerProvenance{ModelID: expectedModelID, Version: tokenizer.Version()}, nil
}
