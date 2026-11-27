package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrAvailabilityEvidencePolicyUnavailable = errors.New("news.available_at.policy_unavailable")
	ErrAvailabilityEvidencePolicyDenied      = errors.New("news.available_at.policy_denied")
	ErrAvailabilityEvidenceWriteUnavailable  = errors.New("news.available_at.write_unavailable")
	ErrAvailabilityEvidenceIDConflict        = errors.New("news.available_at.evidence_id_conflict")
)

// AvailabilityEvidenceWriteTransaction 要求策略锁定与 Evidence 写入共用同一数据库事务。
type AvailabilityEvidenceWriteTransaction interface {
	CurrentIngestPolicyIdentityTransaction
	AppendAvailabilityEvidence(context.Context, domain.AvailabilityEvidence, domain.Observation, domain.NewsItemVersion) error
}

// RecordAvailabilityEvidenceRequest 是 Adapter 解析后的单字段时间候选，不接受调用方提供授权结论。
type RecordAvailabilityEvidenceRequest struct {
	Observation   domain.Observation
	Version       domain.NewsItemVersion
	FieldPath     string
	Candidate     domain.AvailabilityTimeCandidate
	ParserVersion string
	EvidenceRef   *string
	EvidenceHash  *string
	// EvidenceID 只能是从证据语义推导出的确定性 ID；调用方不能自报身份。
	// 为空时保留原有用例的随机 ID 语义，仅供非 G4 直接调用方使用。
	EvidenceID string
}

// RecordAvailabilityEvidence 仅在 Source、Endpoint、Provider 均明确授权目标字段持久化时追加证据。
// 此用例不认定字段语义等同首次公开可获取时间；PIT Decision 仍是独立准入门禁。
type RecordAvailabilityEvidence struct {
	repository IdentityRepository
	sources    SourceCatalog
	providers  ProviderOutputPolicyResolver
	newID      func() string
	now        func() time.Time
}

func NewRecordAvailabilityEvidence(
	repository IdentityRepository,
	sources SourceCatalog,
	providers ProviderOutputPolicyResolver,
	newID func() string,
) (*RecordAvailabilityEvidence, error) {
	if repository == nil || sources == nil || newID == nil {
		return nil, ErrAvailabilityEvidenceWriteUnavailable
	}
	return &RecordAvailabilityEvidence{
		repository: repository, sources: sources, providers: providers, newID: newID, now: time.Now,
	}, nil
}

