package app

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type fakeProvider struct { code string; items []domain.Candidate; err error }
func (p fakeProvider) Code() string { return p.code }
func (p fakeProvider) Search(context.Context, domain.SearchRequest) ([]domain.Candidate,error) { return p.items,p.err }

func TestSearchCandidatesPreservesPartialProviderFailure(t *testing.T) {
	usecase:=NewSearchCandidates(StaticProviderSet{Providers:[]Provider{
		fakeProvider{code:"one",items:[]domain.Candidate{{Title:"headline"}}},
		fakeProvider{code:"two",err:errors.New("upstream unavailable")},
	}})
	result:=usecase.Execute(context.Background(),domain.SearchRequest{Query:"market",Limit:10,QueryDimension:"market"})
	if result.Status!=domain.SearchPartial { t.Fatalf("status=%s",result.Status) }
	if len(result.Candidates)!=1 || result.Candidates[0].ProviderCode!="one" { t.Fatalf("candidates=%+v",result.Candidates) }
	if len(result.Outcomes)!=2 || result.Outcomes[1].Status!=domain.ProviderFailed { t.Fatalf("outcomes=%+v",result.Outcomes) }
}

func TestSearchCandidatesSkipsWhenNoProviderConfigured(t *testing.T) {
	result:=NewSearchCandidates(StaticProviderSet{}).Execute(context.Background(),domain.SearchRequest{Query:"market",Limit:10})
	if result.Status!=domain.SearchSkipped { t.Fatalf("status=%s",result.Status) }
}

func TestSearchCandidatesRejectsInvalidRequest(t *testing.T) {
	result:=NewSearchCandidates(StaticProviderSet{}).Execute(context.Background(),domain.SearchRequest{Query:" ",Limit:10})
	if result.Status!=domain.SearchFailed { t.Fatalf("status=%s",result.Status) }
}
