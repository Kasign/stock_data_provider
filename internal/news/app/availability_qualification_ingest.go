package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrAvailabilityQualificationInput = errors.New("news.available_at.qualification_input_invalid")

// QualifyCollectedAvailabilityRequest is the G3 boundary between a trusted
// collection result and the source-qualification use case. FieldPaths come
// only from the immutable startup qualification catalog; callers cannot name
// an arbitrary response field.
type QualifyCollectedAvailabilityRequest struct {
	Registration SourceEndpointRegistration
	Collection   CollectSourceResult
	FieldPaths   []string
}

// QualifiedCollectedAvailability keeps the collection entry identity next to
// the untrusted time candidate. Record/Review (G4/G5) must resolve the
// Observation and NewsItemVersion separately; this type never carries a
// Decision or an available_at value.
type QualifiedCollectedAvailability struct {
	EntryIndex   int
	SourceItemID string
	Binding      AvailabilityEvidenceSourceBinding
	Qualified    QualifiedAvailabilityEvidence
}

// QualifyCollectedAvailability applies the immutable source qualification to
// fields captured by CollectSource. It is in-memory only: no Evidence, no
// Decision, no persistence and no available_at projection.
type QualifyCollectedAvailability struct {
	qualify *QualifyAvailabilityEvidence
}

func NewQualifyCollectedAvailability(catalog AvailabilitySourceQualificationCatalog) (*QualifyCollectedAvailability, error) {
	qualify, err := NewQualifyAvailabilityEvidence(catalog)
	if err != nil {
		return nil, err
	}
	return &QualifyCollectedAvailability{qualify: qualify}, nil
}

func (u *QualifyCollectedAvailability) Execute(
	ctx context.Context,
	request QualifyCollectedAvailabilityRequest,
) ([]QualifiedCollectedAvailability, error) {
	if u == nil || u.qualify == nil || ctx == nil {
		return nil, ErrAvailabilityQualificationInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(request.FieldPaths) == 0 {
		return nil, nil
	}
	if request.Collection.SourceID != request.Registration.SourceID ||
		request.Collection.EndpointID != request.Registration.EndpointID ||
		request.Collection.SourcePolicyVersionID != request.Registration.SourcePolicyVersionID ||
		request.Collection.EndpointPolicyVersionID != request.Registration.EndpointPolicyVersionID {
		return nil, ErrAvailabilityQualificationInput
	}
	if !request.Collection.hasValidCollectionSeal() {
		return nil, ErrAvailabilityQualificationInput
	}
	binding := AvailabilityEvidenceSourceBinding{
		SourceID:                      request.Registration.SourceID,
		EndpointID:                    request.Registration.EndpointID,
		ProviderCode:                  request.Registration.ProviderCode,
		SourcePolicyVersionID:         request.Registration.SourcePolicyVersionID,
		EndpointPolicyVersionID:       request.Registration.EndpointPolicyVersionID,
		ProviderOutputPolicyRef:       request.Registration.ProviderOutputPolicyRef,
		ProviderOutputPolicyVersionID: request.Registration.ProviderOutputPolicyVersionID,
	}
	if err := validateAvailabilityEvidenceSourceBinding(binding, false); err != nil {
		return nil, errors.Join(ErrAvailabilityQualificationInput, err)
	}
	evidenceRef := strings.TrimSpace(request.Collection.EvidenceRef)
	evidenceHash := strings.TrimSpace(request.Collection.ResponseHash)
	if !validAvailabilityLongRef(evidenceRef, 512) || !validLowerSHA256(evidenceHash) {
		return nil, ErrAvailabilityQualificationInput
	}

	fieldPaths := make([]string, 0, len(request.FieldPaths))
	seenPaths := make(map[string]struct{}, len(request.FieldPaths))
	for _, fieldPath := range request.FieldPaths {
		fieldPath = strings.TrimSpace(fieldPath)
		if !validMetadataEvidenceFieldPath(fieldPath) {
			return nil, ErrAvailabilityQualificationInput
		}
		if _, exists := seenPaths[fieldPath]; exists {
			continue
		}
		seenPaths[fieldPath] = struct{}{}
		fieldPaths = append(fieldPaths, fieldPath)
	}
	sort.Strings(fieldPaths)

	results := make([]QualifiedCollectedAvailability, 0)
	for _, fieldPath := range fieldPaths {
		for index, entry := range request.Collection.Entries {
			rawValue := strings.TrimSpace(entry.Candidate.PublishedAtRaw)
			if rawValue == "" {
				continue
			}
			qualified, err := u.qualify.Execute(ctx, QualifyAvailabilityEvidenceRequest{
				Binding:      binding,
				FieldPath:    fieldPath,
				RawValue:     rawValue,
				EvidenceRef:  evidenceRef,
				EvidenceHash: evidenceHash,
			})
			if errors.Is(err, ErrAvailabilitySourceQualificationNotFound) {
				// The same field path is not qualified for this exact binding:
				// skip the source without inventing a candidate.
				break
			}
			if err != nil {
				return nil, fmt.Errorf("qualify collected availability at %s: %w", fieldPath, err)
			}
			results = append(results, QualifiedCollectedAvailability{
				EntryIndex:   index,
				SourceItemID: strings.TrimSpace(entry.Candidate.SourceItemID),
				Binding:      binding,
				Qualified:    qualified,
			})
		}
	}
	return results, nil
}
