package bootstrap

import (
	"strings"
	"testing"

	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
)

func TestNewsFeedWorkflowTemplateUsesConfiguredSourceSnapshotAndCombinedRegistry(t *testing.T) {
	registry, err := newNewsStageOutputRegistry(newsFeedStageOutputRegistryVersion, true)
	if err != nil {
		t.Fatal(err)
	}
	stage := &newsFeedStageComposition{
		registry: registry,
		resourceRefs: []string{
			"source-a/primary@" + strings.Repeat("a", 64),
			"source-b/primary@" + strings.Repeat("b", 64),
		},
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	value, err := buildNewsFeedWorkflowTemplate(stage, policies)
	if err != nil {
		t.Fatal(err)
	}
	if value.ResourcePolicyRef != resourcepolicy.NewsFeedConfiguredSourcesRef ||
		value.OutputRegistryVersion != newsFeedStageOutputRegistryVersion ||
		len(value.Stages) != 1 || value.Stages[0].Code != newsstage.CollectFeedsStageCode {
		t.Fatalf("unexpected News Feed workflow template: %+v", value)
	}
	if value.Stages[0].Retry.MaxAttempts != 3 ||
		len(value.Stages[0].Retry.RetryableReasonCodes) != 1 ||
		value.Stages[0].Retry.RetryableReasonCodes[0] != "news_feed_retryable_source_failure" {
		t.Fatalf("News Feed retry contract inconsistent: %+v", value.Stages[0].Retry)
	}
}

func TestNewsFeedWorkflowTemplateRejectsMissingSourceInventory(t *testing.T) {
	registry, err := newNewsStageOutputRegistry(newsFeedStageOutputRegistryVersion, true)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildNewsFeedWorkflowTemplate(&newsFeedStageComposition{registry: registry}, policies); err == nil {
		t.Fatal("News Feed workflow template accepted empty source inventory")
	}
}
