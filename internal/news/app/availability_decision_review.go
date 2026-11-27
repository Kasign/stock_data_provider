package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrAvailabilityReviewUnavailable    = errors.New("news.available_at.review_unavailable")
	ErrAvailabilityReviewInvalidRequest = errors.New("news.available_at.review_invalid_request")
	ErrAvailabilityReviewRuleRejected   = errors.New("news.available_at.review_rule_rejected")
	ErrAvailabilityDecisionAppendFailed = errors.New("news.available_at.decision_append_failed")
	ErrAvailabilityDecisionNotFound     = errors.New("news.available_at.decision_not_found")
)

const (
	AvailabilityReviewReasonVerified   = "availability.review_verified"
	AvailabilityReviewReasonConflict   = "availability.review_conflict"
	AvailabilityReviewReasonUnresolved = "availability.review_unresolved"
	AvailabilityReviewReasonRevoked    = "availability.review_revoked"
)

type AvailabilityEvidenceReviewOutcome string

const (
	AvailabilityEvidenceReviewEligible   AvailabilityEvidenceReviewOutcome = "eligible"
	AvailabilityEvidenceReviewIneligible AvailabilityEvidenceReviewOutcome = "ineligible"
	AvailabilityEvidenceReviewRevoked    AvailabilityEvidenceReviewOutcome = "revoked"
)

// AvailabilityEvidenceReview is a source-agnostic rule result. Eligible means
// the rule has proved the source field semantics and authorization binding for
// this exact evidence record; parsed alone is never sufficient.
type AvailabilityEvidenceReview struct {
	Outcome    AvailabilityEvidenceReviewOutcome
	ReasonCode string
	Preferred  bool
}

// AvailabilityReviewRule is injected by the source-qualification layer. The
// review use case never infers source semantics from field names or values.
type AvailabilityReviewRule interface {
	Version() string
	Evaluate(context.Context, domain.AvailabilityEvidence) (AvailabilityEvidenceReview, error)
}

// AvailabilityDecisionState is the latest append-only decision chain head.
type AvailabilityDecisionState struct {
	VersionID  string
	DecisionID string
	Sequence   uint64
	Status     domain.AvailabilityDecisionStatus
}

// AvailabilityDecisionStore is the narrow persistence port for review. The
// append implementation performs version-row serialization and rejects stale
// predecessors, so concurrent reviews cannot silently fork the chain.
type AvailabilityDecisionStore interface {
	LatestAvailabilityDecisionForVersion(context.Context, string) (AvailabilityDecisionState, error)
	AppendAvailabilityDecision(context.Context, domain.AvailabilityDecision) error
}

// ReviewAvailabilityDecisionRequest contains only immutable evidence records
// and the rule version that evaluated them. Callers cannot pass an availability
// value or a pre-approved decision.
type ReviewAvailabilityDecisionRequest struct {
	VersionID         string
	Evidence          []domain.AvailabilityEvidence
	ReviewRuleVersion string
}

// ReviewAvailabilityDecision produces one complete append-only visibility
// state for a version. It does not populate news_item_versions.available_at;
// the PIT read path consumes the decision separately.
type ReviewAvailabilityDecision struct {
	decisions AvailabilityDecisionStore
	rule      AvailabilityReviewRule
	newID     func() string
	now       func() time.Time
}

func NewReviewAvailabilityDecision(
	decisions AvailabilityDecisionStore,
	rule AvailabilityReviewRule,
	newID func() string,
) (*ReviewAvailabilityDecision, error) {
	if decisions == nil || rule == nil || newID == nil {
		return nil, ErrAvailabilityReviewUnavailable
	}
	return &ReviewAvailabilityDecision{
		decisions: decisions,
		rule:      rule,
		newID:     newID,
		now:       time.Now,
	}, nil
}

