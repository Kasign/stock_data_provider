package resources

import (
	"testing"

	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"

	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
)

func TestOpenLegacyAnalysisStorageRequiresContext(t *testing.T) {
	if _, _, err := OpenLegacyAnalysisStorage(nil, mysqlredis.Config{}); err == nil {
		t.Fatal("OpenLegacyAnalysisStorage() error = nil, want context error")
	}
}

func TestStorageCloseIsNilSafe(t *testing.T) {
	var storage *Storage
	if err := storage.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestResourcesCloseIsIdempotent(t *testing.T) {
	resources := &Resources{}
	if err := resources.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := resources.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestResourcesBindHTTPDestinationUsesSharedClient(t *testing.T) {
	client, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		t.Fatalf("create Platform HTTP Client: %v", err)
	}
	resources := &Resources{HTTPClient: client}
	bound, err := resources.BindHTTPDestination(platformhttpclient.DestinationPolicy{
		Dependency: "test.upstream",
		AllowedHosts: []string{"api.example.com"},
	})
	if err != nil {
		t.Fatalf("BindHTTPDestination() error = %v", err)
	}
	if bound == nil {
		t.Fatal("BindHTTPDestination() returned nil doer")
	}
	if err := resources.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := resources.BindHTTPDestination(platformhttpclient.DestinationPolicy{
		Dependency: "test.upstream",
		AllowedHosts: []string{"api.example.com"},
	}); err == nil {
		t.Fatal("BindHTTPDestination() after Close() error = nil")
	}
}

func TestResourcesBindHTTPDestinationRejectsMissingClient(t *testing.T) {
	resources := &Resources{}
	if _, err := resources.BindHTTPDestination(platformhttpclient.DestinationPolicy{
		Dependency: "test.upstream",
		AllowedHosts: []string{"api.example.com"},
	}); err == nil {
		t.Fatal("BindHTTPDestination() error = nil, want missing client error")
	}
}

func TestResourcesBindHTTPDestinationRejectsInvalidPolicy(t *testing.T) {
	client, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		t.Fatalf("create Platform HTTP Client: %v", err)
	}
	resources := &Resources{HTTPClient: client}
	t.Cleanup(func() { _ = resources.Close() })
	if _, err := resources.BindHTTPDestination(platformhttpclient.DestinationPolicy{
		Dependency: "invalid dependency",
		AllowedHosts: []string{"api.example.com"},
	}); err == nil {
		t.Fatal("BindHTTPDestination() error = nil, want invalid policy error")
	}
}
