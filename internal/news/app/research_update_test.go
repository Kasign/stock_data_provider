package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type memoryResearchRepository struct {
	observations map[string]domain.Observation
	versions     map[string][]domain.NewsItemVersion
	relations    map[string][]domain.RelationAssertion
	evidence     map[string][][]domain.RelationEvidence
	failAppend   bool
}

type memoryResearchTransaction struct {
	repository *memoryResearchRepository
}

func (r *memoryResearchRepository) WithinResearchUpdateTransaction(ctx context.Context, fn func(ResearchUpdateTransaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	working := &memoryResearchRepository{
		observations: map[string]domain.Observation{},
		versions:     map[string][]domain.NewsItemVersion{},
		relations:    map[string][]domain.RelationAssertion{},
		evidence:     map[string][][]domain.RelationEvidence{},
		failAppend:   r.failAppend,
	}
	for key, value := range r.observations {
		working.observations[key] = value
	}
	for key, values := range r.versions {
		working.versions[key] = append([]domain.NewsItemVersion(nil), values...)
	}
	for key, values := range r.relations {
		working.relations[key] = append([]domain.RelationAssertion(nil), values...)
	}
	for key, values := range r.evidence {
		working.evidence[key] = append([][]domain.RelationEvidence(nil), values...)
	}
	if err := fn(memoryResearchTransaction{repository: working}); err != nil {
		return err
	}
	*r = *working
	return nil
}

func (tx memoryResearchTransaction) LatestNewsItemVersion(_ context.Context, newsID string) (domain.NewsItemVersion, bool, error) {
	versions := tx.repository.versions[newsID]
	if len(versions) == 0 {
		return domain.NewsItemVersion{}, false, nil
	}
	return versions[len(versions)-1], true, nil
}

func (tx memoryResearchTransaction) NewsItemVersionByID(_ context.Context, versionID string) (domain.NewsItemVersion, bool, error) {
	for _, versions := range tx.repository.versions {
		for _, version := range versions {
			if version.ID == versionID {
				return version, true, nil
			}
		}
	}
	return domain.NewsItemVersion{}, false, nil
}

func (tx memoryResearchTransaction) ObservationByID(_ context.Context, observationID string) (domain.Observation, bool, error) {
	observation, exists := tx.repository.observations[observationID]
	return observation, exists, nil
}

func (tx memoryResearchTransaction) AppendNewsItemVersion(_ context.Context, version domain.NewsItemVersion) error {
	if tx.repository.failAppend {
		return errors.New("append version failed")
	}
	versions := tx.repository.versions[version.NewsID]
	if len(versions)+1 != version.VersionNo {
		return ErrVersionHistoryInvalid
	}
	for _, existing := range versions {
		if existing.Fingerprint == version.Fingerprint {
			return ErrVersionHistoryInvalid
		}
	}
	tx.repository.versions[version.NewsID] = append(versions, version)
	return nil
}

func (tx memoryResearchTransaction) ListRelationAssertions(_ context.Context, newsID string) ([]domain.RelationAssertion, error) {
	return append([]domain.RelationAssertion(nil), tx.repository.relations[newsID]...), nil
}

func (tx memoryResearchTransaction) AppendRelationAssertion(_ context.Context, assertion domain.RelationAssertion, evidence []domain.RelationEvidence) error {
	if tx.repository.failAppend {
		return errors.New("append relation failed")
	}
	for _, existing := range tx.repository.relations[assertion.NewsID] {
		if existing.ID == assertion.ID {
			return domain.ErrRelationHistoryConflict
		}
		if existing.SupersedesRelationID != "" && existing.SupersedesRelationID == assertion.SupersedesRelationID && assertion.SupersedesRelationID != "" {
			return domain.ErrRelationHistoryConflict
		}
	}
	tx.repository.relations[assertion.NewsID] = append(tx.repository.relations[assertion.NewsID], assertion)
	tx.repository.evidence[assertion.NewsID] = append(tx.repository.evidence[assertion.NewsID], append([]domain.RelationEvidence(nil), evidence...))
	return nil
}

func newResearchUseCase(r *memoryResearchRepository) *ApplyResearchUpdate {
	if r.observations == nil {
		r.observations = map[string]domain.Observation{}
	}
	for _, id := range []string{"obs-1", "obs-2", "obs-3"} {
		r.observations[id] = domain.Observation{ID: id, NewsID: "news-1"}
	}
	u := NewApplyResearchUpdate(r, func() string { return "version-next" })
	u.now = func() time.Time { return time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC) }
	return u
}