func (u *ReviewAvailabilityDecision) Execute(
	ctx context.Context,
	request ReviewAvailabilityDecisionRequest,
) (domain.AvailabilityDecision, error) {
	if u == nil || u.decisions == nil || u.rule == nil || u.newID == nil || u.now == nil || ctx == nil {
		return domain.AvailabilityDecision{}, ErrAvailabilityReviewUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.AvailabilityDecision{}, err
	}
	if !validAvailabilityReviewRef(request.VersionID) ||
		!validAvailabilityReviewRef(request.ReviewRuleVersion) {
		return domain.AvailabilityDecision{}, ErrAvailabilityReviewInvalidRequest
	}
	ruleVersion := u.rule.Version()
	if !validAvailabilityReviewRef(ruleVersion) || ruleVersion != request.ReviewRuleVersion {
		return domain.AvailabilityDecision{}, ErrAvailabilityReviewRuleRejected
	}

	recordedAt := u.now().UTC().Truncate(time.Microsecond)
	if recordedAt.IsZero() {
		return domain.AvailabilityDecision{}, ErrAvailabilityReviewInvalidRequest
	}
	evidence, considered, err := normalizeAvailabilityReviewEvidence(request.VersionID, request.Evidence, recordedAt)
	if err != nil {
		return domain.AvailabilityDecision{}, errors.Join(ErrAvailabilityReviewInvalidRequest, err)
	}

	state, err := u.decisions.LatestAvailabilityDecisionForVersion(ctx, request.VersionID)
	switch {
	case err == nil:
		if state.Sequence == 0 || state.VersionID != request.VersionID ||
			!validAvailabilityReviewRef(state.DecisionID) || !validAvailabilityDecisionStatus(state.Status) {
			return domain.AvailabilityDecision{}, ErrAvailabilityReviewInvalidRequest
		}
	case errors.Is(err, ErrAvailabilityDecisionNotFound):
		state = AvailabilityDecisionState{VersionID: request.VersionID}
	default:
		return domain.AvailabilityDecision{}, errors.Join(ErrAvailabilityReviewUnavailable, err)
	}
	if state.Sequence == ^uint64(0) {
		return domain.AvailabilityDecision{}, ErrAvailabilityReviewInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return domain.AvailabilityDecision{}, err
	}

	reviews, err := evaluateAvailabilityReviews(ctx, u.rule, evidence)
	if err != nil {
		return domain.AvailabilityDecision{}, errors.Join(ErrAvailabilityReviewRuleRejected, err)
	}
	status, selected, reasonCode, err := decideAvailabilityStatus(reviews)
	if err != nil {
		return domain.AvailabilityDecision{}, errors.Join(ErrAvailabilityReviewRuleRejected, err)
	}

	var predecessor *string
	if state.Sequence > 0 {
		value := state.DecisionID
		predecessor = &value
	}
	decision := domain.AvailabilityDecision{
		ID:                    u.newID(),
		VersionID:             request.VersionID,
		Sequence:              state.Sequence + 1,
		Status:                status,
		SelectedEvidenceID:    selected,
		ConsideredEvidenceIDs: considered,
		ReasonCode:            reasonCode,
		ReviewRuleVersion:     request.ReviewRuleVersion,
		SupersedesDecisionID:  predecessor,
		RecordedAt:            recordedAt,
	}
	if err := decision.Validate(); err != nil {
		return domain.AvailabilityDecision{}, errors.Join(ErrAvailabilityReviewInvalidRequest, err)
	}
	if err := ctx.Err(); err != nil {
		return domain.AvailabilityDecision{}, err
	}
	if err := u.decisions.AppendAvailabilityDecision(ctx, decision); err != nil {
		return domain.AvailabilityDecision{}, errors.Join(ErrAvailabilityDecisionAppendFailed, err)
	}
	return decision, nil
}

func normalizeAvailabilityReviewEvidence(
	versionID string,
	input []domain.AvailabilityEvidence,
	recordedAt time.Time,
) ([]domain.AvailabilityEvidence, []string, error) {
	evidence := make([]domain.AvailabilityEvidence, len(input))
	copy(evidence, input)
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].ID < evidence[j].ID })

	var newsID, observationID string
	seen := make(map[string]struct{}, len(evidence))
	ids := make([]string, 0, len(evidence))
	for index, item := range evidence {
		if !validAvailabilityReviewRef(item.ID) || item.VersionID != versionID ||
			item.PayloadPurgedAt != nil || !validAvailabilityReviewRef(item.NewsID) ||
			!validAvailabilityReviewRef(item.ObservationID) {
			return nil, nil, errors.New("considered evidence is not bound to an active version")
		}
		if index == 0 {
			newsID, observationID = item.NewsID, item.ObservationID
		} else if item.NewsID != newsID || item.ObservationID != observationID {
			return nil, nil, errors.New("considered evidence has inconsistent identity bindings")
		}
		if _, exists := seen[item.ID]; exists {
			return nil, nil, errors.New("considered evidence contains a duplicate id")
		}
		seen[item.ID] = struct{}{}
		if err := item.Validate(); err != nil {
			return nil, nil, err
		}
		if item.RecordedAt.After(recordedAt) || item.ObservedAt.After(recordedAt) ||
			!item.RetentionUntil.After(recordedAt) {
			return nil, nil, errors.New("considered evidence is not available at review time")
		}
		ids = append(ids, item.ID)
	}
	return evidence, ids, nil
}

