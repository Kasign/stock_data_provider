package contextassembly

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

var ErrInvalidResearchContextProjection = errors.New("invalid Research Context projection")

// SectionMetadata carries Research-owned semantic metadata that the planner
// must not invent or infer from token-budget decisions.
type SectionMetadata struct {
	Version      string
	QualityStatus string
	AssembledAt  time.Time
}

// ResearchContextSnapshot contains the semantic fields assessed by Research.
// Quality status, conflict/stale counts and reason codes are passed through;
// Researchflow only reconciles omission counts with its own selection plan.
type ResearchContextSnapshot struct {
	ResearchQuestion string
	Scope            string
	SubjectRefs      []string
	SystemAsOf       time.Time
	BuilderVersion   string
	QualitySummary   researchapp.ContextQualitySummary
}

// ToResearchCreateContextCommand maps a completed selection plan to the
// stable Research Application DTO. It does not create a Context, authorize
// evidence, or fetch section content.
func ToResearchCreateContextCommand(
	snapshot ResearchContextSnapshot,
	candidates []SectionCandidate,
	metadata map[string]SectionMetadata,
	plan Plan,
) (researchapp.CreateContextCommand, error) {
	if strings.TrimSpace(snapshot.ResearchQuestion) == "" ||
		snapshot.ResearchQuestion != strings.TrimSpace(snapshot.ResearchQuestion) ||
		strings.TrimSpace(snapshot.Scope) == "" || snapshot.Scope != strings.TrimSpace(snapshot.Scope) ||
		snapshot.BuilderVersion == "" || snapshot.BuilderVersion != strings.TrimSpace(snapshot.BuilderVersion) ||
		snapshot.SystemAsOf.IsZero() || snapshot.SystemAsOf.Location() != time.UTC ||
		snapshot.SubjectRefs == nil || snapshot.QualitySummary.ReasonCodes == nil {
		return researchapp.CreateContextCommand{}, ErrInvalidResearchContextProjection
	}
	if plan.BudgetTokens <= 0 || plan.UsedTokens < 0 || plan.UsedTokens > plan.BudgetTokens ||
		plan.IncludedSections == nil || plan.OmittedSections == nil || len(candidates) == 0 {
		return researchapp.CreateContextCommand{}, ErrInvalidResearchContextProjection
	}
	if snapshot.QualitySummary.MissingRequiredCount < 0 ||
		snapshot.QualitySummary.MissingOptionalCount < 0 ||
		snapshot.QualitySummary.ConflictCount < 0 || snapshot.QualitySummary.StaleCount < 0 ||
		!sort.StringsAreSorted(snapshot.QualitySummary.ReasonCodes) ||
		hasDuplicates(snapshot.QualitySummary.ReasonCodes) {
		return researchapp.CreateContextCommand{}, ErrInvalidResearchContextProjection
	}

	if len(metadata) != len(candidates) {
		return researchapp.CreateContextCommand{}, fmt.Errorf("%w: section metadata does not match candidates", ErrInvalidResearchContextProjection)
	}
	candidateByID := make(map[string]SectionCandidate, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == "" || candidate.SourceOwner == "" {
			return researchapp.CreateContextCommand{}, ErrInvalidResearchContextProjection
		}
		if _, exists := candidateByID[candidate.ID]; exists {
			return researchapp.CreateContextCommand{}, fmt.Errorf("%w: duplicate candidate %q", ErrInvalidResearchContextProjection, candidate.ID)
		}
		candidateByID[candidate.ID] = candidate
	}
	included := make(map[string]struct{}, len(plan.IncludedSections))
	for _, id := range plan.IncludedSections {
		if _, exists := candidateByID[id]; !exists {
			return researchapp.CreateContextCommand{}, fmt.Errorf("%w: unknown included section %q", ErrInvalidResearchContextProjection, id)
		}
		if _, exists := included[id]; exists {
			return researchapp.CreateContextCommand{}, fmt.Errorf("%w: duplicate included section %q", ErrInvalidResearchContextProjection, id)
		}
		included[id] = struct{}{}
	}
	omitted := make(map[string]OmittedSection, len(plan.OmittedSections))
	for _, value := range plan.OmittedSections {
		candidate, exists := candidateByID[value.SectionID]
		if !exists {
			return researchapp.CreateContextCommand{}, fmt.Errorf("%w: unknown omitted section %q", ErrInvalidResearchContextProjection, value.SectionID)
		}
		if _, exists := omitted[value.SectionID]; exists {
			return researchapp.CreateContextCommand{}, fmt.Errorf("%w: duplicate omitted section %q", ErrInvalidResearchContextProjection, value.SectionID)
		}
		if _, selected := included[value.SectionID]; selected ||
			value.SourceOwner != candidate.SourceOwner || value.Required != candidate.Required ||
			!slices.Equal(value.SourceRefs, candidate.SourceRefs) ||
			value.ReasonCode != OmissionBudgetExceeded || value.SafeMessage == "" {
			return researchapp.CreateContextCommand{}, fmt.Errorf("%w: omitted section %q does not match candidate", ErrInvalidResearchContextProjection, value.SectionID)
		}
		omitted[value.SectionID] = value
	}
	if len(included)+len(omitted) != len(candidateByID) {
		return researchapp.CreateContextCommand{}, fmt.Errorf("%w: plan does not partition all candidates", ErrInvalidResearchContextProjection)
	}
	var accountedTokens int
	for id := range included {
		accountedTokens += candidateByID[id].EstimatedTokens
	}
	if accountedTokens != plan.UsedTokens {
		return researchapp.CreateContextCommand{}, fmt.Errorf("%w: used token count does not match included sections", ErrInvalidResearchContextProjection)
	}
	for id, value := range omitted {
		candidate := candidateByID[id]
		if candidate.Required || len(candidate.ProtectedEvidenceRefs) > 0 ||
			value.Retryable || value.SafeMessage != ContextBudgetMessage {
			return researchapp.CreateContextCommand{}, fmt.Errorf("%w: mandatory or protected section %q cannot be omitted", ErrInvalidResearchContextProjection, id)
		}
	}

	sectionRefs := make([]researchapp.ContextSectionRef, 0, len(candidates))
	missingRequired, missingOptional := 0, 0
	for _, candidate := range candidates {
		section, exists := metadata[candidate.ID]
		if !exists || section.Version == "" || section.Version != strings.TrimSpace(section.Version) ||
			!validQualityStatus(section.QualityStatus) || section.AssembledAt.IsZero() ||
			section.AssembledAt.Location() != time.UTC {
			return researchapp.CreateContextCommand{}, fmt.Errorf("%w: missing or invalid metadata for section %q", ErrInvalidResearchContextProjection, candidate.ID)
		}
		_, selected := included[candidate.ID]
		sectionRefs = append(sectionRefs, researchapp.ContextSectionRef{
			SectionID: candidate.ID, SectionVersion: section.Version,
			SourceOwner: candidate.SourceOwner,
			SourceRefs: append([]string{}, candidate.SourceRefs...),
			EvidenceRefs: append([]string{}, candidate.EvidenceRefs...),
			QualityStatus: section.QualityStatus, Required: candidate.Required,
			Included: selected, AssembledAt: section.AssembledAt, SystemAsOf: snapshot.SystemAsOf,
		})
		if !selected {
			if candidate.Required {
				missingRequired++
			} else {
				missingOptional++
			}
		}
	}
	if snapshot.QualitySummary.MissingRequiredCount != missingRequired ||
		snapshot.QualitySummary.MissingOptionalCount != missingOptional ||
		(missingRequired > 0 && snapshot.QualitySummary.Status == "valid") {
		return researchapp.CreateContextCommand{}, fmt.Errorf("%w: quality summary omission counts do not match plan", ErrInvalidResearchContextProjection)
	}

	includedIDs := append([]string{}, plan.IncludedSections...)
	omittedSections := make([]researchapp.ContextOmittedSection, 0, len(plan.OmittedSections))
	for _, value := range plan.OmittedSections {
		omittedSections = append(omittedSections, researchapp.ContextOmittedSection{
			SectionID: value.SectionID, SourceOwner: value.SourceOwner,
			ReasonCode: value.ReasonCode, Required: value.Required,
			SourceRefs: append([]string{}, value.SourceRefs...),
			SafeMessage: value.SafeMessage, Retryable: value.Retryable,
		})
	}
	quality := snapshot.QualitySummary
	quality.ReasonCodes = append([]string{}, quality.ReasonCodes...)
	return researchapp.CreateContextCommand{
		ResearchQuestion: snapshot.ResearchQuestion, Scope: snapshot.Scope,
		SubjectRefs: append([]string{}, snapshot.SubjectRefs...),
		SystemAsOf: snapshot.SystemAsOf, SectionRefs: sectionRefs,
		QualitySummary: quality, IncludedSections: includedIDs,
		OmittedSections: omittedSections, BuilderVersion: snapshot.BuilderVersion,
	}, nil
}

