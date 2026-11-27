package resultvalidation

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrRunResultNotPersistable is returned when a composed result lacks the
// minimum facts, provenance and non-authoritative explanation required for the
// Researchflow-owned RunResult resource.
var ErrRunResultNotPersistable = errors.New("Researchflow RunResult is not persistable")

// RunResultPayloadSchemaVersion is the immutable schema version of the
// persisted RunResult payload. It is separate from Run Schema versions and
// from the Quant/Research source resource versions.
const RunResultPayloadSchemaVersion = "1.0.0"

// PersistedLLMExplanation keeps the model explanation explicitly
// non-authoritative and records which response produced it.
type PersistedLLMExplanation struct {
	Text           string `json:"text"`
	ProviderID     string `json:"provider_id"`
	ModelRequested string `json:"model_requested"`
	ModelActual    string `json:"model_actual"`
	ModelVersion   string `json:"model_version"`
	ResponseID     string `json:"response_id"`
	Authority      string `json:"authority"`
}

// RunResultSourceRefs carries only identities of source resources. It never
// copies Quant Payload, Research Evidence content, Claims or Context bodies.
type RunResultSourceRefs struct {
	ContextID           string   `json:"context_id"`
	ContextVersion      int64    `json:"context_version"`
	ContextSourceRefs   []string `json:"context_source_refs"`
	ContextEvidenceRefs []string `json:"context_evidence_refs"`
	QuantResultRefs     []string `json:"quant_result_refs"`
	QuantSnapshotRefs   []string `json:"quant_snapshot_refs"`
	EvidenceRefs        []string `json:"evidence_refs"`
	ClaimRefs           []string `json:"claim_refs"`
}

// RunResultPayload is the only persistable projection of a complete report.
// RiskDecision is an authoritative projection from the Quant risk Envelope,
// not a copied Quant resource.
type RunResultPayload struct {
	SchemaVersion string                  `json:"schema_version"`
	RunRef        string                  `json:"run_ref"`
	SystemAsOf    string                  `json:"system_as_of"`
	Sources       RunResultSourceRefs     `json:"sources"`
	Explanation   PersistedLLMExplanation `json:"explanation"`
	RiskDecision  *RiskDecision           `json:"risk_decision,omitempty"`
	Limitations   []string                `json:"limitations"`
}