func baseResearchRequest() ResearchUpdateRequest {
	return ResearchUpdateRequest{
		NewsID: "news-1", ObservationID: "obs-1",
		Candidate: domain.Candidate{Title: "标题", PublisherName: "媒体"},
		Content:   domain.ContentLayers{SourceSummary: "来源摘要", SourceSummaryOrigin: domain.ContentOriginFeedSummary},
	}
}

func TestApplyResearchUpdateUsesPersistedObservationURLForFingerprint(t *testing.T) {
	repository := &memoryResearchRepository{
		versions: map[string][]domain.NewsItemVersion{},
		observations: map[string]domain.Observation{
			"obs-1": {
				ID: "obs-1", NewsID: "news-1",
				Candidate: domain.Candidate{
					RawURL:       "https://publisher.example/raw?tracking=1",
					CanonicalURL: "https://publisher.example/article",
				},
			},
		},
	}
	request := baseResearchRequest()
	request.Candidate.RawURL = "https://attacker.example/forged"
	request.Candidate.CanonicalURL = "https://publisher.example/forged-canonical"
	result, err := newResearchUseCase(repository).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidate := request.Candidate
	expectedCandidate.RawURL = "https://publisher.example/raw?tracking=1"
	expectedCandidate.CanonicalURL = "https://publisher.example/article"
	expected := domain.BuildVersionFingerprint(expectedCandidate, request.Content)
	if result.Version.Fingerprint != expected {
		t.Fatalf("fingerprint did not use persisted observation URL: got=%s want=%s", result.Version.Fingerprint, expected)
	}
}

func TestApplyResearchUpdateDoesNotTrustCanonicalURLWhenObservationHasNone(t *testing.T) {
	repository := &memoryResearchRepository{
		versions: map[string][]domain.NewsItemVersion{},
		observations: map[string]domain.Observation{
			"obs-1": {
				ID: "obs-1", NewsID: "news-1",
				Candidate: domain.Candidate{RawURL: "https://publisher.example/article"},
			},
		},
	}
	request := baseResearchRequest()
	request.Candidate.RawURL = "https://attacker.example/forged"
	request.Candidate.CanonicalURL = "https://publisher.example/forged-canonical"
	result, err := newResearchUseCase(repository).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	expectedCandidate := request.Candidate
	expectedCandidate.RawURL = "https://publisher.example/article"
	expectedCandidate.CanonicalURL = ""
	expected := domain.BuildVersionFingerprint(expectedCandidate, request.Content)
	if result.Version.Fingerprint != expected {
		t.Fatalf("fingerprint used caller URL instead of observation raw URL: got=%s want=%s", result.Version.Fingerprint, expected)
	}
}

func TestApplyResearchUpdateAppendsOnlyChangedVersions(t *testing.T) {
	repository := &memoryResearchRepository{versions: map[string][]domain.NewsItemVersion{}}
	useCase := newResearchUseCase(repository)
	first, err := useCase.Execute(context.Background(), baseResearchRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !first.VersionCreated || first.Version.VersionNo != 1 {
		t.Fatalf("first=%+v", first)
	}

	retry := baseResearchRequest()
	retry.ObservationID = "obs-2"
	retry.Candidate.ObservedAt = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	second, err := useCase.Execute(context.Background(), retry)
	if err != nil {
		t.Fatal(err)
	}
	if second.VersionCreated || second.Version.ID != first.Version.ID || len(repository.versions["news-1"]) != 1 {
		t.Fatalf("second=%+v versions=%+v", second, repository.versions)
	}

	changed := baseResearchRequest()
	changed.ObservationID = "obs-3"
	changed.Content.SourceSummary = "修订摘要"
	third, err := useCase.Execute(context.Background(), changed)
	if err != nil {
		t.Fatal(err)
	}
	if !third.VersionCreated || third.Version.VersionNo != 2 || len(repository.versions["news-1"]) != 2 {
		t.Fatalf("third=%+v", third)
	}
}

func TestApplyResearchUpdateAddsConfirmedSecurityRelationWithEvidence(t *testing.T) {
	repository := &memoryResearchRepository{versions: map[string][]domain.NewsItemVersion{}}
	request := baseResearchRequest()
	request.Relations = []domain.RelationAssertion{{
		ID: "relation-1", SubjectType: domain.RelationSubjectSecurity,
		Security: &domain.SecurityRelationSubject{Market: "XSHG", Symbol: "600000", Name: "浦发银行"},
		Kind:     domain.RelationMentions, Method: domain.RelationSourceExplicit, Status: domain.RelationConfirmed,
	}}
	request.Evidence = map[string][]domain.RelationEvidence{
		"relation-1": {{Type: domain.RelationEvidenceSourceField, FieldPath: "related_securities"}},
	}
	result, err := newResearchUseCase(repository).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CurrentRelations) != 1 || result.CurrentRelations[0].ID != "relation-1" {
		t.Fatalf("result=%+v", result)
	}
	if len(repository.evidence["news-1"]) != 1 {
		t.Fatalf("evidence=%+v", repository.evidence)
	}
}

