package app

import (
	"context"
	"errors"
	"strings"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var ErrAvailabilityRecordInput = errors.New("news.available_at.record_input_invalid")

// IngestCollectedSourceResult keeps the persisted Observation together with the
// exact NewsItemVersion written in the same identity transaction. G4 must not
// reconstruct a Version from a fingerprint: doing so would claim an unpersisted
// content version.
type IngestCollectedSourceResult struct {
	Observation domain.Observation
	Version     domain.NewsItemVersion
}

// RecordCollectedAvailabilityRequest is produced inside one Feed collection
// execution. Qualified entries are matched by their original collection index;
// ingest entries are the exact persisted Observation/Version bindings.
type RecordCollectedAvailabilityRequest struct {
	Qualified []QualifiedCollectedAvailability
	Ingest    []IngestCollectedSourceResult
}

// RecordCollectedAvailability is the G4 bridge from G3's untrusted candidate to
// the existing RecordAvailabilityEvidence use case. It adds no authorization
// conclusion and never creates a Decision or available_at projection.
type RecordCollectedAvailability struct {
	recorder *RecordAvailabilityEvidence
}

func NewRecordCollectedAvailability(recorder *RecordAvailabilityEvidence) (*RecordCollectedAvailability, error) {
	if recorder == nil {
		return nil, ErrAvailabilityRecordInput
	}
	return &RecordCollectedAvailability{recorder: recorder}, nil
}

func (u *RecordCollectedAvailability) Execute(
	ctx context.Context,
	request RecordCollectedAvailabilityRequest,
) ([]domain.AvailabilityEvidence, error) {
	if u == nil || u.recorder == nil || ctx == nil {
		return nil, ErrAvailabilityRecordInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(request.Qualified) == 0 {
		return []domain.AvailabilityEvidence{}, nil
	}
	if len(request.Ingest) == 0 {
		return nil, ErrAvailabilityRecordInput
	}

	recorded := make([]domain.AvailabilityEvidence, 0, len(request.Qualified))
	for _, qualified := range request.Qualified {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if qualified.EntryIndex < 0 || qualified.EntryIndex >= len(request.Ingest) {
			return nil, ErrAvailabilityRecordInput
		}
		bound := request.Ingest[qualified.EntryIndex]
		if err := validateRecordedAvailabilityBinding(qualified, bound); err != nil {
			return nil, err
		}
		recordRequest := RecordAvailabilityEvidenceRequest{
			Observation:   bound.Observation,
			Version:       bound.Version,
			FieldPath:     qualified.Qualified.FieldPath,
			Candidate:     qualified.Qualified.Candidate,
			ParserVersion: qualified.Qualified.ParserVersion,
			EvidenceRef:   qualified.Qualified.EvidenceRef,
			EvidenceHash:  qualified.Qualified.EvidenceHash,
		}
		recordRequest.EvidenceID = recordAvailabilityEvidenceID(recordRequest)
		if recordRequest.EvidenceID == "" {
			return nil, ErrAvailabilityRecordInput
		}
		evidence, err := u.recorder.Execute(ctx, recordRequest)
		if err != nil {
			return nil, err
		}
		recorded = append(recorded, evidence)
	}
	return recorded, nil
}

func validateRecordedAvailabilityBinding(
	qualified QualifiedCollectedAvailability,
	bound IngestCollectedSourceResult,
) error {
	observation, version := bound.Observation, bound.Version
	binding := qualified.Binding
	if strings.TrimSpace(observation.ID) == "" || strings.TrimSpace(observation.NewsID) == "" ||
		strings.TrimSpace(version.ID) == "" ||
		version.NewsID != observation.NewsID ||
		version.ObservationID != observation.ID ||
		!version.ObservedAt.Equal(observation.ObservedAt) {
		return ErrAvailabilityRecordInput
	}
	if observation.SourceID != binding.SourceID || observation.EndpointID != binding.EndpointID ||
		observation.SourcePolicyVersionID != binding.SourcePolicyVersionID ||
		observation.EndpointPolicyVersionID != binding.EndpointPolicyVersionID ||
		observation.ProviderCode != binding.ProviderCode ||
		observation.ProviderOutputPolicyRef != binding.ProviderOutputPolicyRef ||
		observation.ProviderOutputPolicyVersionID != binding.ProviderOutputPolicyVersionID {
		return ErrAvailabilityRecordInput
	}
	if strings.TrimSpace(qualified.SourceItemID) == "" ||
		observation.Candidate.SourceItemID != qualified.SourceItemID {
		return ErrAvailabilityRecordInput
	}
	if binding.EvidenceID != "" {
		return ErrAvailabilityRecordInput
	}
	return nil
}