// RunResultFromComposed refuses to project empty or unbound reports. The
// trustedRunRef must come from the Run being finalized, never from caller input.
func RunResultFromComposed(composed ComposedResearchResult, trustedRunRef string) (RunResultPayload, error) {
	if strings.TrimSpace(trustedRunRef) == "" || trustedRunRef != strings.TrimSpace(trustedRunRef) ||
		strings.TrimSpace(composed.RunID) == "" || trustedRunRef != composed.RunID {
		return RunResultPayload{}, fmt.Errorf("%w: run_ref is missing or does not match the composed Run", ErrRunResultNotPersistable)
	}
	if composed.SystemAsOf.IsZero() || !isUTC(composed.SystemAsOf) {
		return RunResultPayload{}, fmt.Errorf("%w: system_as_of must be UTC", ErrRunResultNotPersistable)
	}
	if strings.TrimSpace(composed.Context.ContextID) == "" ||
		composed.Context.ContextID != strings.TrimSpace(composed.Context.ContextID) ||
		composed.Context.ContextVersion <= 0 {
		return RunResultPayload{}, fmt.Errorf("%w: Research Context identity is missing", ErrRunResultNotPersistable)
	}
	if len(composed.Evidence) == 0 || len(composed.Claims) == 0 || len(composed.QuantResults) == 0 {
		return RunResultPayload{}, fmt.Errorf("%w: complete report needs Evidence, Claims and Quant results", ErrRunResultNotPersistable)
	}
	if composed.RiskDecision == nil {
		return RunResultPayload{}, fmt.Errorf("%w: complete report needs a risk conclusion", ErrRunResultNotPersistable)
	}
	if err := ValidateRiskDecision(*composed.RiskDecision, composed.SystemAsOf); err != nil {
		return RunResultPayload{}, fmt.Errorf("%w: %v", ErrRunResultNotPersistable, err)
	}
	explanation, err := persistableExplanation(composed.Explanation)
	if err != nil {
		return RunResultPayload{}, err
	}

	contextSourceRefs := make([]string, 0)
	contextEvidenceRefs := make([]string, 0)
	for _, section := range composed.Context.SectionRefs {
		if !section.Included {
			continue
		}
		for _, ref := range section.SourceRefs {
			if err := validateRunResultRef(ref); err != nil {
				return RunResultPayload{}, err
			}
			contextSourceRefs = append(contextSourceRefs, ref)
		}
		for _, ref := range section.EvidenceRefs {
			if err := validateRunResultRef(ref); err != nil {
				return RunResultPayload{}, err
			}
			contextEvidenceRefs = append(contextEvidenceRefs, ref)
		}
	}

	quantResultRefs := make([]string, 0, len(composed.QuantResults))
	quantSnapshotRefs := make([]string, 0)
	hasUsableQuant := false
	for _, quantResult := range composed.QuantResults {
		if err := validateRunResultRef(quantResult.Result.ResultID); err != nil {
			return RunResultPayload{}, err
		}
		quantResultRefs = append(quantResultRefs, quantResult.Result.ResultID)
		for _, snapshotRef := range quantResult.Result.InputSnapshotRefs {
			if err := validateRunResultRef(snapshotRef); err != nil {
				return RunResultPayload{}, err
			}
			quantSnapshotRefs = append(quantSnapshotRefs, snapshotRef)
		}
		if quantResult.Usable {
			hasUsableQuant = true
		}
	}
	if !hasUsableQuant {
		return RunResultPayload{}, fmt.Errorf("%w: complete report needs at least one usable Quant result", ErrRunResultNotPersistable)
	}

	evidenceRefs := make([]string, 0, len(composed.Evidence))
	for _, evidence := range composed.Evidence {
		if err := validateRunResultRef(evidence.EvidenceID); err != nil {
			return RunResultPayload{}, err
		}
		evidenceRefs = append(evidenceRefs, evidence.EvidenceID)
	}
	claimRefs := make([]string, 0, len(composed.Claims))
	for _, claim := range composed.Claims {
		if err := validateRunResultRef(claim.ClaimID); err != nil {
			return RunResultPayload{}, err
		}
		claimRefs = append(claimRefs, claim.ClaimID)
	}
	limitations, err := cloneJSON(composed.Limitations)
	if err != nil {
		return RunResultPayload{}, fmt.Errorf("%w: copy limitations: %v", ErrRunResultNotPersistable, err)
	}
	riskDecision, err := cloneJSON(*composed.RiskDecision)
	if err != nil {
		return RunResultPayload{}, fmt.Errorf("%w: copy risk decision: %v", ErrRunResultNotPersistable, err)
	}

	payload := RunResultPayload{
		SchemaVersion: RunResultPayloadSchemaVersion,
		RunRef:        trustedRunRef,
		SystemAsOf:    composed.SystemAsOf.UTC().Format(time.RFC3339Nano),
		Sources: RunResultSourceRefs{
			ContextID:           composed.Context.ContextID,
			ContextVersion:      composed.Context.ContextVersion,
			ContextSourceRefs:   sortedUniqueRunResultRefs(contextSourceRefs),
			ContextEvidenceRefs: sortedUniqueRunResultRefs(contextEvidenceRefs),
			QuantResultRefs:     sortedUniqueRunResultRefs(quantResultRefs),
			QuantSnapshotRefs:   sortedUniqueRunResultRefs(quantSnapshotRefs),
			EvidenceRefs:        sortedUniqueRunResultRefs(evidenceRefs),
			ClaimRefs:           sortedUniqueRunResultRefs(claimRefs),
		},
		Explanation:  explanation,
		RiskDecision: &riskDecision,
		Limitations:  limitations,
	}
	isolated, err := cloneJSON(payload)
	if err != nil {
		return RunResultPayload{}, fmt.Errorf("%w: copy RunResult payload: %v", ErrRunResultNotPersistable, err)
	}
	return isolated, nil
}

func persistableExplanation(value *LLMExplanation) (PersistedLLMExplanation, error) {
	if value == nil || strings.TrimSpace(value.Text) == "" ||
		value.Authority != LLMExplanationAuthority {
		return PersistedLLMExplanation{}, fmt.Errorf("%w: non-authoritative explanation is missing", ErrRunResultNotPersistable)
	}
	for name, field := range map[string]string{
		"provider_id":     value.ProviderID,
		"model_requested": value.ModelRequested,
		"model_actual":    value.ModelActual,
		"model_version":   value.ModelVersion,
		"response_id":     value.ResponseID,
	} {
		if err := validateRunResultNamedRef(name, field); err != nil {
			return PersistedLLMExplanation{}, err
		}
	}
	return PersistedLLMExplanation{
		Text:           value.Text,
		ProviderID:     value.ProviderID,
		ModelRequested: value.ModelRequested,
		ModelActual:    value.ModelActual,
		ModelVersion:   value.ModelVersion,
		ResponseID:     value.ResponseID,
		Authority:      value.Authority,
	}, nil
}

func validateRunResultRef(value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%w: source ref is invalid", ErrRunResultNotPersistable)
	}
	return nil
}

func validateRunResultNamedRef(name, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%w: %s is invalid", ErrRunResultNotPersistable, name)
	}
	return nil
}

func sortedUniqueRunResultRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
