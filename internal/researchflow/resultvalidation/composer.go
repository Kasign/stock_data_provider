package resultvalidation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	llmapp "example.com/stock-analysis-flow/internal/llm"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

var (
	ErrInvalidResultComposition = errors.New("invalid Researchflow result composition")
	ErrInvalidLLMExplanation    = errors.New("invalid non-authoritative LLM explanation")
)


const LLMExplanationAuthority = "non_authoritative"

// ResultCompositionInput keeps facts, evidence, risk and model explanation in
// separate typed inputs. No LLM field can replace a Quant or Research field.
type ResultCompositionInput struct {
	RunID        string
	SystemAsOf   time.Time
	Context      researchapp.ResearchContext
	QuantResults []QuantResultEnvelope
	Evidence     []researchapp.EvidenceRecord
	Claims       []researchapp.Claim
	LLMResponse  *llmapp.ModelResponse
	RiskDecision *RiskDecision
	Limitations  []string
	QuantPolicy  ValidationPolicy
}

// LLMExplanation is explicitly non-authoritative and never carries Quant
// values, Research evidence state or RiskDecision fields.
type LLMExplanation struct {
	Text           string
	ProviderID     string
	ModelRequested string
	ModelActual    string
	ModelVersion   string
	ResponseID     string
	Authority      string
}

// ComposedResearchResult preserves source facts in independent fields. It
// deliberately does not calculate a score or aggregate an actionable status.
type ComposedResearchResult struct {
	RunID        string
	SystemAsOf   time.Time
	Context      researchapp.ResearchContext
	QuantResults []QuantValidation
	Evidence     []researchapp.EvidenceRecord
	Claims       []researchapp.Claim
	Explanation  *LLMExplanation
	RiskDecision *RiskDecision
	Limitations  []string
}

