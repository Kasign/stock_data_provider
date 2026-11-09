package contextassembly

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

var (
	ErrInvalidModelInput = errors.New("invalid model input composition")
	ErrModelInputOverBudget = errors.New("composed model input exceeds available token budget")
)

// ModelInputSection is the authorized, versioned section payload sent to the
// model. Source references are retained so the model can cite, not invent,
// provenance.
type ModelInputSection struct {
	SectionID     string   `json:"section_id"`
	SectionVersion string  `json:"section_version"`
	SourceOwner   string   `json:"source_owner"`
	SourceRefs    []string `json:"source_refs"`
	EvidenceRefs  []string `json:"evidence_refs"`
	ProtectedEvidenceRefs []string `json:"protected_evidence_refs"`
	QualityStatus string   `json:"quality_status"`
	Required      bool     `json:"required"`
	AssembledAt   string   `json:"assembled_at"`
	Text          string   `json:"text"`
}

// ModelInputEnvelope is the deterministic JSON payload passed as LLM input.
type ModelInputEnvelope struct {
	SchemaVersion   string              `json:"schema_version"`
	ResearchQuestion string             `json:"research_question"`
	Scope            string             `json:"scope"`
	SubjectRefs      []string            `json:"subject_refs"`
	SystemAsOf       string              `json:"system_as_of"`
	BuilderVersion   string              `json:"builder_version"`
	QualitySummary   researchapp.ContextQualitySummary `json:"quality_summary"`
	OmittedSections []researchapp.ContextOmittedSection `json:"omitted_sections"`
	Sections         []ModelInputSection `json:"sections"`
}

// ComposedModelInput contains the final serialized input and exact tokenizer
// count for that serialization. It contains authorized content only in memory.
type ComposedModelInput struct {
	Text            string
	TokenCount       int
	Tokenizer       TokenizerProvenance
}

// ComposeModelInput serializes only the sections selected by Plan, then counts
// the exact final JSON payload using the target model tokenizer. It does not
// fetch content, authorize sources, persist the payload or call the LLM.
func ComposeModelInput(
	snapshot ResearchContextSnapshot,
	candidates []SectionCandidate,
	metadata map[string]SectionMetadata,
	plan Plan,
	contents []SectionContent,
	tokenizer Tokenizer,
	expectedModelID string,
	availableTokens int,
) (ComposedModelInput, error) {
	if availableTokens <= 0 {
		return ComposedModelInput{}, fmt.Errorf("%w: available token budget must be positive", ErrInvalidModelInput)
	}
	if _, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan); err != nil {
		return ComposedModelInput{}, fmt.Errorf("%w: invalid context plan: %v", ErrInvalidModelInput, err)
	}
	if tokenizer == nil || expectedModelID == "" || expectedModelID != strings.TrimSpace(expectedModelID) ||
		tokenizer.ModelID() != expectedModelID || tokenizer.Version() == "" ||
		tokenizer.Version() != strings.TrimSpace(tokenizer.Version()) {
		return ComposedModelInput{}, ErrTokenizerUnavailable
	}

	command, _ := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan)
	candidateByID := make(map[string]SectionCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.ID] = candidate
	}
	contentByID := make(map[string]string, len(contents))
	for _, content := range contents {
		if content.SectionID == "" || content.SectionID != strings.TrimSpace(content.SectionID) ||
			strings.TrimSpace(content.Text) == "" {
			return ComposedModelInput{}, fmt.Errorf("%w: invalid section content", ErrInvalidModelInput)
		}
		if _, exists := contentByID[content.SectionID]; exists {
			return ComposedModelInput{}, fmt.Errorf("%w: duplicate content for section %q", ErrInvalidModelInput, content.SectionID)
		}
		contentByID[content.SectionID] = content.Text
	}
	if len(contentByID) != len(plan.IncludedSections) {
		return ComposedModelInput{}, fmt.Errorf("%w: content count does not match included sections", ErrInvalidModelInput)
	}

	sections := make([]ModelInputSection, 0, len(plan.IncludedSections))
	for _, id := range plan.IncludedSections {
		candidate, exists := candidateByID[id]
		if !exists {
			return ComposedModelInput{}, fmt.Errorf("%w: unknown included section %q", ErrInvalidModelInput, id)
		}
		content, exists := contentByID[id]
		if !exists {
			return ComposedModelInput{}, fmt.Errorf("%w: missing authorized content for section %q", ErrInvalidModelInput, id)
		}
		section := metadata[id]
		sections = append(sections, ModelInputSection{
			SectionID: id, SectionVersion: section.Version,
			SourceOwner: candidate.SourceOwner,
			SourceRefs: append([]string{}, candidate.SourceRefs...),
			EvidenceRefs: append([]string{}, candidate.EvidenceRefs...),
			ProtectedEvidenceRefs: append([]string{}, candidate.ProtectedEvidenceRefs...),
			QualityStatus: section.QualityStatus, Required: candidate.Required,
			AssembledAt: section.AssembledAt.Format("2006-01-02T15:04:05.999999999Z"), Text: content,
		})
	}
	envelope := ModelInputEnvelope{
		SchemaVersion: "researchflow.context-input.v1",
		ResearchQuestion: snapshot.ResearchQuestion, Scope: snapshot.Scope,
		SubjectRefs: append([]string{}, snapshot.SubjectRefs...),
		SystemAsOf: snapshot.SystemAsOf.Format("2006-01-02T15:04:05.999999999Z"),
		BuilderVersion: snapshot.BuilderVersion, QualitySummary: command.QualitySummary,
		OmittedSections: command.OmittedSections, Sections: sections,
	}
	serialized, err := json.Marshal(envelope)
	if err != nil {
		return ComposedModelInput{}, fmt.Errorf("%w: serialize context input: %v", ErrInvalidModelInput, err)
	}
	tokenCount, err := tokenizer.CountTokens(string(serialized))
	if err != nil {
		return ComposedModelInput{}, fmt.Errorf("count composed model input: %w", err)
	}
	if tokenCount <= 0 {
		return ComposedModelInput{}, fmt.Errorf("%w: tokenizer returned non-positive count", ErrInvalidTokenEstimate)
	}
	if tokenCount > availableTokens {
		return ComposedModelInput{}, fmt.Errorf("%w: used %d tokens, available %d", ErrModelInputOverBudget, tokenCount, availableTokens)
	}
	return ComposedModelInput{
		Text: string(serialized), TokenCount: tokenCount,
		Tokenizer: TokenizerProvenance{ModelID: expectedModelID, Version: tokenizer.Version()},
	}, nil
}

