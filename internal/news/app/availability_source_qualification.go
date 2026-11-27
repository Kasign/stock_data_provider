package app

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrAvailabilitySourceQualificationNotFound = errors.New("news.available_at.source_qualification_not_found")
	ErrAvailabilitySourceQualificationDenied   = errors.New("news.available_at.source_qualification_denied")
	ErrAvailabilityEvidenceBindingNotFound     = errors.New("news.available_at.evidence_binding_not_found")
)

type AvailabilitySourceQualificationStatus string

const (
	AvailabilitySourceQualificationQualified AvailabilitySourceQualificationStatus = "qualified"
	AvailabilitySourceQualificationRevoked   AvailabilitySourceQualificationStatus = "revoked"
)

// AvailabilityEvidenceSourceBinding binds one Evidence record to the exact
// Source/Endpoint/Provider policy versions observed when the field was read.
// A qualification is valid only for this complete binding.
type AvailabilityEvidenceSourceBinding struct {
	EvidenceID                    string
	SourceID                      string
	EndpointID                    string
	ProviderCode                  string
	SourcePolicyVersionID         string
	EndpointPolicyVersionID       string
	ProviderOutputPolicyRef       string
	ProviderOutputPolicyVersionID string
}

// AvailabilitySourceQualification is an immutable, source-owner-approved record
// proving that a specific source field is first publicly available at the value
// carried by the Evidence. It contains no raw payload and no sample value.
type AvailabilitySourceQualification struct {
	QualificationRef              string
	SourceID                      string
	EndpointID                    string
	ProviderCode                  string
	SourcePolicyVersionID         string
	EndpointPolicyVersionID       string
	ProviderOutputPolicyRef       string
	ProviderOutputPolicyVersionID string
	FieldPath                     string
	ParserVersion                 string
	ReviewRuleVersion             string
	DeclaredTimezoneOrOffset      string
	AllowedPrecisions             []domain.AvailabilityTimePrecision
	OfficialSemanticsRef          string
	EndpointAuthorizationRef      string
	VersionBindingRef             string
	RawResponseSampleHash         string
	Status                        AvailabilitySourceQualificationStatus
	EffectiveAt                   time.Time
	RevokedAt                     *time.Time
}