// ComposeResult validates each source layer and returns a separated result.
// The caller must fetch Research/Quant data through their public Facades and
// complete current Owner authorization before passing protected content.
func ComposeResult(input ResultCompositionInput, quantRegistry *Registry) (ComposedResearchResult, error) {
	if strings.TrimSpace(input.RunID) == "" || input.SystemAsOf.IsZero() || !isUTC(input.SystemAsOf) {
		return ComposedResearchResult{}, ErrInvalidResultComposition
	}
	if err := validateResearchContext(input.Context, input.SystemAsOf); err != nil {
		return ComposedResearchResult{}, err
	}
	evidenceIDs, err := validateResearchEvidence(input.Evidence, input.SystemAsOf)
	if err != nil {
		return ComposedResearchResult{}, err
	}
	contextSourceRefs, err := validateContextReferences(input.Context, evidenceIDs)
	if err != nil {
		return ComposedResearchResult{}, err
	}
	claimIDs, err := validateResearchClaims(input.Claims, evidenceIDs, input.SystemAsOf)
	if err != nil {
		return ComposedResearchResult{}, err
	}
	for _, evidence := range input.Evidence {
		if evidence.ClaimRef != nil {
			if _, exists := claimIDs[*evidence.ClaimRef]; !exists {
				return ComposedResearchResult{}, fmt.Errorf("%w: Evidence %q references missing Claim %q", ErrInvalidResultComposition, evidence.EvidenceID, *evidence.ClaimRef)
			}
		}
	}

	quantResults := make([]QuantValidation, 0, len(input.QuantResults))
	seenQuant := make(map[string]struct{}, len(input.QuantResults))
	for _, quantResult := range input.QuantResults {
		if quantRegistry == nil {
			return ComposedResearchResult{}, ErrPayloadValidatorUnavailable
		}
		validationPolicy := input.QuantPolicy
		validationPolicy.ExpectedSystemAsOf = input.SystemAsOf
		validated, err := quantRegistry.Validate(quantResult, validationPolicy)
		if err != nil {
			return ComposedResearchResult{}, fmt.Errorf("%w: Quant result: %v", ErrInvalidResultComposition, err)
		}
		if _, exists := seenQuant[validated.Result.ResultID]; exists {
			return ComposedResearchResult{}, fmt.Errorf("%w: duplicate Quant result %q", ErrInvalidResultComposition, validated.Result.ResultID)
		}
		seenQuant[validated.Result.ResultID] = struct{}{}
		quantResults = append(quantResults, validated)
	}

	var explanation *LLMExplanation
	if input.LLMResponse != nil {
		value, err := validateLLMExplanation(*input.LLMResponse, input.SystemAsOf)
		if err != nil {
			return ComposedResearchResult{}, err
		}
		explanation = &value
	}

	var riskDecision *RiskDecision
	if input.RiskDecision != nil {
		if err := ValidateRiskDecision(*input.RiskDecision, input.SystemAsOf); err != nil {
			return ComposedResearchResult{}, fmt.Errorf("%w: RiskDecision: %v", ErrInvalidResultComposition, err)
		}
		quantRefs := make(map[string]struct{}, len(quantResults))
		for _, quantResult := range quantResults {
			quantRefs[quantResult.Result.ResultID] = struct{}{}
		}
		if err := validateRiskReferences(*input.RiskDecision, quantRefs, contextSourceRefs, evidenceIDs); err != nil {
			return ComposedResearchResult{}, err
		}
		copyValue, err := cloneJSON(*input.RiskDecision)
		if err != nil {
			return ComposedResearchResult{}, fmt.Errorf("%w: copy RiskDecision: %v", ErrInvalidResultComposition, err)
		}
		riskDecision = &copyValue
	}

	contextCopy, err := cloneJSON(input.Context)
	if err != nil {
		return ComposedResearchResult{}, fmt.Errorf("%w: copy Research Context: %v", ErrInvalidResultComposition, err)
	}
	evidenceCopy, err := cloneJSON(input.Evidence)
	if err != nil {
		return ComposedResearchResult{}, fmt.Errorf("%w: copy Research Evidence: %v", ErrInvalidResultComposition, err)
	}
	claimsCopy, err := cloneJSON(input.Claims)
	if err != nil {
		return ComposedResearchResult{}, fmt.Errorf("%w: copy Research Claims: %v", ErrInvalidResultComposition, err)
	}
	limitations := append([]string{}, input.Limitations...)
	for _, limitation := range limitations {
		if strings.TrimSpace(limitation) == "" || limitation != strings.TrimSpace(limitation) {
			return ComposedResearchResult{}, fmt.Errorf("%w: invalid limitation", ErrInvalidResultComposition)
		}
	}
	return ComposedResearchResult{
		RunID: input.RunID, SystemAsOf: input.SystemAsOf,
		Context: contextCopy, QuantResults: quantResults,
		Evidence: evidenceCopy, Claims: claimsCopy,
		Explanation: explanation, RiskDecision: riskDecision,
		Limitations: limitations,
	}, nil
}


