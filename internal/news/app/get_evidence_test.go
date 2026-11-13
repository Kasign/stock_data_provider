package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type getEvidencePITRepositoryStub struct {
	record           EvidencePITRecord
	err              error
	afterRead        func()
	afterContentRead func()
}

func (s getEvidencePITRepositoryStub) GetEvidenceVersionAt(context.Context, GetEvidenceRequest) (EvidencePITRecord, error) {
	if s.afterRead != nil {
		s.afterRead()
	}
	record := s.record
	record.Version.Content = domain.ContentLayers{}
	return record, s.err
}

func (s getEvidencePITRepositoryStub) ListEvidenceVersionsAt(context.Context, ListEvidenceRequest) ([]EvidencePITRecord, error) {
	return nil, ErrInvalidEvidenceQuery
}

func (s getEvidencePITRepositoryStub) GetEvidenceContentWithCurrentAuthorization(
	ctx context.Context,
	binding domain.NewsAuthorizationBinding,
	_ domain.Observation,
	_ domain.NewsItemVersion,
	_ time.Time,
	authorize func(context.Context, domain.CurrentAuthorizationPolicySet, domain.Observation) error,
) (domain.ContentLayers, domain.Observation, error) {
	policies := make([]domain.CurrentAuthorizationPolicy, 0, len(binding.PolicySnapshot.Policies))
	for _, item := range binding.PolicySnapshot.Policies {
		policies = append(policies, domain.CurrentAuthorizationPolicy{
			PolicyType: item.PolicyType, PolicyRef: item.PolicyRef, PolicyVersion: item.PolicyVersion,
			Status: domain.AuthorizationApproved, Allows: true,
		})
	}
	current := domain.CurrentAuthorizationPolicySet{EvaluatedAt: time.Now().UTC(), Policies: policies}
	if err := authorize(ctx, current, s.record.Observation); err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
	}
	if s.afterContentRead != nil {
		s.afterContentRead()
	}
	return s.record.Version.Content, s.record.Observation, s.err
}

type getEvidenceAuthorizationSnapshotStub struct {
	binding   domain.NewsAuthorizationBinding
	err       error
	afterRead func()
}

func (s getEvidenceAuthorizationSnapshotStub) AppendAuthorizationSnapshot(context.Context, domain.NewsAuthorizationBinding, domain.Observation, domain.NewsItemVersion) error {
	return nil
}

func (s getEvidenceAuthorizationSnapshotStub) GetAuthorizationSnapshot(context.Context, string) (domain.NewsAuthorizationBinding, error) {
	return s.binding, s.err
}

func (s getEvidenceAuthorizationSnapshotStub) GetLatestAuthorizationSnapshotForEvidence(_ context.Context, caller, resource, purpose string, level domain.AuthorizationContentLevel) (domain.NewsAuthorizationBinding, error) {
	if s.err != nil {
		return domain.NewsAuthorizationBinding{}, s.err
	}
	if s.afterRead != nil {
		s.afterRead()
	}
	if s.binding.CallerRef != caller || s.binding.ResourceRef != resource ||
		s.binding.PurposeCode != purpose || s.binding.RequestedContentLevel != level {
		return domain.NewsAuthorizationBinding{}, ErrInvalidAuthorizationSnapshot
	}
	return s.binding, nil
}

type getEvidenceAuthorizerStub struct {
	err          error
	calls        int
	observation  domain.Observation
	afterExecute func()
}

func (s *getEvidenceAuthorizerStub) ExecuteWithCurrentPolicies(_ context.Context, _ domain.NewsAuthorizationBinding, observation domain.Observation, _ domain.NewsItemVersion, _ string, _ string, _ domain.AuthorizationContentLevel, _ domain.CurrentAuthorizationPolicySet) error {
	s.calls++
	s.observation = observation
	if s.afterExecute != nil {
		s.afterExecute()
	}
	return s.err
}

