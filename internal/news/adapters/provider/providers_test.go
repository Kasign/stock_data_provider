package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type roundTripFunc func(*http.Request) (*http.Response,error)
func (f roundTripFunc) Do(r *http.Request) (*http.Response,error) { return f(r) }
func response(body string) *http.Response { return &http.Response{StatusCode:200,Body:io.NopCloser(strings.NewReader(body)),Header:make(http.Header)} }

func TestBochaPreservesUnknownPublicationTimeAndProvider(t *testing.T) {
	client:=roundTripFunc(func(req *http.Request)(*http.Response,error){
		if req.Method!=http.MethodPost { t.Fatalf("method=%s",req.Method) }
		if req.Header.Get("Authorization")!="Bearer test-secret" { t.Fatal("missing auth header") }
		return response(`{"code":0,"data":{"webPages":{"value":[{"name":"标题","url":"https://example.com/a","siteName":"媒体","summary":"摘要"}]}}}`),nil
	})
	p,err:=NewBocha("https://api.example.test/v1/web-search","test-secret",client);if err!=nil{t.Fatal(err)}
	items,err:=p.Search(context.Background(),domain.SearchRequest{Query:"测试",Limit:10});if err!=nil{t.Fatal(err)}
	if len(items)!=1 {t.Fatalf("items=%d",len(items))}
	got:=items[0]
	if got.ProviderCode!=BochaCode || got.PublisherName!="媒体" || got.PublishedAtRaw!="" || got.PublishedAt!=nil {t.Fatalf("candidate=%+v",got)}
	if got.PublisherDomain!="example.com" {t.Fatalf("domain=%s",got.PublisherDomain)}
}

func TestBraveUsesSubscriptionHeaderAndKeepsPageAgeSeparateFromPublishedAt(t *testing.T) {
	client:=roundTripFunc(func(req *http.Request)(*http.Response,error){
		if req.Header.Get("X-Subscription-Token")!="secret" {t.Fatal("missing brave token")}
		return response(`{"results":[{"title":"标题","url":"https://news.example/a","description":"摘要","page_age":"2026-11-05T01:00:00Z"}]}`),nil
	})
	p,err:=NewBrave("https://api.example.test/news","secret",client);if err!=nil{t.Fatal(err)}
	items,err:=p.Search(context.Background(),domain.SearchRequest{Query:"测试",Limit:3});if err!=nil{t.Fatal(err)}
	if items[0].PublishedAtRaw!="" {t.Fatalf("page_age must not be publication time: %q",items[0].PublishedAtRaw)}
}

func TestProviderRejectsCredentialURLAndInvalidEndpoint(t *testing.T) {
	client:=roundTripFunc(func(req *http.Request)(*http.Response,error){return response("{}"),nil})
	if _,err:=NewBocha("https://user:pass@example.com","x",client);err==nil{t.Fatal("expected credential URL rejection")}
	if _,err:=NewSearXNG("file:///tmp",client);err==nil{t.Fatal("expected scheme rejection")}
}
