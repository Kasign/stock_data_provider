package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type authorizationSnapshotRepositoryStub struct {
	appended []domain.NewsAuthorizationBinding
}

func (s *authorizationSnapshotRepositoryStub) AppendAuthorizationSnapshot(_ context.Context, binding domain.NewsAuthorizationBinding, _ domain.Observation, _ domain.NewsItemVersion) error {
	s.appended = append(s.appended, binding)
	return nil
}

func (s *authorizationSnapshotRepositoryStub) GetAuthorizationSnapshot(context.Context, string) (domain.NewsAuthorizationBinding, error) {
	return domain.NewsAuthorizationBinding{}, ErrAuthorizationSnapshotNotFound
}

func TestRecordAuthorizationSnapshotValidatesAndPersistsBinding(t *testing.T) {
	binding := loadAppAuthorizationBinding(t)
	repository := &authorizationSnapshotRepositoryStub{}
	service, err := NewRecordAuthorizationSnapshot(repository)
	if err != nil {
		t.Fatal(err)
	}
	observation, version := appAuthorizationEvidence()
	if err := service.Execute(context.Background(), binding, observation, version); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(repository.appended) != 1 ||
		repository.appended[0].AuthorizationRef.AuthorizationRef != binding.AuthorizationRef.AuthorizationRef {
		t.Fatalf("persisted authorization binding = %+v", repository.appended)
	}
}

func TestRecordAuthorizationSnapshotRejectsInvalidBindingBeforePersistence(t *testing.T) {
	repository := &authorizationSnapshotRepositoryStub{}
	service, err := NewRecordAuthorizationSnapshot(repository)
	if err != nil {
		t.Fatal(err)
	}
	binding := loadAppAuthorizationBinding(t)
	binding.AuthorizationRef.ResourceRef = "different-resource"
	observation, version := appAuthorizationEvidence()
	if err := service.Execute(context.Background(), binding, observation, version); err != ErrInvalidAuthorizationSnapshot {
		t.Fatalf("Execute() error = %v, want ErrInvalidAuthorizationSnapshot", err)
	}
	if len(repository.appended) != 0 {
		t.Fatalf("invalid authorization binding reached repository: %+v", repository.appended)
	}
}

func loadAppAuthorizationBinding(t *testing.T) domain.NewsAuthorizationBinding {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "整体任务规划", "golden", "NEWS-CTR-002-authorization-binding.golden.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		AuthorizationBinding domain.NewsAuthorizationBinding `json:"authorization_binding"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.AuthorizationBinding
}

func appAuthorizationEvidence() (domain.Observation, domain.NewsItemVersion) {
	observation := domain.Observation{
		ID: "observation-001", NewsID: "news_001", SourceID: "sec-edgar",
		ProviderCode: "tavily", SourcePolicyVersionID: "2.1.0",
		EndpointPolicyVersionID: "1.0.0",
	}
	version := domain.NewsItemVersion{
		ID: "version-001", NewsID: "news_001", VersionNo: 1,
		ObservationID: observation.ID,
	}
	return observation, version
}