func validGetEvidenceFixture(now time.Time) (GetEvidenceRequest, EvidencePITRecord, domain.NewsAuthorizationBinding) {
	observedAt := now.Add(-2 * time.Hour).UTC()
	resourceRef, _ := domain.NewsEvidenceResourceRef("news-evidence-1", 1)
	observation := domain.Observation{
		ID: "observation-evidence-1", NewsID: "news-evidence-1", SourceID: "source-evidence-1",
		SourcePolicyVersionID: "source-policy-v1", IdentityStatus: domain.ObservationMatched,
		ObservedAt: observedAt, Candidate: domain.Candidate{
			PublishedAtRaw: "2026-11-09T07:00:00Z",
		},
	}
	version := domain.NewsItemVersion{
		ID: "version-evidence-1", NewsID: "news-evidence-1", VersionNo: 1,
		Fingerprint: strings.Repeat("b", 64), Title: "headline",
		PublisherName: "publisher", PublisherDomain: "publisher.example",
		ObservedAt: observedAt, ObservationID: observation.ID,
		Content: domain.ContentLayers{
			SourceSummary: "source summary",
			Body: &domain.ContentPayload{
				Origin: domain.ContentOriginFeedBody, Format: domain.ContentFormatPlainText,
				Kind: domain.ContentFullText, Text: "full article body",
			},
		},
	}
	request := GetEvidenceRequest{
		CallerRef: "service:research", PurposeCode: EvidencePurposeResearchAnalysis,
		RequestedContentLevel: EvidenceContentSummary, SystemAsOf: now.Add(-time.Hour).UTC(),
		NewsID: version.NewsID, VersionID: version.ID,
	}
	binding := domain.NewsAuthorizationBinding{
		CallerRef: request.CallerRef, ResourceRef: resourceRef,
		PurposeCode:           string(request.PurposeCode),
		RequestedContentLevel: domain.AuthorizationContentSummary,
		ProviderRef:           domain.NullableOpaqueRef{Present: true},
		EndpointRef:           domain.NullableOpaqueRef{Present: true},
		SourceRef:             observation.SourceID,
		AuthorizationRef: domain.NewsAuthorizationRef{
			AuthorizationRef: "authorization-evidence-1", PolicyOwner: "news", SchemaVersion: "1.0.0",
			PolicyVersion: "snapshot-evidence-v1", Decision: domain.AuthorizationDecisionAllowed,
			PurposeCode: string(request.PurposeCode), ContentLevel: domain.AuthorizationContentSummary,
			SubjectRef: request.CallerRef, ResourceRef: resourceRef,
			DecidedAt:  now.Add(-3 * time.Hour).UTC().Format(time.RFC3339Nano),
			ExpiresAt:  domain.NullableDateTime{Present: true},
			Conditions: []string{}, DecisionFingerprint: strings.Repeat("a", 64),
		},
		PolicySnapshot: domain.AuthorizationPolicySnapshot{
			SnapshotRef: "snapshot-evidence-v1", SchemaVersion: "1.0.0",
			Policies: []domain.AuthorizationPolicyVersion{{
				PolicyType: "source", PolicyRef: observation.SourceID, PolicyVersion: observation.SourcePolicyVersionID,
			}},
		},
	}
	return request, EvidencePITRecord{
		SourceRef: observation.SourceID, Observation: observation, Version: version,
	}, binding
}

func TestGetEvidenceAppliesAuthorizationAndContentLevelBeforeReturningDTO(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request, pit, binding := validGetEvidenceFixture(now)
	contentRead := false
	authorizationBeforeContent := false
	authorizer := &getEvidenceAuthorizerStub{afterExecute: func() { authorizationBeforeContent = !contentRead }}
	usecase, err := NewGetEvidence(
		getEvidencePITRepositoryStub{record: pit, afterContentRead: func() { contentRead = true }},
		getEvidenceAuthorizationSnapshotStub{binding: binding},
		authorizer,
	)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if authorizer.calls != 1 || !authorizationBeforeContent || !contentRead ||
		result.Title == nil || *result.Title != "headline" ||
		result.ContentExcerpt == nil || *result.ContentExcerpt != "source summary" {
		t.Fatalf("unexpected authorized evidence result: %+v calls=%d", result, authorizer.calls)
	}
	if result.ContentLevel != EvidenceContentSummary || result.AvailableAt != nil ||
		result.ContentExcerpt == nil || strings.Contains(*result.ContentExcerpt, "full article body") {
		t.Fatalf("content exceeded requested level or invented available_at: %+v", result)
	}
	if result.AuthorizationRef != binding.AuthorizationRef.AuthorizationRef ||
		result.AuthorizationStatus != string(domain.AuthorizationDecisionAllowed) {
		t.Fatalf("authorization reference missing from result: %+v", result)
	}
}

