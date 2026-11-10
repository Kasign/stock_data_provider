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

type callbackProvider struct {
\tcode string
\tsearch func(context.Context, domain.SearchRequest) ([]domain.Candidate, error)
}

func (p callbackProvider) Code() string { return p.code }
func (p callbackProvider) Search(ctx context.Context, request domain.SearchRequest) ([]domain.Candidate, error) {
\treturn p.search(ctx, request)
}

func TestSearchCandidatesStopsAfterProviderCancelsRequest(t *testing.T) {
\tctx, cancel := context.WithCancel(context.Background())
\tdefer cancel()
\tsecondCalled := false
\tusecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
\t\tcallbackProvider{code: "first", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
\t\t\tcancel()
\t\t\treturn []domain.Candidate{{Title: "must not be returned"}}, nil
\t\t}},
\t\tcallbackProvider{code: "second", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
\t\t\tsecondCalled = true
\t\t\treturn nil, nil
\t\t}},
\t}})
\tresult := usecase.Execute(ctx, domain.SearchRequest{Query: "market", Limit: 10})
\tif secondCalled {
\t\tt.Fatal("subsequent provider must not be called after request cancellation")
\t}
\tif result.Status != domain.SearchFailed || result.FailureCode != "context_canceled" {
\t\tt.Fatalf("result=%+v", result)
\t}
\tif len(result.Outcomes) != 0 || len(result.Candidates) != 0 {
\t\tt.Fatalf("canceled provider result must not be recorded as completed: %+v", result)
\t}
}

func TestSearchCandidatesDoesNotCallProviderForCanceledContext(t *testing.T) {
\tctx, cancel := context.WithCancel(context.Background())
\tcancel()
\tcalled := false
\tusecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
\t\tcallbackProvider{code: "first", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
\t\t\tcalled = true
\t\t\treturn nil, nil
\t\t}},
\t}})
\tresult := usecase.Execute(ctx, domain.SearchRequest{Query: "market", Limit: 10})
\tif called {
\t\tt.Fatal("provider must not be called with an already canceled context")
\t}
\tif result.Status != domain.SearchFailed || result.FailureCode != "context_canceled" {
\t\tt.Fatalf("result=%+v", result)
\t}
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
			Title: "  标题  ", ProviderSummary: " 摘要  内容 ", RawURL: " https://publisher.example/a ",
			PublisherName: " 媒体 ", PublishedAt: &publishedAt,
		}}},
	}})
	result := usecase.Execute(context.Background(), domain.SearchRequest{Query: "market", Limit: 10, QueryDimension: "news"})
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates=%+v", result.Candidates)
	}
	got := result.Candidates[0]
	if got.ProviderCode != "provider-a" || got.Title != "标题" || got.ProviderSummary != "摘要 内容" {
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