func (q AvailabilitySourceQualification) Validate() error {
	for name, value := range map[string]string{
		"qualification_ref":        q.QualificationRef,
		"source_id":                q.SourceID,
		"endpoint_id":              q.EndpointID,
		"source_policy_version":    q.SourcePolicyVersionID,
		"field_path":               q.FieldPath,
		"parser_version":           q.ParserVersion,
		"review_rule_version":      q.ReviewRuleVersion,
		"declared_timezone":        q.DeclaredTimezoneOrOffset,
		"official_semantics_ref":   q.OfficialSemanticsRef,
		"endpoint_authorization":   q.EndpointAuthorizationRef,
		"version_binding_ref":      q.VersionBindingRef,
		"raw_response_sample_hash": q.RawResponseSampleHash,
	} {
		if !validAvailabilityLongRef(value, 512) {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	if q.ProviderCode == "" {
		if q.ProviderOutputPolicyRef != "" || q.ProviderOutputPolicyVersionID != "" {
			return errors.New("provider policy binding requires a provider code")
		}
	} else if !validAvailabilityLongRef(q.ProviderCode, 128) ||
		!validAvailabilityLongRef(q.ProviderOutputPolicyRef, 128) ||
		!validAvailabilityLongRef(q.ProviderOutputPolicyVersionID, 128) {
		return errors.New("provider policy binding is invalid")
	}
	if !validAvailabilityLongRef(q.EndpointPolicyVersionID, 128) {
		return errors.New("endpoint policy version is invalid")
	}
	if !validMetadataEvidenceFieldPath(q.FieldPath) {
		return errors.New("availability qualification field path is invalid")
	}
	if len(q.AllowedPrecisions) == 0 {
		return errors.New("availability qualification needs explicit allowed precisions")
	}
	seen := make(map[domain.AvailabilityTimePrecision]struct{}, len(q.AllowedPrecisions))
	for _, precision := range q.AllowedPrecisions {
		if precision != domain.AvailabilityTimePrecisionSecond &&
			precision != domain.AvailabilityTimePrecisionMinute {
			return errors.New("availability qualification precision is not PIT-safe")
		}
		if _, exists := seen[precision]; exists {
			return errors.New("availability qualification precision is duplicated")
		}
		seen[precision] = struct{}{}
	}
	if _, err := hex.DecodeString(q.RawResponseSampleHash); err != nil ||
		len(q.RawResponseSampleHash) != 64 || q.RawResponseSampleHash != strings.ToLower(q.RawResponseSampleHash) {
		return errors.New("raw response sample hash must be lowercase SHA-256 hex")
	}
	if err := validQualificationUTC("effective_at", q.EffectiveAt); err != nil {
		return err
	}
	switch q.Status {
	case AvailabilitySourceQualificationQualified:
		if q.RevokedAt != nil {
			return errors.New("qualified source cannot already be revoked")
		}
	case AvailabilitySourceQualificationRevoked:
		if q.RevokedAt == nil {
			return errors.New("revoked source requires revoked_at")
		}
		if err := validQualificationUTC("revoked_at", *q.RevokedAt); err != nil {
			return err
		}
		if !q.RevokedAt.After(q.EffectiveAt) {
			return errors.New("revoked_at must be after effective_at")
		}
	default:
		return errors.New("availability qualification status is invalid")
	}
	return nil
}

func (q AvailabilitySourceQualification) Clone() AvailabilitySourceQualification {
	q.AllowedPrecisions = append([]domain.AvailabilityTimePrecision(nil), q.AllowedPrecisions...)
	if q.RevokedAt != nil {
		revokedAt := *q.RevokedAt
		q.RevokedAt = &revokedAt
	}
	return q
}

// AvailabilitySourceQualificationCatalog is an immutable startup snapshot.
// Revoked records remain readable so the review layer can distinguish revoked
// source semantics from never-qualified ones.
type AvailabilitySourceQualificationCatalog interface {
	LookupAvailabilitySourceQualification(
		context.Context,
		AvailabilityEvidenceSourceBinding,
		string,
		time.Time,
	) (AvailabilitySourceQualification, error)
}

// QualifyAvailabilityEvidenceRequest is a source-adapter boundary: the caller
// supplies the raw field and trusted policy binding, never an approval result.
type QualifyAvailabilityEvidenceRequest struct {
	Binding      AvailabilityEvidenceSourceBinding
	FieldPath    string
	RawValue     string
	EvidenceRef  string
	EvidenceHash string
}

// QualifiedAvailabilityEvidence is ready for RecordAvailabilityEvidence. The
// parsed candidate remains untrusted until a review decision selects it.
type QualifiedAvailabilityEvidence struct {
	FieldPath         string
	Candidate         domain.AvailabilityTimeCandidate
	ParserVersion     string
	ReviewRuleVersion string
	EvidenceRef       *string
	EvidenceHash      *string
}

type QualifyAvailabilityEvidence struct {
	catalog AvailabilitySourceQualificationCatalog
	now     func() time.Time
}

func NewQualifyAvailabilityEvidence(catalog AvailabilitySourceQualificationCatalog) (*QualifyAvailabilityEvidence, error) {
	if catalog == nil {
		return nil, ErrAvailabilitySourceQualificationDenied
	}
	return &QualifyAvailabilityEvidence{catalog: catalog, now: time.Now}, nil
}

func (u *QualifyAvailabilityEvidence) Execute(
	ctx context.Context,
	request QualifyAvailabilityEvidenceRequest,
) (QualifiedAvailabilityEvidence, error) {
	if u == nil || u.catalog == nil || u.now == nil || ctx == nil {
		return QualifiedAvailabilityEvidence{}, ErrAvailabilitySourceQualificationDenied
	}
	if err := ctx.Err(); err != nil {
		return QualifiedAvailabilityEvidence{}, err
	}
	if err := validateAvailabilityEvidenceSourceBinding(request.Binding, false); err != nil {
		return QualifiedAvailabilityEvidence{}, errors.Join(ErrAvailabilitySourceQualificationDenied, err)
	}
	if !validMetadataEvidenceFieldPath(request.FieldPath) ||
		!validAvailabilityLongRef(request.EvidenceRef, 512) ||
		!validLowerSHA256(request.EvidenceHash) {
		return QualifiedAvailabilityEvidence{}, ErrAvailabilitySourceQualificationDenied
	}

	now := u.now().UTC()
	if err := validQualificationUTC("review_time", now); err != nil {
		return QualifiedAvailabilityEvidence{}, ErrAvailabilitySourceQualificationDenied
	}
	qualification, err := u.catalog.LookupAvailabilitySourceQualification(
		ctx, request.Binding, request.FieldPath, now,
	)
	if err != nil {
		return QualifiedAvailabilityEvidence{}, err
	}
	if err := qualification.Validate(); err != nil {
		return QualifiedAvailabilityEvidence{}, errors.Join(ErrAvailabilitySourceQualificationDenied, err)
	}
	if !availabilitySourceBindingMatches(qualification, request.Binding) ||
		qualification.FieldPath != request.FieldPath ||
		!qualificationActiveAt(qualification, now) {
		return QualifiedAvailabilityEvidence{}, ErrAvailabilitySourceQualificationDenied
	}

	candidate := domain.ParseAvailabilityTime(request.RawValue)
	if candidate.Status != domain.AvailabilityTimeParsed || candidate.CandidateAvailableAt == nil ||
		candidate.SourceTimezoneOrOffset != qualification.DeclaredTimezoneOrOffset ||
		!containsAvailabilityPrecision(qualification.AllowedPrecisions, candidate.Precision) {
		return QualifiedAvailabilityEvidence{}, ErrAvailabilitySourceQualificationDenied
	}
	evidenceRef, evidenceHash := request.EvidenceRef, request.EvidenceHash
	return QualifiedAvailabilityEvidence{
		FieldPath:         qualification.FieldPath,
		Candidate:         candidate,
		ParserVersion:     qualification.ParserVersion,
		ReviewRuleVersion: qualification.ReviewRuleVersion,
		EvidenceRef:       &evidenceRef,
		EvidenceHash:      &evidenceHash,
	}, nil
}

// SourceBoundAvailabilityReviewRule is an optional extension used when source
// qualification is required. ReviewAvailabilityDecision resolves the Evidence
// binding before calling it; generic rules never see source metadata.
type SourceBoundAvailabilityReviewRule interface {
	AvailabilityReviewRule
	EvaluateSourceBound(
		context.Context,
		domain.AvailabilityEvidence,
		AvailabilityEvidenceSourceBinding,
	) (AvailabilityEvidenceReview, error)
}

type AvailabilityEvidenceBindingResolver interface {
	ResolveAvailabilityEvidenceSourceBinding(
		context.Context,
		domain.AvailabilityEvidence,
	) (AvailabilityEvidenceSourceBinding, error)
}

type SourceQualifiedAvailabilityReviewRule struct {
	catalog     AvailabilitySourceQualificationCatalog
	ruleVersion string
	now         func() time.Time
}

func NewSourceQualifiedAvailabilityReviewRule(
	catalog AvailabilitySourceQualificationCatalog,
	ruleVersion string,
) (*SourceQualifiedAvailabilityReviewRule, error) {
	if catalog == nil || !validAvailabilityReviewRef(ruleVersion) {
		return nil, ErrAvailabilitySourceQualificationDenied
	}
	return &SourceQualifiedAvailabilityReviewRule{
		catalog: catalog, ruleVersion: ruleVersion, now: time.Now,
	}, nil
}

func (r *SourceQualifiedAvailabilityReviewRule) Version() string {
	if r == nil {
		return ""
	}
	return r.ruleVersion
}

// Evaluate deliberately refuses to qualify Evidence without its source binding.
func (r *SourceQualifiedAvailabilityReviewRule) Evaluate(
	context.Context,
	domain.AvailabilityEvidence,
) (AvailabilityEvidenceReview, error) {
	return AvailabilityEvidenceReview{
		Outcome:    AvailabilityEvidenceReviewIneligible,
		ReasonCode: "availability.review_source_binding_required",
	}, nil
}

func (r *SourceQualifiedAvailabilityReviewRule) EvaluateSourceBound(
	ctx context.Context,
	evidence domain.AvailabilityEvidence,
	binding AvailabilityEvidenceSourceBinding,
) (AvailabilityEvidenceReview, error) {
	if r == nil || r.catalog == nil || r.now == nil || ctx == nil {
		return AvailabilityEvidenceReview{}, ErrAvailabilitySourceQualificationDenied
	}
	if err := ctx.Err(); err != nil {
		return AvailabilityEvidenceReview{}, err
	}
	if binding.EvidenceID != evidence.ID ||
		validateAvailabilityEvidenceSourceBinding(binding, true) != nil {
		return AvailabilityEvidenceReview{
			Outcome:    AvailabilityEvidenceReviewIneligible,
			ReasonCode: "availability.review_source_binding_invalid",
		}, nil
	}

	now := r.now().UTC()
	qualification, err := r.catalog.LookupAvailabilitySourceQualification(
		ctx, binding, evidence.FieldPath, now,
	)
	if errors.Is(err, ErrAvailabilitySourceQualificationNotFound) {
		return AvailabilityEvidenceReview{
			Outcome:    AvailabilityEvidenceReviewIneligible,
			ReasonCode: "availability.source_not_qualified",
		}, nil
	}
	if err != nil {
		return AvailabilityEvidenceReview{}, err
	}
	if err := qualification.Validate(); err != nil {
		return AvailabilityEvidenceReview{}, errors.Join(ErrAvailabilitySourceQualificationDenied, err)
	}
	if qualification.Status == AvailabilitySourceQualificationRevoked ||
		(qualification.RevokedAt != nil && !qualification.RevokedAt.After(now)) {
		return AvailabilityEvidenceReview{
			Outcome:    AvailabilityEvidenceReviewRevoked,
			ReasonCode: "availability.source_qualification_revoked",
		}, nil
	}
	if qualification.ReviewRuleVersion != r.ruleVersion ||
		qualification.FieldPath != evidence.FieldPath ||
		qualification.ParserVersion != evidence.ParserVersion ||
		!availabilitySourceBindingMatches(qualification, binding) ||
		qualification.DeclaredTimezoneOrOffset != evidence.SourceTimezoneOrOffset ||
		!containsAvailabilityPrecision(qualification.AllowedPrecisions, evidence.Precision) {
		return AvailabilityEvidenceReview{
			Outcome:    AvailabilityEvidenceReviewIneligible,
			ReasonCode: "availability.source_binding_mismatch",
		}, nil
	}
	if evidence.ParseStatus != domain.AvailabilityTimeParsed ||
		evidence.CandidateAvailableAt == nil ||
		(evidence.EvidenceRef == nil && evidence.EvidenceHash == nil) {
		return AvailabilityEvidenceReview{
			Outcome:    AvailabilityEvidenceReviewIneligible,
			ReasonCode: "availability.parsed_not_verified",
		}, nil
	}
	return AvailabilityEvidenceReview{
		Outcome:    AvailabilityEvidenceReviewEligible,
		ReasonCode: "availability.source_semantics_verified",
	}, nil
}

func validateAvailabilityEvidenceSourceBinding(
	binding AvailabilityEvidenceSourceBinding,
	requireEvidenceID bool,
) error {
	if requireEvidenceID && !validAvailabilityLongRef(binding.EvidenceID, 128) {
		return errors.New("evidence binding evidence id is invalid")
	}
	if binding.EvidenceID != "" && !validAvailabilityLongRef(binding.EvidenceID, 128) {
		return errors.New("evidence binding evidence id is invalid")
	}
	for name, value := range map[string]string{
		"source_id":                      binding.SourceID,
		"endpoint_id":                    binding.EndpointID,
		"source_policy_version":          binding.SourcePolicyVersionID,
		"provider_output_policy_ref":     binding.ProviderOutputPolicyRef,
		"provider_output_policy_version": binding.ProviderOutputPolicyVersionID,
	} {
		if !validAvailabilityLongRef(value, 128) {
			return fmt.Errorf("evidence binding %s is invalid", name)
		}
	}
	if !validAvailabilityLongRef(binding.EndpointPolicyVersionID, 128) {
		return errors.New("evidence binding endpoint policy version is invalid")
	}
	if binding.ProviderCode == "" {
		if binding.ProviderOutputPolicyRef != "" || binding.ProviderOutputPolicyVersionID != "" {
			return errors.New("evidence binding provider policy requires a provider code")
		}
	} else if !validAvailabilityLongRef(binding.ProviderCode, 128) ||
		binding.ProviderOutputPolicyRef == "" || binding.ProviderOutputPolicyVersionID == "" {
		return errors.New("evidence binding provider policy is invalid")
	}
	return nil
}

func availabilitySourceBindingMatches(
	qualification AvailabilitySourceQualification,
	binding AvailabilityEvidenceSourceBinding,
) bool {
	return qualification.SourceID == binding.SourceID &&
		qualification.EndpointID == binding.EndpointID &&
		qualification.ProviderCode == binding.ProviderCode &&
		qualification.SourcePolicyVersionID == binding.SourcePolicyVersionID &&
		qualification.EndpointPolicyVersionID == binding.EndpointPolicyVersionID &&
		qualification.ProviderOutputPolicyRef == binding.ProviderOutputPolicyRef &&
		qualification.ProviderOutputPolicyVersionID == binding.ProviderOutputPolicyVersionID
}

func qualificationActiveAt(qualification AvailabilitySourceQualification, now time.Time) bool {
	return qualification.Status == AvailabilitySourceQualificationQualified &&
		!qualification.EffectiveAt.After(now) &&
		(qualification.RevokedAt == nil || qualification.RevokedAt.After(now))
}

func containsAvailabilityPrecision(
	values []domain.AvailabilityTimePrecision,
	target domain.AvailabilityTimePrecision,
) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validAvailabilityLongRef(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maximum
}

func validLowerSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validQualificationUTC(name string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s is required", name)
	}
	_, offset := value.Zone()
	if offset != 0 || value.Location() != time.UTC {
		return fmt.Errorf("%s must be UTC", name)
	}
	return nil
}

// sortedQualificationRefs is used by immutable catalog implementations to keep
// qualification selection deterministic when several versions share a binding.
func sortedQualificationRefs(values []AvailabilitySourceQualification) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].EffectiveAt.Equal(values[j].EffectiveAt) {
			return values[i].QualificationRef < values[j].QualificationRef
		}
		return values[i].EffectiveAt.Before(values[j].EffectiveAt)
	})
}