func TestGetEvidenceSuppressesCandidateMetadataAfterRetentionDeadline(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request, pit, binding := validGetEvidenceFixture(now)
	expired := now.Add(-time.Minute)
	pit.Observation.RetentionUntil = expired
	pit.Observation.Candidate = domain.Candidate{
		PublishedAtRaw: "raw-source-time",
		FetchedAt:      func() *time.Time { value := now.Add(-2 * time.Hour); return &value }(),
	}
	authorizer := &getEvidenceAuthorizerStub{}
	usecase, err := NewGetEvidence(
		getEvidencePITRepositoryStub{record: pit},
		getEvidenceAuthorizationSnapshotStub{binding: binding},
		authorizer,
	)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.PublishedAtRaw != nil || result.FetchedAt != nil || result.NormalizedAt != nil {
		t.Fatalf("expired candidate metadata was disclosed: %+v", result)
	}
	if result.Title == nil || result.ContentExcerpt == nil || *result.ContentExcerpt != "source summary" {
		t.Fatalf("separately authorized version evidence should remain available: %+v", result)
	}
}

func TestGetEvidenceKeepsAuthorizedVersionReadableAfterObservationPayloadPurge(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	request, pit, binding := validGetEvidenceFixture(now)
	pit.Observation.RetentionUntil = pit.Observation.ObservedAt.Add(time.Hour)
	purgedAt := pit.Observation.RetentionUntil
	pit.Observation.PayloadPurgedAt = &purgedAt
	pit.Observation.Candidate = domain.Candidate{}

	authorizer := &getEvidenceAuthorizerStub{}
	usecase, err := NewGetEvidence(
		getEvidencePITRepositoryStub{record: pit},
		getEvidenceAuthorizationSnapshotStub{binding: binding},
		authorizer,
	)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() after observation payload purge: %v", err)
	}
	if authorizer.calls != 1 || !reflect.DeepEqual(authorizer.observation.Candidate, domain.Candidate{}) ||
		authorizer.observation.PayloadPurgedAt == nil ||
		!authorizer.observation.PayloadPurgedAt.Equal(purgedAt) {
		t.Fatalf("authorizer did not receive a consistent tombstone: calls=%d observation=%+v", authorizer.calls, authorizer.observation)
	}
	if result.PublishedAtRaw != nil || result.FetchedAt != nil || result.NormalizedAt != nil {
		t.Fatalf("purged observation metadata was disclosed: %+v", result)
	}
	if result.Title == nil || *result.Title != pit.Version.Title ||
		result.ContentExcerpt == nil || *result.ContentExcerpt != "source summary" {
		t.Fatalf("separately authorized immutable version content should remain readable: %+v", result)
	}
}

func TestGetEvidenceRejectsPITRepositoryRecordOutsideRequestedBoundary(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request, pit, binding := validGetEvidenceFixture(now)
	pit.Version.ObservedAt = request.SystemAsOf.Add(time.Second)
	authorizer := &getEvidenceAuthorizerStub{}
	usecase, err := NewGetEvidence(
		getEvidencePITRepositoryStub{record: pit},
		getEvidenceAuthorizationSnapshotStub{binding: binding},
		authorizer,
	)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if !errors.Is(err, ErrInvalidEvidenceQuery) || authorizer.calls != 0 || result.Title != nil {
		t.Fatalf("PIT boundary violation was not rejected: result=%+v err=%v calls=%d", result, err, authorizer.calls)
	}
}

func TestGetEvidenceReturnsNoDTOWhenCurrentAuthorizationDenies(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request, pit, binding := validGetEvidenceFixture(now)
	denied := errors.New("current authorization revoked")
	authorizer := &getEvidenceAuthorizerStub{err: denied}
	contentRead := false
	usecase, err := NewGetEvidence(
		getEvidencePITRepositoryStub{record: pit, afterContentRead: func() { contentRead = true }},
		getEvidenceAuthorizationSnapshotStub{binding: binding},
		authorizer,
	)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if !errors.Is(err, denied) || result.Title != nil || result.ContentExcerpt != nil || contentRead {
		t.Fatalf("denied request disclosed evidence: result=%+v err=%v", result, err)
	}
}

func TestGetEvidenceRejectsSnapshotBoundToDifferentSource(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request, pit, binding := validGetEvidenceFixture(now)
	binding.SourceRef = "another-source"
	authorizer := &getEvidenceAuthorizerStub{}
	usecase, err := NewGetEvidence(
		getEvidencePITRepositoryStub{record: pit},
		getEvidenceAuthorizationSnapshotStub{binding: binding},
		authorizer,
	)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if !errors.Is(err, ErrInvalidAuthorizationSnapshot) || authorizer.calls != 0 || result.Title != nil {
		t.Fatalf("mismatched authorization binding was not rejected: result=%+v err=%v calls=%d", result, err, authorizer.calls)
	}
}

