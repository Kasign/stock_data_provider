package context

import (
    "fmt"
    "math"
    "slices"
    "strings"
    "time"
)

var schemaVersionPattern = regexp.MustCompile(\`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$\`)

// Validate checks the ResearchContext DTO invariants. It does not fetch or
// authorize referenced resources; those checks belong to source-owner adapters.
func (c Context) Validate() error {
    if c.SchemaVersion != SchemaVersion {
        return fmt.Errorf("schema_version must equal the supported version %q", SchemaVersion)
    }
    if c.ContextID == "" || c.ContextID != strings.TrimSpace(c.ContextID) {
        return fmt.Errorf("context_id is required without surrounding whitespace")
    }
    if c.ContextVersion < 1 {
        return fmt.Errorf("context_version must be greater than zero")
    }
    if strings.TrimSpace(c.ResearchQuestion) == "" {
        return fmt.Errorf("research_question is required")
    }
    if err := validateScope(c.Scope); err != nil {
        return err
    }
    if c.SubjectRefs == nil {
        return fmt.Errorf("subject_refs must be a non-null array")
    }
    if err := validateUniqueStrings("subject_refs", c.SubjectRefs); err != nil {
        return err
    }
    if c.SystemAsOf.IsZero() || !isUTC(c.SystemAsOf) {
        return fmt.Errorf("system_as_of is required and must use UTC")
    }
    if c.CreatedAt.IsZero() || !isUTC(c.CreatedAt) {
        return fmt.Errorf("created_at is required and must use UTC")
    }
    if c.BuilderVersion == "" || c.BuilderVersion != strings.TrimSpace(c.BuilderVersion) {
        return fmt.Errorf("builder_version is required without surrounding whitespace")
    }
    if c.SectionRefs == nil || len(c.SectionRefs) == 0 {
        return fmt.Errorf("section_refs must contain at least one assessed section")
    }
    if c.IncludedSections == nil || c.OmittedSections == nil {
        return fmt.Errorf("included_sections and omitted_sections must be non-null arrays")
    }
    if err := validateQualitySummary(c.QualitySummary); err != nil {
        return err
    }

    sections := make(map[string]SectionRef, len(c.SectionRefs))
    included := make(map[string]struct{}, len(c.SectionRefs))
    omitted := make(map[string]struct{}, len(c.SectionRefs))
    missingRequired, missingOptional := 0, 0
    for _, section := range c.SectionRefs {
        if section.SectionID == "" || section.SectionID != strings.TrimSpace(section.SectionID) ||
            section.SectionVersion == "" || section.SectionVersion != strings.TrimSpace(section.SectionVersion) {
            return fmt.Errorf("section_id and section_version are required without surrounding whitespace")
        }
        if _, exists := sections[section.SectionID]; exists {
            return fmt.Errorf("section_refs contains duplicate section_id %q", section.SectionID)
        }
        if err := validateSourceOwner(section.SourceOwner); err != nil {
            return err
        }
        if section.SourceRefs == nil || section.EvidenceRefs == nil {
            return fmt.Errorf("section %q source_refs and evidence_refs must be non-null arrays", section.SectionID)
        }
        if section.SourceOwner != SourceOwnerResearch && len(section.SourceRefs) == 0 {
            return fmt.Errorf("section %q requires source_refs for owner %q", section.SectionID, section.SourceOwner)
        }
        if err := validateUniqueStrings("section source_refs", section.SourceRefs); err != nil {
            return err
        }
        if err := validateUniqueStrings("section evidence_refs", section.EvidenceRefs); err != nil {
            return err
        }
        if err := validateQualityStatus(section.QualityStatus); err != nil {
            return fmt.Errorf("section %q: %w", section.SectionID, err)
        }
        if section.AssembledAt.IsZero() || !isUTC(section.AssembledAt) {
            return fmt.Errorf("section %q assembled_at is required and must use UTC", section.SectionID)
        }
        if section.SystemAsOf.IsZero() || !isUTC(section.SystemAsOf) || !section.SystemAsOf.Equal(c.SystemAsOf) {
            return fmt.Errorf("section %q system_as_of does not match Context", section.SectionID)
        }
        sections[section.SectionID] = section
        if section.Included {
            included[section.SectionID] = struct{}{}
        } else {
            omitted[section.SectionID] = struct{}{}
            if section.Required {
                missingRequired++
            } else {
                missingOptional++
            }
        }
    }

    if err := validateUniqueStrings("included_sections", c.IncludedSections); err != nil {
        return err
    }
    if len(c.IncludedSections) != len(included) {
        return fmt.Errorf("included_sections does not match section_refs")
    }
    for _, sectionID := range c.IncludedSections {
        if _, exists := included[sectionID]; !exists {
            return fmt.Errorf("included_sections contains unknown or omitted section %q", sectionID)
        }
    }

    if len(c.OmittedSections) != len(omitted) {
        return fmt.Errorf("omitted_sections does not match section_refs")
    }
    for _, value := range c.OmittedSections {
        if strings.TrimSpace(value.SectionID) == "" || strings.TrimSpace(value.SafeMessage) == "" {
            return fmt.Errorf("omitted section section_id and safe_message are required")
        }
        section, exists := sections[value.SectionID]
        if !exists || section.Included {
            return fmt.Errorf("omitted_sections contains unknown or included section %q", value.SectionID)
        }
        if _, exists := omitted[value.SectionID]; !exists {
            return fmt.Errorf("omitted_sections contains duplicate section %q", value.SectionID)
        }
        delete(omitted, value.SectionID)
        if err := validateSourceOwner(value.SourceOwner); err != nil {
            return err
        }
        if value.SourceOwner != section.SourceOwner || value.Required != section.Required ||
            !slices.Equal(value.SourceRefs, section.SourceRefs) {
            return fmt.Errorf("omitted section %q metadata does not match section_refs", value.SectionID)
        }
        if value.SourceRefs == nil {
            return fmt.Errorf("omitted section %q source_refs must be a non-null array", value.SectionID)
        }
        if err := validateUniqueStrings("omitted section source_refs", value.SourceRefs); err != nil {
            return err
        }
        if err := validateOmissionReason(value.ReasonCode); err != nil {
            return err
        }
    }
    if len(omitted) != 0 {
        return fmt.Errorf("omitted_sections is missing one or more omitted sections")
    }

    if c.QualitySummary.MissingRequiredCount != missingRequired ||
        c.QualitySummary.MissingOptionalCount != missingOptional {
        return fmt.Errorf("quality_summary missing counts do not match omitted sections")
    }
    if missingRequired > 0 && c.QualitySummary.Status == QualityValid {
        return fmt.Errorf("Context with omitted required sections cannot have valid quality status")
    }
    return nil
}

// ValidateSystemAsOf compares the Context cutoff with the caller's authoritative Run cutoff.
func (c Context) ValidateSystemAsOf(expected time.Time) error {
    if expected.IsZero() {
        return fmt.Errorf("expected system_as_of is required")
    }
    if c.SystemAsOf.IsZero() || !c.SystemAsOf.Equal(expected) {
        return fmt.Errorf("context system_as_of does not match expected cutoff")
    }
    return nil
}

func validateQualitySummary(value QualitySummary) error {
    if err := validateQualityStatus(value.Status); err != nil {
        return err
    }
    if value.MissingRequiredCount < 0 || value.MissingOptionalCount < 0 ||
        value.ConflictCount < 0 || value.StaleCount < 0 {
        return fmt.Errorf("quality_summary counts cannot be negative")
    }
    if value.ReasonCodes == nil {
        return fmt.Errorf("quality_summary reason_codes must be a non-null array")
    }
    if err := validateUniqueStrings("quality_summary reason_codes", value.ReasonCodes); err != nil {
        return err
    }
    if !slices.IsSorted(value.ReasonCodes) {
        return fmt.Errorf("quality_summary reason_codes must be sorted")
    }
    if value.Status != QualityValid && len(value.ReasonCodes) == 0 {
        return fmt.Errorf("quality_summary reason_codes are required for non-valid status")
    }
    return nil
}

func validateQualityStatus(value QualityStatus) error {
    switch value {
    case QualityValid, QualityDegraded, QualityInvalid, QualityUnknown:
        return nil
    default:
        return fmt.Errorf("quality_status is invalid")
    }
}

func validateScope(value Scope) error {
    switch value {
    case ScopeMarket, ScopeIndustry, ScopeStock, ScopeEvent:
        return nil
    default:
        return fmt.Errorf("scope is invalid")
    }
}

func validateSourceOwner(value SourceOwner) error {
    switch value {
    case SourceOwnerData, SourceOwnerQuant, SourceOwnerResearch, SourceOwnerNews:
        return nil
    default:
        return fmt.Errorf("source_owner is invalid")
    }
}

func validateOmissionReason(value OmissionReason) error {
    switch value {
    case OmissionNotRequested, OmissionBudgetExceeded, OmissionUnavailable, OmissionUnauthorized,
        OmissionQualityRejected, OmissionFailed, OmissionDuplicate:
        return nil
    default:
        return fmt.Errorf("omitted section reason_code is invalid")
    }
}

func validateUniqueStrings(name string, values []string) error {
    seen := make(map[string]struct{}, len(values))
    for _, value := range values {
        if value == "" || value != strings.TrimSpace(value) {
            return fmt.Errorf("%s cannot contain empty or whitespace-padded values", name)
        }
        if _, exists := seen[value]; exists {
            return fmt.Errorf("%s contains duplicate value %q", name, value)
        }
        seen[value] = struct{}{}
    }
    return nil
}


// ValidateNextVersion verifies immutable Context version progression.
// It checks only version-chain invariants; referenced resources and their
// current authorization remain the responsibility of their owning modules.
func (next Context) ValidateNextVersion(previous Context) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous Context: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("next Context: %w", err)
	}
	if next.ContextID != previous.ContextID {
		return fmt.Errorf("context_id cannot change across versions")
	}
	if previous.ContextVersion == math.MaxInt64 {
		return fmt.Errorf("context_version reached the supported integer limit")
	}
	if next.ContextVersion != previous.ContextVersion+1 {
		return fmt.Errorf("context_version must increment by exactly one")
	}
	if next.SystemAsOf.Before(previous.SystemAsOf) {
		return fmt.Errorf("system_as_of cannot move backwards across Context versions")
	}
	if next.CreatedAt.Before(previous.CreatedAt) {
		return fmt.Errorf("created_at cannot move backwards across Context versions")
	}
	return nil
}


func isUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}
