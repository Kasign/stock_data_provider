package source

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

func staticCatalogRegistration() app.SourceEndpointRegistration {
	expiresAt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	return app.SourceEndpointRegistration{
		SourceID: "source-a", EndpointID: "rss-main", Enabled: true,
		URL: "https://publisher.example/feed.xml", PublisherName: "Publisher",
		PublisherDomain: "publisher.example", AccessMethod: domain.AccessRSS,
		SourcePolicyVersionID: "source-policy-v1",
		SourcePolicy: domain.SourcePolicy{
			Status:               domain.AuthorizationApproved,
			AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
			AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:  []domain.ContentKind{domain.ContentMetadata},
			AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient},
			MaxRetention:         time.Hour, EffectiveAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			ExpiresAt: &expiresAt, EvidenceRef: "authorization-evidence",
		},
		URLPolicy: domain.SourceURLPolicy{
			AllowedSchemes: []string{"https"},
			AllowedHosts:   []string{"publisher.example"},
		},
	}
}

func TestStaticSourceCatalogReturnsDefensiveCopies(t *testing.T) {
	registration := staticCatalogRegistration()
	catalog, err := NewStaticSourceCatalog([]app.SourceEndpointRegistration{registration})
	if err != nil {
		t.Fatal(err)
	}

	// 修改构造参数不能改变目录中的策略快照。
	registration.SourcePolicy.AllowedPurposes[0] = domain.PurposePublicDisplay
	registration.URLPolicy.AllowedHosts[0] = "attacker.example"

	first, err := catalog.LookupEndpoint(context.Background(), "source-a", "rss-main")
	if err != nil {
		t.Fatal(err)
	}
	if first.SourcePolicy.AllowedPurposes[0] != domain.PurposeInternalPersonalResearch ||
		first.URLPolicy.AllowedHosts[0] != "publisher.example" {
		t.Fatalf("catalog changed after input mutation: %+v", first)
	}

	// 修改查询结果也不能污染后续读取。
	first.SourcePolicy.AllowedPurposes[0] = domain.PurposePublicDisplay
	first.URLPolicy.AllowedHosts[0] = "attacker.example"
	again, err := catalog.LookupEndpoint(context.Background(), "source-a", "rss-main")
	if err != nil {
		t.Fatal(err)
	}
	if again.SourcePolicy.AllowedPurposes[0] != domain.PurposeInternalPersonalResearch ||
		again.URLPolicy.AllowedHosts[0] != "publisher.example" {
		t.Fatalf("catalog leaked mutable policy data: %+v", again)
	}
}

func TestStaticSourceCatalogRejectsDuplicateAndInvalidRegistrations(t *testing.T) {
	valid := staticCatalogRegistration()
	if _, err := NewStaticSourceCatalog([]app.SourceEndpointRegistration{valid, valid}); !errors.Is(err, ErrInvalidSourceCatalog) {
		t.Fatalf("duplicate registration error = %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*app.SourceEndpointRegistration)
	}{
		{
			name: "missing source policy version",
			mutate: func(value *app.SourceEndpointRegistration) {
				value.SourcePolicyVersionID = ""
			},
		},
		{
			name: "endpoint policy without immutable version",
			mutate: func(value *app.SourceEndpointRegistration) {
				value.EndpointPolicy = &domain.EndpointPolicy{Status: domain.AuthorizationApproved}
			},
		},
		{
			name: "endpoint version without policy",
			mutate: func(value *app.SourceEndpointRegistration) {
				value.EndpointPolicyVersionID = "endpoint-policy-v1"
			},
		},
		{
			name: "url outside exact host allowlist",
			mutate: func(value *app.SourceEndpointRegistration) {
				value.URL = "https://unlisted.example/feed.xml"
			},
		},
		{
			name: "whitespace identity",
			mutate: func(value *app.SourceEndpointRegistration) {
				value.EndpointID = " rss-main"
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value := staticCatalogRegistration()
			testCase.mutate(&value)
			if _, err := NewStaticSourceCatalog([]app.SourceEndpointRegistration{value}); !errors.Is(err, ErrInvalidSourceCatalog) {
				t.Fatalf("invalid registration error = %v", err)
			}
		})
	}
}

func TestStaticSourceCatalogLookupIsExactAndHonorsContext(t *testing.T) {
	catalog, err := NewStaticSourceCatalog([]app.SourceEndpointRegistration{staticCatalogRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.LookupEndpoint(context.Background(), "source-a", "RSS-MAIN"); !errors.Is(err, app.ErrSourceEndpointNotFound) {
		t.Fatalf("non-exact endpoint lookup error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := catalog.LookupEndpoint(ctx, "source-a", "rss-main"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lookup error = %v", err)
	}
}
