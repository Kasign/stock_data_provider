package source

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"example.com/stock-analysis-flow/internal/news/app"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestEndpointTransportUsesExactSourceEndpointBinding(t *testing.T) {
	calls := map[string]int{}
	first := doerFunc(func(request *http.Request) (*http.Response, error) {
		calls["source-a/feed"]++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	second := doerFunc(func(request *http.Request) (*http.Response, error) {
		calls["source-b/feed"]++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	transport, err := NewEndpointTransport(map[EndpointKey]app.HTTPDoer{
		{SourceID: "source-a", EndpointID: "feed"}: first,
		{SourceID: "source-b", EndpointID: "feed"}: second,
	})
	if err != nil {
		t.Fatal(err)
	}

	request, _ := http.NewRequest(http.MethodGet, "https://publisher.example/feed.xml", nil)
	if _, err := transport.Do(context.Background(), "source-b", "feed", request); err != nil {
		t.Fatal(err)
	}
	if calls["source-a/feed"] != 0 || calls["source-b/feed"] != 1 {
		t.Fatalf("unexpected endpoint dispatch: %+v", calls)
	}
}

func TestEndpointTransportDoesNotFallbackForUnboundEndpoint(t *testing.T) {
	calls := 0
	doer := doerFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	transport, err := NewEndpointTransport(map[EndpointKey]app.HTTPDoer{
		{SourceID: "source-a", EndpointID: "feed"}: doer,
	})
	if err != nil {
		t.Fatal(err)
	}

	request, _ := http.NewRequest(http.MethodGet, "https://publisher.example/feed.xml", nil)
	_, err = transport.Do(context.Background(), "source-a", "other-endpoint", request)
	if !errors.Is(err, ErrEndpointTransportNotBound) || calls != 0 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

func TestEndpointTransportCopiesBindingMap(t *testing.T) {
	calls := 0
	first := doerFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	second := doerFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatal("mutated binding must not be observed")
		return nil, nil
	})
	bindings := map[EndpointKey]app.HTTPDoer{
		{SourceID: "source-a", EndpointID: "feed"}: first,
	}
	transport, err := NewEndpointTransport(bindings)
	if err != nil {
		t.Fatal(err)
	}
	bindings[EndpointKey{SourceID: "source-a", EndpointID: "feed"}] = second

	request, _ := http.NewRequest(http.MethodGet, "https://publisher.example/feed.xml", nil)
	if _, err := transport.Do(context.Background(), "source-a", "feed", request); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestEndpointTransportUsesSuppliedContext(t *testing.T) {
	type contextKey string
	expected := context.WithValue(context.Background(), contextKey("request"), "collect-1")
	doer := doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Context().Value(contextKey("request")) != "collect-1" {
			t.Fatal("request context was not propagated")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	transport, err := NewEndpointTransport(map[EndpointKey]app.HTTPDoer{
		{SourceID: "source-a", EndpointID: "feed"}: doer,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://publisher.example/feed.xml", nil)
	if _, err := transport.Do(expected, "source-a", "feed", request); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointTransportRejectsInvalidBindings(t *testing.T) {
	valid := doerFunc(func(*http.Request) (*http.Response, error) { return nil, nil })
	for _, bindings := range []map[EndpointKey]app.HTTPDoer{
		nil,
		{},
		{{SourceID: " ", EndpointID: "feed"}: valid},
		{{SourceID: "source-a", EndpointID: " feed"}: valid},
		{{SourceID: "source-a", EndpointID: "feed"}: nil},
	} {
		if _, err := NewEndpointTransport(bindings); !errors.Is(err, ErrInvalidEndpointBinding) {
			t.Errorf("bindings=%v error=%v", bindings, err)
		}
	}
}
