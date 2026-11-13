package source

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

// TestLiveFeedProbe performs one explicitly authorized RSS/Atom fetch through
// the Platform SSRF boundary. It keeps the response in memory and never persists entries.
func TestLiveFeedProbe(t *testing.T) {
	if os.Getenv("NEWS_LIVE_PROBE") != "1" {
		t.Skip("NEWS_LIVE_PROBE is not enabled")
	}
	feedURL := strings.TrimSpace(os.Getenv("NEWS_LIVE_FEED_URL"))
	evidenceRef := strings.TrimSpace(os.Getenv("NEWS_LIVE_FEED_AUTHORIZATION_EVIDENCE"))
	if feedURL == "" || evidenceRef == "" {
		t.Skip("NEWS_LIVE_FEED_URL or NEWS_LIVE_FEED_AUTHORIZATION_EVIDENCE is not configured")
	}

	parsed, err := url.Parse(feedURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		t.Fatal("live feed URL must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	allowedPorts := []uint16(nil)
	if parsed.Port() != "" {
		port, parseErr := strconv.ParseUint(parsed.Port(), 10, 16)
		if parseErr != nil || port == 0 {
			t.Fatal("live feed URL has an invalid port")
		}
		if port != 443 {
			allowedPorts = []uint16{uint16(port)}
		}
	}

	now := time.Now().UTC()
	registration := app.SourceEndpointRegistration{
		SourceID:              "live-probe-source",
		EndpointID:            "live-probe-feed",
		Enabled:               true,
		URL:                   feedURL,
		PublisherName:         "Live probe source",
		PublisherDomain:       host,
		AccessMethod:          domain.AccessRSS,
		SourcePolicyVersionID: "live-probe-source-policy-v1",
		SourcePolicy: domain.SourcePolicy{
			Status:               domain.AuthorizationApproved,
			AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
			AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:  []domain.ContentKind{domain.ContentMetadata},
			AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient},
			MaxRetention:         time.Minute,
			EffectiveAt:          now.Add(-time.Minute),
			EvidenceRef:          evidenceRef,
		},
		URLPolicy: domain.SourceURLPolicy{
			AllowedSchemes: []string{"https"},
			AllowedHosts:   []string{host},
			AllowedPorts:   allowedPorts,
		},
		MaxResponseBytes: 2 << 20,
	}

	catalog, err := NewStaticSourceCatalog([]app.SourceEndpointRegistration{registration})
	if err != nil {
		t.Fatalf("create explicit live-feed registration: %v", err)
	}
	client, err := httpclient.New(httpclient.Config{Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("create Platform HTTP client: %v", err)
	}
	defer client.CloseIdleConnections()

	transport, err := NewPlatformEndpointTransport(client, []app.SourceEndpointRegistration{registration})
	if err != nil {
		t.Fatalf("create Platform-bound feed transport: %v", err)
	}
	collector := app.NewCollectSource(catalog, transport)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	started := time.Now()
	result, err := collector.Execute(ctx, app.CollectSourceRequest{
		SourceID:       registration.SourceID,
		EndpointID:     registration.EndpointID,
		Purpose:        domain.PurposeInternalPersonalResearch,
		ContentKind:    domain.ContentMetadata,
		StorageMode:    domain.StorageTransient,
		Retention:      time.Minute,
		MaxEntries:     5,
		QueryDimension: "live_probe",
	})
	duration := time.Since(started)
	if err != nil {
		t.Fatalf("live RSS/Atom probe failed: code=%s duration=%s", app.SourceCollectionErrorCode(err), duration)
	}
	if result.FeedFormat == "" {
		t.Fatal("live RSS/Atom probe returned no feed format")
	}
	t.Logf("live feed probe responded: format=%s entry_count=%d duration=%s", result.FeedFormat, len(result.Entries), duration)
}