func TestApplyResearchUpdateModelProposalMustBeManuallySuperseded(t *testing.T) {
	repository := &memoryResearchRepository{versions: map[string][]domain.NewsItemVersion{}}
	request := baseResearchRequest()
	model := domain.RelationAssertion{
		ID: "model-1", SubjectType: domain.RelationSubjectSecurity,
		Security: &domain.SecurityRelationSubject{Market: "XSHG", Symbol: "600000"},
		Kind:     domain.RelationMentions, Method: domain.RelationModel, Status: domain.RelationProposed,
	}
	request.Relations = []domain.RelationAssertion{model}
	request.Evidence = map[string][]domain.RelationEvidence{"model-1": {{Type: domain.RelationEvidenceModelOutput, ModelVersion: "model-v1"}}}
	first, err := newResearchUseCase(repository).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.CurrentRelations) != 0 {
		t.Fatalf("proposed model relation became current: %+v", first.CurrentRelations)
	}

	confirm := baseResearchRequest()
	confirm.ObservationID = "obs-2"
	confirm.Relations = []domain.RelationAssertion{{
		ID: "manual-1", SubjectType: domain.RelationSubjectSecurity,
		Security: &domain.SecurityRelationSubject{Market: "XSHG", Symbol: "600000"},
		Kind:     domain.RelationMentions, Method: domain.RelationManual, Status: domain.RelationConfirmed,
		SupersedesRelationID: "model-1",
	}}
	confirm.Evidence = map[string][]domain.RelationEvidence{"manual-1": {{Type: domain.RelationEvidenceManual}}}
	result, err := newResearchUseCase(repository).Execute(context.Background(), confirm)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CurrentRelations) != 1 || result.CurrentRelations[0].ID != "manual-1" {
		t.Fatalf("result=%+v", result)
	}
}

func TestApplyResearchUpdateRejectsEvidenceFromAnotherNewsItem(t *testing.T) {
	repository := &memoryResearchRepository{
		versions: map[string][]domain.NewsItemVersion{},
		observations: map[string]domain.Observation{
			"foreign-observation": {ID: "foreign-observation", NewsID: "news-foreign"},
		},
	}
	request := baseResearchRequest()
	request.Relations = []domain.RelationAssertion{{
		ID: "relation-foreign", SubjectType: domain.RelationSubjectSecurity,
		Security: &domain.SecurityRelationSubject{Market: "XSHG", Symbol: "600000"},
		Kind:     domain.RelationMentions, Method: domain.RelationSourceExplicit, Status: domain.RelationConfirmed,
	}}
	request.Evidence = map[string][]domain.RelationEvidence{
		"relation-foreign": {{Type: domain.RelationEvidenceSourceField, FieldPath: "related_securities", ObservationID: "foreign-observation"}},
	}
	if _, err := newResearchUseCase(repository).Execute(context.Background(), request); !errors.Is(err, ErrResearchUpdateTransaction) {
		t.Fatalf("expected foreign observation rejection, got %v", err)
	}
	if len(repository.versions["news-1"]) != 0 || len(repository.relations["news-1"]) != 0 {
		t.Fatalf("foreign evidence must rollback all writes: %+v", repository)
	}
}

