package source

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
)

var ErrInvalidAvailabilityQualificationCatalog = errors.New("news: invalid availability qualification catalog")

type availabilityQualificationKey struct {
	sourceID                      string
	endpointID                    string
	providerCode                  string
	sourcePolicyVersionID         string
	endpointPolicyVersionID       string
	providerOutputPolicyRef       string
	providerOutputPolicyVersionID string
	fieldPath                     string
}

// StaticAvailabilityQualificationCatalog is an immutable startup snapshot of
// source-owner-approved availability field semantics. An empty catalog is valid
// and fail-closed: no source may qualify a time field without an explicit record.
type StaticAvailabilityQualificationCatalog struct {
	entries map[availabilityQualificationKey][]app.AvailabilitySourceQualification
}

func NewStaticAvailabilityQualificationCatalog(
	qualifications []app.AvailabilitySourceQualification,
) (*StaticAvailabilityQualificationCatalog, error) {
	catalog := &StaticAvailabilityQualificationCatalog{
		entries: make(map[availabilityQualificationKey][]app.AvailabilitySourceQualification),
	}
	seenRefs := make(map[string]struct{}, len(qualifications))
	for _, qualification := range qualifications {
		if err := qualification.Validate(); err != nil {
			return nil, errors.Join(ErrInvalidAvailabilityQualificationCatalog, err)
		}
		if _, exists := seenRefs[qualification.QualificationRef]; exists {
			return nil, fmt.Errorf("%w: duplicate qualification ref %q", ErrInvalidAvailabilityQualificationCatalog, qualification.QualificationRef)
		}
		seenRefs[qualification.QualificationRef] = struct{}{}
		key := availabilityQualificationKeyFor(qualification.SourceID, qualification.EndpointID,
			qualification.ProviderCode, qualification.SourcePolicyVersionID,
			qualification.EndpointPolicyVersionID, qualification.ProviderOutputPolicyRef,
			qualification.ProviderOutputPolicyVersionID, qualification.FieldPath)
		catalog.entries[key] = append(catalog.entries[key], qualification.Clone())
	}
	for _, entries := range catalog.entries {
		sortAvailabilityQualifications(entries)
		for index := 1; index < len(entries); index++ {
			if entries[index].EffectiveAt.Equal(entries[index-1].EffectiveAt) {
				return nil, fmt.Errorf("%w: ambiguous effective_at for one binding", ErrInvalidAvailabilityQualificationCatalog)
			}
		}
	}
	return catalog, nil
}

func (c *StaticAvailabilityQualificationCatalog) LookupAvailabilitySourceQualification(
	ctx context.Context,
	binding app.AvailabilityEvidenceSourceBinding,
	fieldPath string,
	now time.Time,
) (app.AvailabilitySourceQualification, error) {
	if c == nil || ctx == nil || now.IsZero() {
		return app.AvailabilitySourceQualification{}, app.ErrAvailabilitySourceQualificationNotFound
	}
	if err := ctx.Err(); err != nil {
		return app.AvailabilitySourceQualification{}, err
	}
	key := availabilityQualificationKeyFor(binding.SourceID, binding.EndpointID,
		binding.ProviderCode, binding.SourcePolicyVersionID,
		binding.EndpointPolicyVersionID, binding.ProviderOutputPolicyRef,
		binding.ProviderOutputPolicyVersionID, fieldPath)
	entries := c.entries[key]
	boundary := now.UTC()
	for index := len(entries) - 1; index >= 0; index-- {
		if !entries[index].EffectiveAt.After(boundary) {
			return entries[index].Clone(), nil
		}
	}
	return app.AvailabilitySourceQualification{}, app.ErrAvailabilitySourceQualificationNotFound
}

func availabilityQualificationKeyFor(
	sourceID, endpointID, providerCode, sourcePolicyVersionID, endpointPolicyVersionID,
	providerOutputPolicyRef, providerOutputPolicyVersionID, fieldPath string,
) availabilityQualificationKey {
	return availabilityQualificationKey{
		sourceID:                      sourceID,
		endpointID:                    endpointID,
		providerCode:                  providerCode,
		sourcePolicyVersionID:         sourcePolicyVersionID,
		endpointPolicyVersionID:       endpointPolicyVersionID,
		providerOutputPolicyRef:       providerOutputPolicyRef,
		providerOutputPolicyVersionID: providerOutputPolicyVersionID,
		fieldPath:                     fieldPath,
	}
}

func sortAvailabilityQualifications(values []app.AvailabilitySourceQualification) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0; cursor-- {
			if !values[cursor].EffectiveAt.Before(values[cursor-1].EffectiveAt) {
				break
			}
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}

var _ app.AvailabilitySourceQualificationCatalog = (*StaticAvailabilityQualificationCatalog)(nil)
