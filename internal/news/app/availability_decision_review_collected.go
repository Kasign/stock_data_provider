package app

import (
	"context"
	"errors"
	"sort"

	"example.com/stock-analysis-flow/internal/news/domain"
)

// ErrAvailabilityReviewCollectedInput fails closed when G4-recorded evidence
// cannot be reviewed as a complete, version-bound set.
var ErrAvailabilityReviewCollectedInput = errors.New("news.available_at.review_collected_input_invalid")

// ReviewCollectedAvailabilityRequest carries only Evidence records returned by
// the G4 recorder in this same Feed execution. Callers cannot pass a decision
// or an availability value.
type ReviewCollectedAvailabilityRequest struct {
	Evidence []domain.AvailabilityEvidence
}

// ReviewCollectedAvailability bridges G4 Evidence to the existing production
// ReviewAvailabilityDecision use case. Evidence is grouped by its exact
// persisted VersionID so one append-only decision chain is produced per
// version; the source-bound rule and the repository binding resolver decide
// whether any candidate becomes verified. Nothing here upgrades `parsed` on
// its own or writes an availability value.
type ReviewCollectedAvailability struct {
	review      *ReviewAvailabilityDecision
	ruleVersion string
}

func NewReviewCollectedAvailability(
	review *ReviewAvailabilityDecision,
	ruleVersion string,
) (*ReviewCollectedAvailability, error) {
	if review == nil || !validAvailabilityReviewRef(ruleVersion) {
		return nil, ErrAvailabilityReviewCollectedInput
	}
	return &ReviewCollectedAvailability{review: review, ruleVersion: ruleVersion}, nil
}

func (u *ReviewCollectedAvailability) Execute(
	ctx context.Context,
	request ReviewCollectedAvailabilityRequest,
) ([]domain.AvailabilityDecision, error) {
	if u == nil || u.review == nil || !validAvailabilityReviewRef(u.ruleVersion) || ctx == nil {
		return nil, ErrAvailabilityReviewCollectedInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(request.Evidence) == 0 {
		return []domain.AvailabilityDecision{}, nil
	}

	grouped := make(map[string][]domain.AvailabilityEvidence)
	for _, evidence := range request.Evidence {
		versionID := evidence.VersionID
		if !validAvailabilityReviewRef(versionID) || !validAvailabilityReviewRef(evidence.ID) {
			return nil, ErrAvailabilityReviewCollectedInput
		}
		grouped[versionID] = append(grouped[versionID], evidence)
	}
	if len(grouped) == 0 {
		return nil, ErrAvailabilityReviewCollectedInput
	}

	versionIDs := make([]string, 0, len(grouped))
	for versionID := range grouped {
		versionIDs = append(versionIDs, versionID)
	}
	sort.Strings(versionIDs)

	decisions := make([]domain.AvailabilityDecision, 0, len(versionIDs))
	for _, versionID := range versionIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		decision, err := u.review.Execute(ctx, ReviewAvailabilityDecisionRequest{
			VersionID:         versionID,
			Evidence:          grouped[versionID],
			ReviewRuleVersion: u.ruleVersion,
		})
		if err != nil {
			return nil, errors.Join(ErrAvailabilityReviewCollectedInput, err)
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}
