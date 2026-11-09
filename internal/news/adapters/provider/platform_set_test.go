package provider

import (
	"net/http"
	"testing"

	"example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

func TestNewPlatformProviderSetBindsConfiguredProviders(t *testing.T) {
	values := map[string]string{
		newsconfig.BochaEndpointEnv:   "https://bocha.example.com/v1/search",
		newsconfig.BochaAPIKeyEnv:     "bocha-secret",
		newsconfig.TavilyEndpointEnv:  "https://api.tavily.example/search",
		newsconfig.TavilyAPIKeyEnv:    "tavily-secret"
		newsconfig.SearXNGEndpointEnv: "https://search.example.com",
	}
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	config, err := newsconfig.Load(env)
	if err != nil {
		t.Fatalf("newsconfig.Load() error = %v", err)
	}
	client, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatalf("httpclient.New() error = %v", err)
	}
	defer client.CloseIdleConnections()

	set, err := NewPlatformProviderSet(config, client)
	if err != nil {
		t.Fatalf("NewPlatformProviderSet() error = %v", err)
	}
	providers := set.All()
	if len(providers) != 3 {
		t.Fatalf("provider count = %d, want 3", len(providers))
	}
	want := []string{BochaCode, TavilyCode, SearXNGCode}
	for index, provider := range providers {
		if provider.Code() != want[index] {
			t.Fatalf("provider[%d] code = %q, want %q", index, provider.Code(), want[index])
		}
		var doer app.HTTPDoer
		switch value := provider.(type) {
		case *Bocha:
			doer = value.cfg.client
		case *Tavily:
			doer = value.cfg.client
		case *SearXNG:
			doer = value.cfg.client
		default:
			t.Fatalf("unexpected provider type %T", provider)
		}
		bound, ok := doer.(*platformHTTPDoer)
		if !ok || bound.bound == nil {
			t.Fatalf("provider %s is not backed by a Platform BoundDoer", provider.Code())
		}
	}
}

func TestNewPlatformProviderSetAllowsNoOptionalProviders(t *testing.T) {
	set, err := NewPlatformProviderSet(newsconfig.Config{}, nil)
	if err != nil {
		t.Fatalf("NewPlatformProviderSet() error = %v", err)
	}
	if providers := set.All(); len(providers) != 0 {
		t.Fatalf("provider count = %d, want 0", len(providers))
	}
}

func TestNewPlatformProviderSetRejectsConfiguredProviderWithoutClient(t *testing.T) {
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case newsconfig.BochaEndpointEnv:
			return "https://bocha.example.com/search", true
		case newsconfig.BochaAPIKeyEnv:
			return "bocha-secret", true
		default:
			return "", false
		}
	})
	config, err := newsconfig.Load(env)
	if err != nil {
		t.Fatalf("newsconfig.Load() error = %v", err)
	}
	if _, err := NewPlatformProviderSet(config, nil); err == nil {
		t.Fatal("NewPlatformProviderSet() error = nil, want missing client error")
	}
}
