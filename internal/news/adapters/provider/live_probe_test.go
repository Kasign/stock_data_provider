package provider

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

// TestLiveProviderProbe is an explicitly enabled, read-only online smoke test.
// It never persists Provider output and never logs candidate content or credentials.
func TestLiveProviderProbe(t *testing.T) {
	if os.Getenv("NEWS_LIVE_PROBE") != "1" {
		t.Skip("NEWS_LIVE_PROBE is not enabled")
	}

	cfg, err := newsconfig.Load(platformconfig.NewEnv())
	if err != nil {
		t.Fatalf("load News Provider configuration: %v", err)
	}
	client, err := httpclient.New(httpclient.Config{Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("create Platform HTTP client: %v", err)
	}
	defer client.CloseIdleConnections()

	set, err := NewPlatformProviderSet(cfg, client)
	if err != nil {
		t.Fatalf("create Platform-bound Provider set: %v", err)
	}
	providers := set.All()
	if len(providers) == 0 {
		t.Skip("no News Provider credentials/endpoints are configured")
	}

	query := strings.TrimSpace(os.Getenv("NEWS_LIVE_QUERY"))
	if query == "" {
		query = "A股上市公司公告"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	for _, current := range providers {
		provider := current
		t.Run(provider.Code(), func(t *testing.T) {
			started := time.Now()
			candidates, searchErr := provider.Search(ctx, domain.SearchRequest{
				Query: query, QueryDimension: "live_probe", Limit: 3,
			})
			duration := time.Since(started)
			if searchErr != nil {
				var providerErr *app.ProviderError
				if errors.As(searchErr, &providerErr) {
					t.Fatalf("live Provider probe failed: provider=%s code=%s duration=%s", provider.Code(), providerErr.Code, duration)
				}
				t.Fatalf("live Provider probe failed: provider=%s duration=%s", provider.Code(), duration)
			}
			t.Logf("live Provider probe responded: provider=%s candidate_count=%d duration=%s", provider.Code(), len(candidates), duration)
		})
	}
}
