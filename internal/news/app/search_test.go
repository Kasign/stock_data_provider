package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type fakeProvider struct {
	code  string
	items []domain.Candidate
	err   error
}

func (p fakeProvider) Code() string { return p.code }

func (p fakeProvider) Search(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
	return p.items, p.err
}

func TestSearchCandidatesPreservesPartialProviderFailure(t *testing.T) {
	usecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
		fakeProvider{code: "one", items: []domain.Candidate{{Title: "headline"}}},
		fakeProvider{code: "two", err: errors.New("upstream unavailable")},
	}})
	result := usecase.Execute(context.Background(), domain.SearchRequest{Query: "market", Limit: 10, QueryDimension: "market"})
	if result.Status != domain.SearchPartial {
		t.Fatalf("status=%s", result.Status)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].ProviderCode != "one" {
		t.Fatalf("candidates=%+v", result.Candidates)
	}
	if len(result.Outcomes) != 2 || result.Outcomes[1].Status != domain.ProviderFailed {
		t.Fatalf("outcomes=%+v", result.Outcomes)
	}
	if result.Outcomes[0].QueryDimension != "market" || result.Outcomes[1].CandidateCount != 0 {
		t.Fatalf("provider outcome detail mismatch: %+v", result.Outcomes)
	}
	if result.Outcomes[1].Retryable {
		t.Fatal("unknown provider errors must not be marked retryable")
	}
}

func TestSearchCandidatesNormalizesProviderCandidates(t *testing.T) {
	publishedAt := time.Date(2026, 11, 4, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	usecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
		fakeProvider{code: " provider-a ", items: []domain.Candidate{{
			Title: "  标题  ", Summary: " 摘要  内容 ", RawURL: " https://publisher.example/a ",
			PublisherName: " 媒体 ", PublishedAt: &publishedAt,
		}}},
	}})
	result := usecase.Execute(context.Background(), domain.SearchRequest{Query: "market", Limit: 10, QueryDimension: "news"})
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates=%+v", result.Candidates)
	}
	got := result.Candidates[0]
	if got.ProviderCode != "provider-a" || got.Title != "标题" || got.Summary != "摘要 内容" {
		t.Fatalf("candidate=%+v", got)
	}
	if got.QueryDimension != "news" || got.ObservedAt.IsZero() || got.PublishedAt.Location() != time.UTC {
		t.Fatalf("candidate context/time=%+v", got)
	}
}

func TestSearchCandidatesReturnsEmptyWhenAllProvidersReturnEmpty(t *testing.T) {
	usecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
		fakeProvider{code: "one"},
		fakeProvider{code: "two"},
	}})
	result := usecase.Execute(context.Background(), domain.SearchRequest{Query: "market", Limit: 10})
	if result.Status != domain.SearchEmpty {
		t.Fatalf("status=%s", result.Status)
	}
}

func TestSearchCandidatesReturnsPartialForSuccessAndSkipped(t *testing.T) {
	usecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
		fakeProvider{code: "one", items: []domain.Candidate{{Title: "headline"}}},
		fakeProvider{code: "two", err: &ProviderError{Code: "provider_not_configured", Skipped: true}},
	}})
	result := usecase.Execute(context.Background(), domain.SearchRequest{Query: "market", Limit: 10})
	if result.Status != domain.SearchPartial {
		t.Fatalf("status=%s", result.Status)
	}
	if result.Outcomes[1].Status != domain.ProviderSkipped {
		t.Fatalf("outcome=%+v", result.Outcomes[1])
	}
}

func TestSearchCandidatesSkipsWhenNoProviderConfigured(t *testing.T) {
	result := NewSearchCandidates(StaticProviderSet{}).Execute(context.Background(), domain.SearchRequest{Query: "market", Limit: 10})
	if result.Status != domain.SearchSkipped {
		t.Fatalf("status=%s", result.Status)
	}
}

func TestSearchCandidatesRejectsInvalidRequest(t *testing.T) {
	result := NewSearchCandidates(StaticProviderSet{}).Execute(context.Background(), domain.SearchRequest{Query: " ", Limit: 10})
	if result.Status != domain.SearchFailed || result.FailureCode != "invalid_request" {
		t.Fatalf("result=%+v", result)
	}
}
