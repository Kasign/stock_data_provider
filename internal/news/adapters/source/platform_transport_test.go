package source

import (
	"testing"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

func TestNewPlatformEndpointTransportBindsEachEnabledFeedEndpoint(t *testing.T) {
	client, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatalf("httpclient.New() error = %v", err)
	}
	defer client.CloseIdleConnections()

	transport, err := NewPlatformEndpointTransport(client, []app.SourceEndpointRegistration{
		{
			SourceID: "source-a", EndpointID: "rss-main", Enabled: true,
			URL: "https://feeds-a.example.com/rss?channel=market", AccessMethod: domain.AccessRSS,
		},
		{
			SourceID: "source-a", EndpointID: "rss-secondary", Enabled: true,
			URL: "https://feeds-b.example.com/rss", AccessMethod: domain.AccessLicensedFeed,
		},
		{
			SourceID: "source-a", EndpointID: "disabled", Enabled: false,
			URL: "https://disabled.example.com/rss", AccessMethod: domain.AccessRSS,
		},
		{
			SourceID: "source-a", EndpointID: "unsupported", Enabled: true,
			URL: "https://api.example.com/news", AccessMethod: domain.AccessOfficialAPI,
		},
	})
	if err != nil {
		t.Fatalf("NewPlatformEndpointTransport() error = %v", err)
	}
	if len(transport.doers) != 2 {
		t.Fatalf("bound endpoint count = %d, want 2", len(transport.doers))
	}
	for key, doer := range transport.doers {
		platformDoer, ok := doer.(*platformHTTPDoer)
		if !ok || platformDoer.bound == nil {
			t.Fatalf("endpoint %+v is not backed by a Platform BoundDoer", key)
		}
	}
	if _, exists := transport.doers[EndpointKey{SourceID: "source-a", EndpointID: "disabled"}]; exists {
		t.Fatal("disabled endpoint must not receive a transport binding")
	}
}

func TestNewPlatformEndpointTransportRejectsDuplicateAndInvalidBindings(t *testing.T) {
	client, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatalf("httpclient.New() error = %v", err)
	}
	defer client.CloseIdleConnections()

	duplicate := app.SourceEndpointRegistration{
		SourceID: "source-a", EndpointID: "rss", Enabled: true,
		URL: "https://feeds.example.com/rss", AccessMethod: domain.AccessRSS,
	}
	if _, err := NewPlatformEndpointTransport(client, []app.SourceEndpointRegistration{duplicate, duplicate}); err == nil {
		t.Fatal("duplicate endpoint binding must fail")
	}
	if _, err := NewPlatformEndpointTransport(client, []app.SourceEndpointRegistration{{
		SourceID: "source-a", EndpointID: "bad", Enabled: true,
		URL: "file:///etc/passwd", AccessMethod: domain.AccessRSS,
	}}); err == nil {
		t.Fatal("unsafe endpoint URL must fail")
	}
}

func TestNewPlatformEndpointTransportRejectsEmptySupportedEndpointSet(t *testing.T) {
	client, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatalf("httpclient.New() error = %v", err)
	}
	defer client.CloseIdleConnections()

	_, err = NewPlatformEndpointTransport(client, []app.SourceEndpointRegistration{{
		SourceID: "source-a", EndpointID: "disabled", Enabled: false,
		URL: "https://feeds.example.com/rss", AccessMethod: domain.AccessRSS,
	}})
	if err == nil {
		t.Fatal("empty supported endpoint set must fail")
	}
}