type availabilityEvidenceReviewResult struct {
	evidence domain.AvailabilityEvidence
	review   AvailabilityEvidenceReview
}

func evaluateAvailabilityReviews(
	ctx context.Context,
	rule AvailabilityReviewRule,
	evidence []domain.AvailabilityEvidence,
) ([]availabilityEvidenceReviewResult, error) {
	results := make([]availabilityEvidenceReviewResult, 0, len(evidence))
	for _, item := range evidence {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		review, err := rule.Evaluate(ctx, item)
		if err != nil {
			return nil, err
		}
		if err := validateAvailabilityEvidenceReview(review); err != nil {
			return nil, err
		}
		results = append(results, availabilityEvidenceReviewResult{evidence: item, review: review})
	}
	return results, nil
}

func validateAvailabilityEvidenceReview(review AvailabilityEvidenceReview) error {
	switch review.Outcome {
	case AvailabilityEvidenceReviewEligible, AvailabilityEvidenceReviewIneligible, AvailabilityEvidenceReviewRevoked:
	default:
		return errors.New("availability review outcome is invalid")
	}
	if review.Outcome != AvailabilityEvidenceReviewEligible && review.Preferred {
		return errors.New("only eligible evidence can be preferred")
	}
	if review.ReasonCode == "" {
		return nil
	}
	if review.ReasonCode != strings.TrimSpace(review.ReasonCode) || len(review.ReasonCode) > 128 {
		return errors.New("availability review reason code is invalid")
	}
	return nil
}

func decideAvailabilityStatus(
	reviews []availabilityEvidenceReviewResult,
) (domain.AvailabilityDecisionStatus, *string, string, error) {
	for _, result := range reviews {
		if result.review.Outcome != AvailabilityEvidenceReviewRevoked {
			continue
		}
		reasonCode := result.review.ReasonCode
		if reasonCode == "" {
			reasonCode = AvailabilityReviewReasonRevoked
		}
		return domain.AvailabilityDecisionRevoked, nil, reasonCode, nil
	}

	eligible := make([]availabilityEvidenceReviewResult, 0, len(reviews))
	for _, result := range reviews {
		if result.review.Outcome != AvailabilityEvidenceReviewEligible {
			continue
		}
		if err := eligibleAvailabilityEvidence(result.evidence); err != nil {
			return "", nil, "", err
		}
		eligible = append(eligible, result)
	}
	if len(eligible) == 0 {
		return domain.AvailabilityDecisionUnresolved, nil, AvailabilityReviewReasonUnresolved, nil
	}
	if len(eligible) == 1 {
		return verifiedAvailabilityDecision(eligible[0])
	}

	preferred := make([]availabilityEvidenceReviewResult, 0, 1)
	for _, result := range eligible {
		if result.review.Preferred {
			preferred = append(preferred, result)
		}
	}
	if len(preferred) == 1 {
		return verifiedAvailabilityDecision(preferred[0])
	}
	return domain.AvailabilityDecisionConflict, nil, AvailabilityReviewReasonConflict, nil
}

func verifiedAvailabilityDecision(
	result availabilityEvidenceReviewResult,
) (domain.AvailabilityDecisionStatus, *string, string, error) {
	selected := result.evidence.ID
	reasonCode := result.review.ReasonCode
	if reasonCode == "" {
		reasonCode = AvailabilityReviewReasonVerified
	}
	return domain.AvailabilityDecisionVerified, &selected, reasonCode, nil
}

func eligibleAvailabilityEvidence(evidence domain.AvailabilityEvidence) error {
	if evidence.ParseStatus != domain.AvailabilityTimeParsed ||
		evidence.CandidateAvailableAt == nil ||
		(evidence.Precision != domain.AvailabilityTimePrecisionSecond &&
			evidence.Precision != domain.AvailabilityTimePrecisionMinute) ||
		evidence.SourceTimezoneOrOffset == "" ||
		(evidence.EvidenceRef == nil && evidence.EvidenceHash == nil) {
		return errors.New("eligible availability evidence lacks verified provenance")
	}
	return nil
}

func validAvailabilityDecisionStatus(status domain.AvailabilityDecisionStatus) bool {
	switch status {
	case domain.AvailabilityDecisionVerified, domain.AvailabilityDecisionConflict,
		domain.AvailabilityDecisionUnresolved, domain.AvailabilityDecisionRevoked:
		return true
	default:
		return false
	}
}

func validAvailabilityReviewRef(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 128
}
