package bootstrap

import (
	"testing"

	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
)

func TestNewsAndQuantOutputRegistryVersionsRemainGloballyDistinct(t *testing.T) {
	versions := []string{
		newsSearchStageOutputRegistryVersion,
		quantSMA20StageOutputRegistryVersion,
		newsFeedStageOutputRegistryVersion,
	}
	seen := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		if _, exists := seen[version]; exists {
			t.Fatalf("different immutable Stage Output Registry contents reuse version %q", version)
		}
		seen[version] = struct{}{}
	}
}

func TestNewsSearchAndFeedShareOneImmutableOutputRegistryVersion(t *testing.T) {
	registry, err := newNewsStageOutputRegistry(newsFeedStageOutputRegistryVersion, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Entry(newsstage.SearchCandidatesStageCode, newsstage.SearchCandidatesOutputType, newsstage.SearchCandidatesSchemaVersion); !ok {
		t.Fatal("combined News Output Registry omitted SearchCandidates output")
	}
	if _, ok := registry.Entry(newsstage.CollectFeedsStageCode, newsstage.CollectFeedsOutputType, newsstage.CollectFeedsSchemaVersion); !ok {
		t.Fatal("combined News Output Registry omitted FeedCollection output")
	}
	if _, err := researchflowapplication.NewStageOutputRegistrySet(registry); err != nil {
		t.Fatalf("combined registry cannot be registered as one immutable version: %v", err)
	}
}