func validateContextReferences(value researchapp.ResearchContext, evidenceIDs map[string]struct{}) (map[string]struct{}, error) {
	included := make(map[string]struct{}, len(value.IncludedSections))
	omitted := make(map[string]struct{}, len(value.OmittedSections))
	sourceRefs := make(map[string]struct{})
	sectionIDs := make(map[string]struct{}, len(value.SectionRefs))
	for _, sectionID := range value.IncludedSections {
		if strings.TrimSpace(sectionID) == "" || sectionID != strings.TrimSpace(sectionID) {
			return nil, fmt.Errorf("%w: invalid included section id", ErrInvalidResultComposition)
		}
		if _, exists := included[sectionID]; exists {
			return nil, fmt.Errorf("%w: duplicate included section %q", ErrInvalidResultComposition, sectionID)
		}
		included[sectionID] = struct{}{}
	}
	for _, section := range value.SectionRefs {
		if _, exists := sectionIDs[section.SectionID]; exists {
			return nil, fmt.Errorf("%w: duplicate Context section %q", ErrInvalidResultComposition, section.SectionID)
		}
		sectionIDs[section.SectionID] = struct{}{}
		if section.Included {
			if _, exists := included[section.SectionID]; !exists {
				return nil, fmt.Errorf("%w: included section %q missing from included_sections", ErrInvalidResultComposition, section.SectionID)
			}
		} else if _, exists := included[section.SectionID]; exists {
			return nil, fmt.Errorf("%w: excluded section %q listed as included", ErrInvalidResultComposition, section.SectionID)
		}
		for _, ref := range section.SourceRefs {
			if _, exists := sourceRefs[ref]; exists {
				// A source may legitimately support more than one section.
				continue
			}
			sourceRefs[ref] = struct{}{}
		}
		for _, ref := range section.EvidenceRefs {
			if _, exists := evidenceIDs[ref]; !exists {
				return nil, fmt.Errorf("%w: Context section %q references missing Evidence %q", ErrInvalidResultComposition, section.SectionID, ref)
			}
		}
	}
	for sectionID := range included {
		if _, exists := sectionIDs[sectionID]; !exists {
			return nil, fmt.Errorf("%w: included section %q has no section reference", ErrInvalidResultComposition, sectionID)
		}
	}
	for _, section := range value.OmittedSections {
		if strings.TrimSpace(section.SectionID) == "" || !validSourceOwner(section.SourceOwner) ||
			strings.TrimSpace(section.SafeMessage) == "" || section.SourceRefs == nil ||
			hasDuplicateOrInvalidRefs(section.SourceRefs) {
			return nil, fmt.Errorf("%w: invalid omitted section %q", ErrInvalidResultComposition, section.SectionID)
		}
		switch section.ReasonCode {
		case "not_requested", "budget_exceeded", "unavailable", "unauthorized", "quality_rejected", "failed", "duplicate":
		default:
			return nil, fmt.Errorf("%w: unsupported omitted-section reason %q", ErrInvalidResultComposition, section.ReasonCode)
		}
		if _, exists := omitted[section.SectionID]; exists {
			return nil, fmt.Errorf("%w: duplicate omitted section %q", ErrInvalidResultComposition, section.SectionID)
		}
		if _, exists := included[section.SectionID]; exists {
			return nil, fmt.Errorf("%w: section %q is both included and omitted", ErrInvalidResultComposition, section.SectionID)
		}
		omitted[section.SectionID] = struct{}{}
	}
	return sourceRefs, nil
}

func validateRiskReferences(value RiskDecision, quantRefs, sourceRefs, evidenceIDs map[string]struct{}) error {
	for _, ref := range value.InputRefs {
		if _, quantExists := quantRefs[ref]; quantExists {
			continue
		}
		if _, sourceExists := sourceRefs[ref]; sourceExists {
			continue
		}
		return fmt.Errorf("%w: RiskDecision references unknown input %q", ErrInvalidResultComposition, ref)
	}
	for _, reason := range value.Reasons {
		for _, ref := range reason.EvidenceRefs {
			if _, exists := evidenceIDs[ref]; !exists {
				return fmt.Errorf("%w: RiskDecision reason %q references missing Evidence %q", ErrInvalidResultComposition, reason.Code, ref)
			}
		}
	}
	return nil
}

