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
	code string
	search func(context.Context, domain.SearchRequest) ([]domain.Candidate, error)
}

func (p callbackProvider) Code() string { return p.code }
func (p callbackProvider) Search(ctx context.Context, request domain.SearchRequest) ([]domain.Candidate, error) {
	return p.search(ctx, request)
}

func TestSearchCandidatesStopsAfterProviderCancelsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secondCalled := false
	usecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
		callbackProvider{code: "first", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
			cancel()
			return []domain.Candidate{{Title: "must not be returned"}}, nil
		}},
		callbackProvider{code: "second", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
			secondCalled = true
			return nil, nil
		}},
	}})
	result := usecase.Execute(ctx, domain.SearchRequest{Query: "market", Limit: 10})
	if secondCalled {
		t.Fatal("subsequent provider must not be called after request cancellation")
	}
	if result.Status != domain.SearchFailed || result.FailureCode != "context_canceled" {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Outcomes) != 0 || len(result.Candidates) != 0 {
		t.Fatalf("canceled provider result must not be recorded as completed: %+v", result)
	}
}

func TestSearchCandidatesDoesNotCallProviderForCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	usecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
		callbackProvider{code: "first", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
			called = true
			return nil, nil
		}},
	}})
	result := usecase.Execute(ctx, domain.SearchRequest{Query: "market", Limit: 10})
	if called {
		t.Fatal("provider must not be called with an already canceled context")
	}
	if result.Status != domain.SearchFailed || result.FailureCode != "context_canceled" {
		t.Fatalf("result=%+v", result)
	}
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


func TestSearchCandidatesRejectsInvalidProviderSetBeforeCallingAnyProvider(t *testing.T) {
	tests := []struct {
		name      string
		providers func(*bool) []Provider
	}{
		{
			name: "nil provider",
			providers: func(called *bool) []Provider {
				return []Provider{
					callbackProvider{code: "valid", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
						*called = true
						return nil, nil
					}},
					nil,
				}
			},
		},
		{
			name: "empty code",
			providers: func(called *bool) []Provider {
				return []Provider{
					callbackProvider{code: "valid", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
						*called = true
						return nil, nil
					}},
					fakeProvider{code: ""},
				}
			},
		},
		{
			name: "whitespace code",
			providers: func(called *bool) []Provider {
				return []Provider{fakeProvider{code: " invalid "}}
			},
		},
		{
			name: "duplicate code",
			providers: func(called *bool) []Provider {
				return []Provider{
					callbackProvider{code: "duplicate", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
						*called = true
						return nil, nil
					}},
					fakeProvider{code: "duplicate"},
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			result := NewSearchCandidates(StaticProviderSet{Providers: test.providers(&called)}).
				Execute(context.Background(), domain.SearchRequest{Query: "market", Limit: 10})
			if called {
				t.Fatal("invalid ProviderSet must be rejected before any provider request")
			}
			if result.Status != domain.SearchFailed || result.FailureCode != "provider_registry_invalid" {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestSearchCandidatesPreservesEarlierOutcomesWhenLaterProviderCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	usecase := NewSearchCandidates(StaticProviderSet{Providers: []Provider{
		fakeProvider{code: "first", items: []domain.Candidate{{Title: "completed"}}},
		callbackProvider{code: "second", search: func(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
			cancel()
			return []domain.Candidate{{Title: "canceled result"}}, nil
		}},
	}})
	result := usecase.Execute(ctx, domain.SearchRequest{Query: "market", Limit: 10})
	if result.Status != domain.SearchFailed || result.FailureCode != "context_canceled" {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Outcomes) != 1 || result.Outcomes[0].ProviderCode != "first" {
		t.Fatalf("completed outcomes were not preserved: %+v", result.Outcomes)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Title != "completed" {
		t.Fatalf("canceled Provider result must not be returned: %+v", result.Candidates)
	}
}