func (u *RecordAvailabilityEvidence) Execute(ctx context.Context, request RecordAvailabilityEvidenceRequest) (domain.AvailabilityEvidence, error) {
	if u == nil || u.repository == nil || u.sources == nil || u.newID == nil || u.now == nil || ctx == nil {
		return domain.AvailabilityEvidence{}, ErrInvalidRequest
	}
	observation, version := request.Observation, request.Version
	if err := ctx.Err(); err != nil {
		return domain.AvailabilityEvidence{}, err
	}
	if observation.ID == "" || observation.NewsID == "" || observation.SourceID == "" ||
		observation.EndpointID == "" || observation.SourcePolicyVersionID == "" ||
		observation.RetentionUntil.IsZero() || observation.PayloadPurgedAt != nil ||
		version.ID == "" || version.NewsID != observation.NewsID ||
		version.ObservationID != observation.ID || !version.ObservedAt.Equal(observation.ObservedAt) ||
		!validMetadataEvidenceFieldPath(request.FieldPath) ||
		request.ParserVersion == "" || request.ParserVersion != strings.TrimSpace(request.ParserVersion) {
		return domain.AvailabilityEvidence{}, ErrInvalidRequest
	}

	registration, err := u.sources.LookupEndpoint(ctx, observation.SourceID, observation.EndpointID)
	if err != nil {
		return domain.AvailabilityEvidence{}, errors.Join(ErrAvailabilityEvidencePolicyUnavailable, err)
	}
	if ctx.Err() != nil {
		return domain.AvailabilityEvidence{}, ctx.Err()
	}
	if registration.SourceID != observation.SourceID || registration.EndpointID != observation.EndpointID ||
		!registration.Enabled || registration.AccessMethod == "" ||
		registration.SourcePolicyVersionID != observation.SourcePolicyVersionID ||
		registration.EndpointPolicyVersionID != observation.EndpointPolicyVersionID ||
		(registration.EndpointPolicy == nil) != (observation.EndpointPolicyVersionID == "") {
		return domain.AvailabilityEvidence{}, ErrAvailabilityEvidencePolicyUnavailable
	}

	var providerBinding ProviderOutputPolicyBinding
	if observation.ProviderCode != "" {
		if u.providers == nil {
			return domain.AvailabilityEvidence{}, ErrAvailabilityEvidencePolicyUnavailable
		}
		providerBinding, err = u.providers.ResolveProviderOutputPolicy(ctx, observation.ProviderCode)
		if err != nil || providerBinding.ProviderCode != observation.ProviderCode ||
			providerBinding.PolicyRef != observation.ProviderOutputPolicyRef ||
			providerBinding.PolicyVersion != observation.ProviderOutputPolicyVersionID ||
			providerBinding.PolicyRef == "" || providerBinding.PolicyVersion == "" {
			return domain.AvailabilityEvidence{}, errors.Join(ErrAvailabilityEvidencePolicyUnavailable, err)
		}
	} else if observation.ProviderOutputPolicyRef != "" || observation.ProviderOutputPolicyVersionID != "" {
		return domain.AvailabilityEvidence{}, ErrAvailabilityEvidencePolicyUnavailable
	}

	var persisted domain.AvailabilityEvidence
	err = u.repository.WithinIdentityTransaction(ctx, func(identityTx IdentityTransaction) error {
		tx, ok := identityTx.(AvailabilityEvidenceWriteTransaction)
		if !ok {
			return ErrAvailabilityEvidenceWriteUnavailable
		}
		sourcePolicy, endpointPolicy, providerPolicy, policyErr := tx.LockCurrentIngestPolicies(
			ctx, observation.SourceID, observation.SourcePolicyVersionID,
			observation.EndpointID, observation.EndpointPolicyVersionID,
			providerBinding.PolicyRef, providerBinding.PolicyVersion,
		)
		if policyErr != nil {
			return errors.Join(ErrAvailabilityEvidencePolicyUnavailable, policyErr)
		}
		if (observation.ProviderCode == "") != (providerPolicy == nil) {
			return ErrAvailabilityEvidencePolicyUnavailable
		}

		recordedAt := u.now().UTC().Truncate(time.Microsecond)
		if recordedAt.IsZero() || !observation.RetentionUntil.After(recordedAt) {
			return ErrAvailabilityEvidencePolicyDenied
		}
		policyRequest := domain.CollectionPolicyRequest{
			AccessMethod: registration.AccessMethod,
			Purpose:      domain.PurposeInternalPersonalResearch,
			ContentKind:  domain.ContentMetadata,
			StorageMode:  domain.StoragePersistentMetadata,
			Retention:    time.Second,
			FieldPath:    request.FieldPath,
		}
		decision := domain.EvaluateCandidateCollectionPolicy(providerPolicy, sourcePolicy, endpointPolicy, policyRequest, recordedAt)
		if !decision.Allowed || decision.MaxRetention <= 0 {
			reason := decision.ReasonCode
			if reason == "" {
				reason = "news.policy.metadata_field_denied"
			}
			return errors.Join(ErrAvailabilityEvidencePolicyDenied, errors.New(reason))
		}

		retentionUntil := recordedAt.Add(decision.MaxRetention)
		if observation.RetentionUntil.Before(retentionUntil) {
			retentionUntil = observation.RetentionUntil.UTC()
		}
		evidenceID := strings.TrimSpace(u.newID())
		if request.EvidenceID != "" {
			expectedID := recordAvailabilityEvidenceID(request)
			if request.EvidenceID != expectedID || !validAvailabilityLongRef(request.EvidenceID, 128) {
				return ErrInvalidRequest
			}
			evidenceID = request.EvidenceID
		}
		if !validAvailabilityLongRef(evidenceID, 128) {
			return ErrInvalidRequest
		}
		evidence := domain.AvailabilityEvidence{
			ID: evidenceID, NewsID: version.NewsID, VersionID: version.ID, ObservationID: observation.ID,
			FieldPath: request.FieldPath, RawValue: request.Candidate.RawValue,
			SourceTimezoneOrOffset: request.Candidate.SourceTimezoneOrOffset,
			Precision:              request.Candidate.Precision, ParseStatus: request.Candidate.Status,
			ParseReasonCode:      request.Candidate.ReasonCode,
			CandidateAvailableAt: cloneAvailabilityTime(request.Candidate.CandidateAvailableAt),
			ParserVersion:        request.ParserVersion, EvidenceRef: cloneAvailabilityString(request.EvidenceRef),
			EvidenceHash: cloneAvailabilityString(request.EvidenceHash),
			ObservedAt:   observation.ObservedAt.UTC(), RecordedAt: recordedAt, RetentionUntil: retentionUntil,
		}
		if err := evidence.Validate(); err != nil {
			return errors.Join(ErrInvalidRequest, err)
		}
		if err := tx.AppendAvailabilityEvidence(ctx, evidence, observation, version); err != nil {
			return err
		}
		persisted = evidence
		return nil
	})
	if err != nil {
		return domain.AvailabilityEvidence{}, err
	}
	return persisted, nil
}

