package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	researchflowresourcepolicy "example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

func TestResearchflowOwnerCompositionBuildsInternalOwnerServices(t *testing.T) {
	newsCfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	sharedClient, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer sharedClient.CloseIdleConnections()

	composition, err := newResearchflowOwnerComposition(
		&serverresources.Resources{DB: &sql.DB{}, HTTPClient: sharedClient},
		newsCfg,
		serverconfig.ResearchflowCapacity{Configured: true, MaxQueuedRuns: 10, MaxWorkers: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	if composition.createRun == nil || composition.getRun == nil || composition.cancelRun == nil {
		t.Fatal("internal CreateRun/GetRun/CancelRun services must be composed together")
	}
	if _, err := composition.resourcePolicies.Resolve(context.Background(), researchflowresourcepolicy.NewsSearchConfiguredProvidersRef); !errors.Is(err, researchflowresourcepolicy.ErrNoProviders) {
		t.Fatalf("empty assembled Provider inventory must fail closed, got %v", err)
	}
}

func TestResearchflowIDGeneratorUsesPurposeScopedCryptographicReferences(t *testing.T) {
	generator := researchflowIDGenerator{}
	first, err := generator.NewRef(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	second, err := generator.NewRef(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) != len("run_")+32 || first[:4] != "run_" {
		t.Fatalf("unexpected generated references: %q, %q", first, second)
	}
	if _, err := generator.NewRef(context.Background(), "bad purpose"); err == nil {
		t.Fatal("invalid reference purpose was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := generator.NewRef(ctx, "run"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context was ignored: %v", err)
	}
}