func validQualityStatus(value string) bool {
	switch value {
	case "valid", "degraded", "invalid", "unknown":
		return true
	default:
		return false
	}
}

func hasDuplicates(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return true
		}
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}


// ResearchContextAssembly is the deterministic output prepared for the
// Research Application Port. AvailableTokens is the post-reservation input
// budget; it is not a provider-reported usage value.
type ResearchContextAssembly struct {
	Command         researchapp.CreateContextCommand
	Plan            Plan
	AvailableTokens int
}

// AssembleResearchContext composes budget reservation, deterministic section
// selection and the Research Application DTO projection. Section token
// estimates must already have been produced by the selected model tokenizer.
func AssembleResearchContext(
	snapshot ResearchContextSnapshot,
	candidates []SectionCandidate,
	metadata map[string]SectionMetadata,
	budgetInput BudgetInput,
) (ResearchContextAssembly, error) {
	available, err := AvailableBudget(budgetInput)
	if err != nil {
		return ResearchContextAssembly{}, err
	}
	plan, err := BuildPlan(candidates, available)
	if err != nil {
		return ResearchContextAssembly{}, err
	}
	command, err := ToResearchCreateContextCommand(snapshot, candidates, metadata, plan)
	if err != nil {
		return ResearchContextAssembly{}, err
	}
	return ResearchContextAssembly{Command: command, Plan: plan, AvailableTokens: available}, nil
}