func validMetadataEvidenceFieldPath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 255 &&
		!strings.ContainsAny(value, "*/[]\\")
}

func cloneAvailabilityTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}

func cloneAvailabilityString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

// recordAvailabilityEvidenceID binds the evidence identity to the immutable
// semantic fields, never to the write timestamp. Replaying the same collection
// therefore converges to the same ID instead of creating a second evidence row.
func recordAvailabilityEvidenceID(request RecordAvailabilityEvidenceRequest) string {
	candidateAt := ""
	if request.Candidate.CandidateAvailableAt != nil {
		candidateAt = request.Candidate.CandidateAvailableAt.UTC().Format(time.RFC3339Nano)
	}
	payload, err := json.Marshal(struct {
		NewsID        string `json:"news_id"`
		VersionID     string `json:"version_id"`
		ObservationID string `json:"observation_id"`
		ObservedAt    string `json:"observed_at"`
		FieldPath     string `json:"field_path"`
		RawValue      string `json:"raw_value"`
		Timezone      string `json:"timezone"`
		Precision     string `json:"precision"`
		ParseStatus   string `json:"parse_status"`
		CandidateAt   string `json:"candidate_at"`
		ParserVersion string `json:"parser_version"`
		EvidenceRef   string `json:"evidence_ref"`
		EvidenceHash  string `json:"evidence_hash"`
	}{
		NewsID:        request.Version.NewsID,
		VersionID:     request.Version.ID,
		ObservationID: request.Observation.ID,
		ObservedAt:    request.Observation.ObservedAt.UTC().Format(time.RFC3339Nano),
		FieldPath:     request.FieldPath,
		RawValue:      request.Candidate.RawValue,
		Timezone:      request.Candidate.SourceTimezoneOrOffset,
		Precision:     string(request.Candidate.Precision),
		ParseStatus:   string(request.Candidate.Status),
		CandidateAt:   candidateAt,
		ParserVersion: request.ParserVersion,
		EvidenceRef:   availabilityStringValue(request.EvidenceRef),
		EvidenceHash:  availabilityStringValue(request.EvidenceHash),
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return "availability-evidence-" + hex.EncodeToString(digest[:])
}

func availabilityStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
