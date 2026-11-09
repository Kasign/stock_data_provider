package contextassembly

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrInvalidPlan             = errors.New("invalid context assembly plan")
	ErrMandatoryBudgetExceeded = errors.New("mandatory context sections exceed token budget")
)

const (
	OmissionBudgetExceeded = "budget_exceeded"
	ContextBudgetMessage   = "超出本次模型上下文预算"
)

// SectionCandidate is a Researchflow planning view, not a copy of Research's
// Context domain model. Token estimates are supplied by the model adapter or
// tokenizer selected for the target model.
type SectionCandidate struct {
	ID                    string
	SourceOwner           string
	Priority              int
	Required              bool
	EstimatedTokens       int
	SourceRefs            []string
	EvidenceRefs          []string
	ProtectedEvidenceRefs []string
}

type OmittedSection struct {
	SectionID   string
	SourceOwner string
	ReasonCode  string
	Required    bool
	SourceRefs  []string
	SafeMessage string
	Retryable   bool
}

type Plan struct {
	IncludedSections []string
	OmittedSections  []OmittedSection
	UsedTokens       int
	BudgetTokens     int
}

// BuildPlan deterministically selects complete sections. Required sections and
// sections carrying protected evidence are mandatory. It never silently drops
// protected evidence to satisfy a token budget.
func BuildPlan(candidates []SectionCandidate, budgetTokens int) (Plan, error) {
	if budgetTokens <= 0 {
		return Plan{}, fmt.Errorf("%w: token budget must be positive", ErrInvalidPlan)
	}
	ordered := make([]SectionCandidate, len(candidates))
	copy(ordered, candidates)
	seen := make(map[string]struct{}, len(ordered))
	for i := range ordered {
		candidate := &ordered[i]
		if strings.TrimSpace(candidate.ID) == "" || strings.TrimSpace(candidate.SourceOwner) == "" {
			return Plan{}, fmt.Errorf("%w: section id and source owner are required", ErrInvalidPlan)
		}
		if _, exists := seen[candidate.ID]; exists {
			return Plan{}, fmt.Errorf("%w: duplicate section %q", ErrInvalidPlan, candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
		if candidate.EstimatedTokens < 0 {
			return Plan{}, fmt.Errorf("%w: section %s has negative token estimate", ErrInvalidPlan, candidate.ID)
		}
		if err := validateUniqueRefs(candidate.SourceRefs, "source", candidate.ID); err != nil {
			return Plan{}, err
		}
		if err := validateUniqueRefs(candidate.EvidenceRefs, "evidence", candidate.ID); err != nil {
			return Plan{}, err
		}
		if err := validateUniqueRefs(candidate.ProtectedEvidenceRefs, "protected evidence", candidate.ID); err != nil {
			return Plan{}, err
		}
		evidence := make(map[string]struct{}, len(candidate.EvidenceRefs))
		for _, ref := range candidate.EvidenceRefs {
			evidence[ref] = struct{}{}
		}
		for _, ref := range candidate.ProtectedEvidenceRefs {
			if _, ok := evidence[ref]; !ok {
				return Plan{}, fmt.Errorf("%w: protected evidence %q is not in section %s", ErrInvalidPlan, ref, candidate.ID)
			}
		}
		candidate.SourceRefs = append([]string(nil), candidate.SourceRefs...)
		candidate.EvidenceRefs = append([]string(nil), candidate.EvidenceRefs...)
		candidate.ProtectedEvidenceRefs = append([]string(nil), candidate.ProtectedEvidenceRefs...)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority < ordered[j].Priority
		}
		return ordered[i].ID < ordered[j].ID
	})

	mandatory := make(map[string]bool, len(ordered))
	plan := Plan{IncludedSections: []string{}, OmittedSections: []OmittedSection{}, BudgetTokens: budgetTokens}
	for _, candidate := range ordered {
		isMandatory := candidate.Required || len(candidate.ProtectedEvidenceRefs) > 0
		mandatory[candidate.ID] = isMandatory
		if !isMandatory {
			continue
		}
		if candidate.EstimatedTokens > budgetTokens-plan.UsedTokens {
			return Plan{}, fmt.Errorf("%w: mandatory sections exceed budget %d", ErrMandatoryBudgetExceeded, budgetTokens)
		}
		plan.UsedTokens += candidate.EstimatedTokens
	}

	selected := make(map[string]bool, len(ordered))
	for id, required := range mandatory {
		selected[id] = required
	}
	for _, candidate := range ordered {
		if mandatory[candidate.ID] {
			continue
		}
		if candidate.EstimatedTokens <= budgetTokens-plan.UsedTokens {
			selected[candidate.ID] = true
			plan.UsedTokens += candidate.EstimatedTokens
		}
	}

	for _, candidate := range ordered {
		if selected[candidate.ID] {
			plan.IncludedSections = append(plan.IncludedSections, candidate.ID)
			continue
		}
		plan.OmittedSections = append(plan.OmittedSections, OmittedSection{
			SectionID: candidate.ID, SourceOwner: candidate.SourceOwner,
			ReasonCode: OmissionBudgetExceeded, Required: candidate.Required,
			SourceRefs: append([]string{}, candidate.SourceRefs...),
			SafeMessage: ContextBudgetMessage, Retryable: false,
		})
	}
	return plan, nil
}

func validateUniqueRefs(refs []string, kind, sectionID string) error {
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("%w: section %s has empty %s reference", ErrInvalidPlan, sectionID, kind)
		}
		if _, exists := seen[ref]; exists {
			return fmt.Errorf("%w: section %s has duplicate %s reference %q", ErrInvalidPlan, sectionID, kind, ref)
		}
		seen[ref] = struct{}{}
	}
	return nil
}