func validateResearchContext(value researchapp.ResearchContext, expectedSystemAsOf time.Time) error {
	if strings.TrimSpace(value.ContextID) == "" || value.ContextVersion <= 0 ||
		strings.TrimSpace(value.ResearchQuestion) == "" || strings.TrimSpace(value.BuilderVersion) == "" ||
		!isUTC(value.SystemAsOf) || !value.SystemAsOf.Equal(expectedSystemAsOf) ||
		value.SectionRefs == nil || value.IncludedSections == nil || value.OmittedSections == nil ||
		value.QualitySummary.ReasonCodes == nil || strings.TrimSpace(value.QualitySummary.Status) == "" ||
		value.QualitySummary.MissingRequiredCount < 0 || value.QualitySummary.MissingOptionalCount < 0 ||
		value.QualitySummary.ConflictCount < 0 || value.QualitySummary.StaleCount < 0 {
		return fmt.Errorf("%w: Research Context is invalid or belongs to another system_as_of", ErrInvalidResultComposition)
	}
	if !isUTC(value.CreatedAt) {
		return fmt.Errorf("%w: Research Context created_at must be UTC", ErrInvalidResultComposition)
	}
	for _, section := range value.SectionRefs {
		if strings.TrimSpace(section.SectionID) == "" || strings.TrimSpace(section.SectionVersion) == "" ||
			strings.TrimSpace(section.SourceOwner) == "" || section.SourceRefs == nil || section.EvidenceRefs == nil ||
			strings.TrimSpace(section.QualityStatus) == "" || !isUTC(section.AssembledAt) ||
			!isUTC(section.SystemAsOf) || !section.SystemAsOf.Equal(expectedSystemAsOf) {
			return fmt.Errorf("%w: invalid Research Context section %q", ErrInvalidResultComposition, section.SectionID)
		}
	}
	return nil
}

func validateResearchEvidence(values []researchapp.EvidenceRecord, expectedSystemAsOf time.Time) (map[string]struct{}, error) {
	ids := make(map[string]struct{}, len(values))
	for _, value := range values {
		draft := value.EvidenceDraft
		if strings.TrimSpace(value.EvidenceID) == "" || strings.TrimSpace(value.ContentFingerprint) == "" ||
			strings.TrimSpace(draft.SourceRef) == "" || draft.SourceRef != strings.TrimSpace(draft.SourceRef) ||
			draft.DerivedFromRefs == nil || draft.SubjectRefs == nil || draft.Limitations == nil ||
			!isUTC(draft.SystemObservedAt) || !isUTC(draft.SystemAsOf) ||
			draft.SystemAsOf.After(expectedSystemAsOf) || draft.SystemObservedAt.After(draft.SystemAsOf) ||
			strings.TrimSpace(draft.AuthorizationRef) == "" ||
			hasDuplicateOrInvalidRefs(draft.DerivedFromRefs) || hasDuplicateOrInvalidRefs(draft.SubjectRefs) ||
			hasDuplicateOrInvalidRefs(draft.Limitations) ||
			(draft.ClaimRef != nil && (strings.TrimSpace(*draft.ClaimRef) == "" || *draft.ClaimRef != strings.TrimSpace(*draft.ClaimRef))) {
			return nil, fmt.Errorf("%w: invalid Research Evidence %q", ErrInvalidResultComposition, value.EvidenceID)
		}
		if _, exists := ids[value.EvidenceID]; exists {
			return nil, fmt.Errorf("%w: duplicate Research Evidence %q", ErrInvalidResultComposition, value.EvidenceID)
		}
		ids[value.EvidenceID] = struct{}{}
	}
	for _, value := range values {
		for _, ref := range value.DerivedFromRefs {
			if _, exists := ids[ref]; !exists || ref == value.EvidenceID {
				return nil, fmt.Errorf("%w: Evidence %q references missing or self Evidence %q", ErrInvalidResultComposition, value.EvidenceID, ref)
			}
		}
	}
	if evidenceGraphHasCycle(values) {
		return nil, fmt.Errorf("%w: Research Evidence derivation contains a cycle", ErrInvalidResultComposition)
	}
	return ids, nil
}