func TestGetEvidenceRejectsInvalidRetentionTimelineFromPITRepository(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request, base, binding := validGetEvidenceFixture(now)
	observedAt := base.Observation.ObservedAt
	cases := []struct {
		name   string
		mutate func(*domain.Observation)
	}{
		{
			name: "retention deadline not after observation",
			mutate: func(observation *domain.Observation) {
				observation.RetentionUntil = observedAt
			},
		},
		{
			name: "purge timestamp before retention deadline",
			mutate: func(observation *domain.Observation) {
				observation.RetentionUntil = observedAt.Add(time.Hour)
				purgedAt := observedAt.Add(30 * time.Minute)
				observation.PayloadPurgedAt = &purgedAt
				observation.Candidate = domain.Candidate{}
			},
		},
		{
			name: "purged payload without known retention deadline",
			mutate: func(observation *domain.Observation) {
				purgedAt := observedAt.Add(time.Hour)
				observation.PayloadPurgedAt = &purgedAt
				observation.Candidate = domain.Candidate{}
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			pit := base
			test.mutate(&pit.Observation)
			authorizer := &getEvidenceAuthorizerStub{}
			usecase, err := NewGetEvidence(
				getEvidencePITRepositoryStub{record: pit},
				getEvidenceAuthorizationSnapshotStub{binding: binding},
				authorizer,
			)
			if err != nil {
				t.Fatal(err)
			}
			usecase.now = func() time.Time { return now }
			if _, err := usecase.Execute(context.Background(), request); !errors.Is(err, ErrInvalidEvidenceQuery) {
				t.Fatalf("Execute() error = %v, want invalid PIT record", err)
			}
			if authorizer.calls != 0 {
				t.Fatalf("invalid retention timeline reached authorizer %d times", authorizer.calls)
			}
		})
	}
}

func TestGetEvidenceStopsAfterContextCancellationAtDependencyBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request, pit, binding := validGetEvidenceFixture(now)
	cases := []struct {
		name     string
		cancelAt string
	}{
		{name: "PIT returned", cancelAt: "pit"},
		{name: "authorization snapshot returned", cancelAt: "snapshot"},
		{name: "current authorization returned", cancelAt: "authorizer"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pitRepo := getEvidencePITRepositoryStub{record: pit}
			snapshotRepo := getEvidenceAuthorizationSnapshotStub{binding: binding}
			if test.cancelAt == "pit" {
				pitRepo.afterRead = cancel
			} else {
				snapshotRepo.afterRead = cancel
			}
			authorizer := &getEvidenceAuthorizerStub{}
			if test.cancelAt == "authorizer" {
				authorizer.afterExecute = cancel
			}
			usecase, err := NewGetEvidence(pitRepo, snapshotRepo, authorizer)
			if err != nil {
				t.Fatal(err)
			}
			usecase.now = func() time.Time { return now }
			if _, err := usecase.Execute(ctx, request); !errors.Is(err, context.Canceled) {
				t.Fatalf("Execute() error = %v, want context.Canceled", err)
			}
			if authorizer.calls != 0 {
				t.Fatalf("canceled request reached authorizer: %d", authorizer.calls)
			}
		})
	}
}

func TestGetEvidenceSuppressesCandidateAtExactRetentionDeadline(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request, pit, binding := validGetEvidenceFixture(now)
	pit.Observation.RetentionUntil = now
	pit.Observation.Candidate = domain.Candidate{
		PublishedAtRaw: "raw-source-time",
	}
	authorizer := &getEvidenceAuthorizerStub{}
	usecase, err := NewGetEvidence(
		getEvidencePITRepositoryStub{record: pit},
		getEvidenceAuthorizationSnapshotStub{binding: binding},
		authorizer,
	)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.PublishedAtRaw != nil || result.FetchedAt != nil || result.NormalizedAt != nil {
		t.Fatalf("candidate metadata was disclosed at the exact retention deadline: %+v", result)
	}
	if authorizer.calls != 1 || result.ContentExcerpt == nil || *result.ContentExcerpt != "source summary" {
		t.Fatalf("authorized version content unexpectedly unavailable: result=%+v calls=%d", result, authorizer.calls)
	}
}
