package news

import (
	"context"
	"errors"
	"testing"
	"time"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

type evidenceQueryFake struct {
	record   newsapp.ResearchEvidenceRecord
	err      error
	requests []newsapp.GetEvidenceRequest
}

func (f *evidenceQueryFake) GetEvidence(_ context.Context, request newsapp.GetEvidenceRequest) (newsapp.ResearchEvidenceRecord, error) {
	f.requests = append(f.requests, request)
	return f.record, f.err
}

func (f *evidenceQueryFake) ListEvidence(context.Context, newsapp.ListEvidenceRequest) (newsapp.ResearchEvidencePage, error) {
	return newsapp.ResearchEvidencePage{}, errors.New("not used")
}

type researchPortFake struct {
	command researchapp.AddEvidenceCommand
	calls   int
	err     error
}

func (f *researchPortFake) CreateContext(context.Context, researchapp.CreateContextCommand) (researchapp.ResearchContext, error) {
	return researchapp.ResearchContext{}, errors.New("not used")
}

func (f *researchPortFake) AddEvidence(_ context.Context, command researchapp.AddEvidenceCommand) (researchapp.EvidenceRecord, error) {
	f.calls++
	f.command = command
	return researchapp.EvidenceRecord{}, f.err
}

func (f *researchPortFake) GetContext(context.Context, researchapp.ContextReadRequest) (researchapp.ResearchContext, error) {
	return researchapp.ResearchContext{}, errors.New("not used")
}

func (f *researchPortFake) GetEvidence(context.Context, researchapp.EvidenceRef) (researchapp.EvidenceRecord, error) {
	return researchapp.EvidenceRecord{}, errors.New("not used")
}

func validNewsRecord(asOf time.Time) newsapp.ResearchEvidenceRecord {
	available, observed := asOf.Add(-time.Hour), asOf.Add(-30*time.Minute)
	return newsapp.ResearchEvidenceRecord{
		NewsID: "news_001", ObservationID: "obs_001", VersionID: "version_001", VersionNo: 1,
		ContentFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SourceRef:          "source_001", QualityStatus: newsapp.EvidenceQualityValid,
		BusinessQualityStatus: newsapp.EvidenceBusinessQualityValid,
		AvailableAt:           &available, SystemObservedAt: observed,
		ContentLevel: newsapp.EvidenceContentSummary, AuthorizationRef: "auth_001",
		AuthorizationStatus: "allowed", SystemAsOf: asOf,
		RelationStatus: newsapp.EvidenceRelationNotProvided, RelationRefs: []string{},
	}
}

func TestImporterMapsAuthorizedPITNewsRecordToResearchEvidence(t *testing.T) {
	asOf := time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)
	query, research := &evidenceQueryFake{record: validNewsRecord(asOf)}, &researchPortFake{}
	importer, err := NewImporter(query, research)
	if err != nil {
		t.Fatal(err)
	}
	_, err = importer.ImportEvidence(context.Background(), ImportRequest{
		CallerRef: "service:researchflow", PurposeCode: "research_analysis",
		RequestedContentLevel: "full", SystemAsOf: asOf, NewsID: "news_001", VersionID: "version_001",
	})
	if err != nil {
		t.Fatalf("ImportEvidence() error = %v", err)
	}
	if len(query.requests) != 1 || research.calls != 1 {
		t.Fatalf("unexpected calls: query=%d research=%d", len(query.requests), research.calls)
	}
	got := research.command.Draft
	if got.EvidenceType != "reported_fact" || got.SourceOwner != "news" ||
		got.SourceRef != "source_001" || got.SourceLocator == nil || *got.SourceLocator != "news_001" ||
		got.SourceVersion == nil || *got.SourceVersion != "version_001" ||
		got.AvailableAt == nil || got.SystemAsOf != asOf || got.AuthorizationRef != "auth_001" ||
		got.SubjectRefs == nil || len(got.SubjectRefs) != 0 || got.DerivedFromRefs == nil || len(got.DerivedFromRefs) != 0 {
		t.Fatalf("unexpected Research Evidence mapping: %+v", got)
	}
	if research.command.CallerRef != "service:researchflow" || research.command.PurposeCode != "research_analysis" {
		t.Fatalf("trusted caller/purpose not propagated: %+v", research.command)
	}
}

func TestImporterRejectsMissingOrFutureAvailableAtWithoutPersisting(t *testing.T) {
	asOf := time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*newsapp.ResearchEvidenceRecord)
		want   error
	}{
		{"missing available_at", func(r *newsapp.ResearchEvidenceRecord) { r.AvailableAt = nil }, ErrEvidenceNotPITSafe},
		{"future available_at", func(r *newsapp.ResearchEvidenceRecord) { future := asOf.Add(time.Second); r.AvailableAt = &future }, ErrEvidenceNotPITSafe},
		{"denied", func(r *newsapp.ResearchEvidenceRecord) { r.AuthorizationStatus = "denied" }, ErrEvidenceNotAuthorized},
		{"wrong cutoff", func(r *newsapp.ResearchEvidenceRecord) { r.SystemAsOf = asOf.Add(-time.Second) }, ErrEvidenceNotPITSafe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := validNewsRecord(asOf)
			tt.mutate(&record)
			query, research := &evidenceQueryFake{record: record}, &researchPortFake{}
			importer, _ := NewImporter(query, research)
			_, err := importer.ImportEvidence(context.Background(), ImportRequest{
				CallerRef: "service:researchflow", PurposeCode: "research_analysis",
				RequestedContentLevel: "summary", SystemAsOf: asOf, NewsID: "news_001", VersionID: "version_001",
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if research.calls != 0 {
				t.Fatal("invalid evidence must not be persisted")
			}
		})
	}
}

func TestAuthorizerRechecksCurrentNewsAuthorizationAndExactVersion(t *testing.T) {
	asOf := time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)
	query := &evidenceQueryFake{record: validNewsRecord(asOf)}
	authorizer, err := NewAuthorizer(query)
	if err != nil {
		t.Fatal(err)
	}
	version := "version_001"
	available := asOf.Add(-time.Hour)
	newsID := "news_001"
	value := researchevidence.Evidence{
		EvidenceID: "ev_001", SourceOwner: researchevidence.SourceNews, SourceRef: "source_001",
		SourceVersion: &version, SourceLocator: &newsID, AvailableAt: &available,
		SystemObservedAt: asOf.Add(-30 * time.Minute), SystemAsOf: asOf,
		ContentLevel: researchevidence.ContentSummary, AuthorizationRef: "auth_historical",
	}
	if err := authorizer.AuthorizeEvidence(context.Background(), "service:researchflow", researchevidence.PurposeResearchAnalysis, value); err != nil {
		t.Fatalf("valid current authorization rejected: %v", err)
	}
	if len(query.requests) != 1 || query.requests[0].NewsID != "news_001" ||
		query.requests[0].VersionID != version || query.requests[0].CallerRef != "service:researchflow" {
		t.Fatalf("authorization query not bound to trusted scope: %+v", query.requests)
	}
	query.record.AuthorizationStatus = "denied"
	if err := authorizer.AuthorizeEvidence(context.Background(), "service:researchflow", researchevidence.PurposeResearchAnalysis, value); err == nil {
		t.Fatal("denied current authorization must fail closed")
	}
}