func TestApplyResearchUpdateRejectsUnknownVersionEvidence(t *testing.T) {
	repository := &memoryResearchRepository{versions: map[string][]domain.NewsItemVersion{}}
	request := baseResearchRequest()
	request.Relations = []domain.RelationAssertion{{
		ID: "relation-missing-version", SubjectType: domain.RelationSubjectSecurity,
		Security: &domain.SecurityRelationSubject{Market: "XSHG", Symbol: "600000"},
		Kind:     domain.RelationMentions, Method: domain.RelationContentMatch, Status: domain.RelationConfirmed,
	}}
	request.Evidence = map[string][]domain.RelationEvidence{
		"relation-missing-version": {{Type: domain.RelationEvidenceContentSpan, SourceVersionID: "missing-version"}},
	}
	if _, err := newResearchUseCase(repository).Execute(context.Background(), request); !errors.Is(err, ErrResearchUpdateTransaction) {
		t.Fatalf("expected missing version rejection, got %v", err)
	}
	if len(repository.versions["news-1"]) != 0 {
		t.Fatalf("invalid evidence must rollback version append: %+v", repository.versions)
	}
}

func TestApplyResearchUpdateRejectsVersionEvidenceFromAnotherNewsItem(t *testing.T) {
	repository := &memoryResearchRepository{
		versions: map[string][]domain.NewsItemVersion{
			"news-foreign": {{ID: "foreign-version", NewsID: "news-foreign", VersionNo: 1, Fingerprint: "fingerprint"}},
		},
	}
	request := baseResearchRequest()
	request.Relations = []domain.RelationAssertion{{
		ID: "relation-foreign-version", SubjectType: domain.RelationSubjectSecurity,
		Security: &domain.SecurityRelationSubject{Market: "XSHG", Symbol: "600000"},
		Kind:     domain.RelationMentions, Method: domain.RelationContentMatch, Status: domain.RelationConfirmed,
	}}
	request.Evidence = map[string][]domain.RelationEvidence{
		"relation-foreign-version": {{Type: domain.RelationEvidenceContentSpan, SourceVersionID: "foreign-version"}},
	}
	if _, err := newResearchUseCase(repository).Execute(context.Background(), request); !errors.Is(err, ErrResearchUpdateTransaction) {
		t.Fatalf("expected foreign version rejection, got %v", err)
	}
	if len(repository.versions["news-1"]) != 0 || len(repository.relations["news-1"]) != 0 {
		t.Fatalf("foreign version evidence must rollback all writes: %+v", repository)
	}
}

func TestApplyResearchUpdateRejectsInvalidRelationAndRollsBackVersion(t *testing.T) {
	repository := &memoryResearchRepository{versions: map[string][]domain.NewsItemVersion{}}
	request := baseResearchRequest()
	request.Relations = []domain.RelationAssertion{{
		ID: "invalid", SubjectType: domain.RelationSubjectSecurity,
		Security: &domain.SecurityRelationSubject{Symbol: "600000"},
		Kind:     domain.RelationMentions, Method: domain.RelationSourceExplicit, Status: domain.RelationConfirmed,
	}}
	request.Evidence = map[string][]domain.RelationEvidence{"invalid": {{Type: domain.RelationEvidenceSourceField, FieldPath: "related_securities"}}}
	_, err := newResearchUseCase(repository).Execute(context.Background(), request)
	if !errors.Is(err, ErrResearchUpdateTransaction) {
		t.Fatalf("err=%v", err)
	}
	if len(repository.versions["news-1"]) != 0 {
		t.Fatalf("partial version persisted: %+v", repository.versions)
	}
}

func TestApplyResearchUpdateRejectsForeignRequestObservation(t *testing.T) {
	repository := &memoryResearchRepository{
		versions: map[string][]domain.NewsItemVersion{},
		observations: map[string]domain.Observation{
			"foreign-observation": {ID: "foreign-observation", NewsID: "news-foreign"},
		},
	}
	request := baseResearchRequest()
	request.ObservationID = "foreign-observation"
	if _, err := newResearchUseCase(repository).Execute(context.Background(), request); !errors.Is(err, ErrResearchUpdateTransaction) {
		t.Fatalf("expected foreign request observation rejection, got %v", err)
	}
	if len(repository.versions["news-1"]) != 0 {
		t.Fatalf("foreign observation must not create a version: %+v", repository.versions)
	}
}

func TestApplyResearchUpdateRejectsUnknownRequestObservation(t *testing.T) {
	repository := &memoryResearchRepository{versions: map[string][]domain.NewsItemVersion{}}
	request := baseResearchRequest()
	request.ObservationID = "missing-observation"
	if _, err := newResearchUseCase(repository).Execute(context.Background(), request); !errors.Is(err, ErrResearchUpdateTransaction) {
		t.Fatalf("expected missing request observation rejection, got %v", err)
	}
	if len(repository.versions["news-1"]) != 0 {
		t.Fatalf("missing observation must not create a version: %+v", repository.versions)
	}
}