func validateResearchClaims(values []researchapp.Claim, evidenceIDs map[string]struct{}, expectedSystemAsOf time.Time) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(values))
	claimIDs := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.ClaimID) == "" || value.ClaimVersion <= 0 ||
			strings.TrimSpace(value.Statement) == "" ||
			!isUTC(value.SystemAsOf) || !value.SystemAsOf.Equal(expectedSystemAsOf) ||
			value.ScopeRefs == nil || value.SupportingEvidenceRefs == nil ||
			value.ContradictingEvidenceRefs == nil || value.RequiredDataRefs == nil ||
			value.Limitations == nil ||
			hasDuplicateOrInvalidRefs(value.ScopeRefs) || hasDuplicateOrInvalidRefs(value.SupportingEvidenceRefs) ||
			hasDuplicateOrInvalidRefs(value.ContradictingEvidenceRefs) || hasDuplicateOrInvalidRefs(value.RequiredDataRefs) ||
			hasDuplicateOrInvalidRefs(value.Limitations) {
			return nil, fmt.Errorf("%w: invalid Research Claim %q", ErrInvalidResultComposition, value.ClaimID)
		}
		key := fmt.Sprintf("%s/%d", value.ClaimID, value.ClaimVersion)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("%w: duplicate Research Claim %q", ErrInvalidResultComposition, key)
		}
		seen[key] = struct{}{}
		claimIDs[value.ClaimID] = struct{}{}
		referenced := make(map[string]struct{}, len(value.SupportingEvidenceRefs)+len(value.ContradictingEvidenceRefs))
		for _, ref := range value.SupportingEvidenceRefs {
			if _, exists := evidenceIDs[ref]; !exists || ref == "" {
				return nil, fmt.Errorf("%w: Claim %q references missing supporting Evidence %q", ErrInvalidResultComposition, key, ref)
			}
			referenced[ref] = struct{}{}
		}
		for _, ref := range value.ContradictingEvidenceRefs {
			if _, exists := evidenceIDs[ref]; !exists || ref == "" {
				return nil, fmt.Errorf("%w: Claim %q references missing contradicting Evidence %q", ErrInvalidResultComposition, key, ref)
			}
			if _, overlaps := referenced[ref]; overlaps {
				return nil, fmt.Errorf("%w: Claim %q uses one Evidence as both support and contradiction", ErrInvalidResultComposition, key)
			}
		}
		if value.EpistemicType == "hypothesis" &&
			(value.InvalidationConditions == nil || strings.TrimSpace(pointerValue(value.ValidationMethod)) == "" ||
				strings.TrimSpace(pointerValue(value.ExpectedObservation)) == "" ||
				len(value.InvalidationConditions) == 0) {
			return nil, fmt.Errorf("%w: Hypothesis %q lacks validation or invalidation fields", ErrInvalidResultComposition, key)
		}
	}
	return claimIDs, nil
}

func validateLLMExplanation(value llmapp.ModelResponse, expectedSystemAsOf time.Time) (LLMExplanation, error) {
	if value.Status != llmapp.ResponseCompleted || strings.TrimSpace(value.OutputText) == "" ||
		strings.TrimSpace(value.ProviderID) == "" || strings.TrimSpace(value.ModelRequested) == "" ||
		!isUTC(value.SystemAsOf) || !value.SystemAsOf.Equal(expectedSystemAsOf) ||
		value.Error != nil || len(value.ToolCalls) > 0 {
		return LLMExplanation{}, ErrInvalidLLMExplanation
	}
	for _, item := range value.OutputItems {
		if item.Type == llmapp.OutputItemFunctionCall || item.Type == llmapp.OutputItemRefusal {
			return LLMExplanation{}, ErrInvalidLLMExplanation
		}
	}
	return LLMExplanation{
		Text: value.OutputText, ProviderID: value.ProviderID,
		ModelRequested: value.ModelRequested, ModelActual: value.ModelActual,
		ModelVersion: value.ModelVersion, ResponseID: value.ResponseID,
		Authority: LLMExplanationAuthority,
	}, nil
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cloneJSON[T any](value T) (T, error) {
	var cloned T
	encoded, err := json.Marshal(value)
	if err != nil {
		return cloned, err
	}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return cloned, err
	}
	return cloned, nil
}
