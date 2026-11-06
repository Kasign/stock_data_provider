package diagnostics

import (
	"context"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
	platformdiag "example.com/stock-analysis-flow/internal/platform/diagnostics"
)

func TestBusinessQualityCheckKeepsBusinessAndPlatformStatusesSeparate(t *testing.T) {
	check, err := BusinessQualityCheck(
		"observation-private-id",
		domain.Candidate{RawURL: "https://news.example/item", Title: "private article title"},
		domain.ContentLayers{Body: &domain.ContentPayload{Text: "private article body"}},
		domain.SourceIdentityConflict,
	)
	if err != nil {
		t.Fatal(err)
	}
	report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{check})
	if err != nil {
		t.Fatal(err)
	}
	result := report.Results[0]
	if result.Observation.Status != platformdiag.StatusHealthy {
		t.Fatalf("Platform status=%s, want healthy assessment execution", result.Observation.Status)
	}
	if result.Observation.Attributes["business_quality"] != string(domain.BusinessQualityInvalid) {
		t.Fatalf("business quality=%q", result.Observation.Attributes["business_quality"])
	}
	if result.Observation.Attributes["metadata_quality"] != string(domain.BusinessQualityInvalid) ||
		result.Observation.Attributes["source_quality"] != string(domain.BusinessQualityInvalid) {
		t.Fatalf("dimension attributes=%#v", result.Observation.Attributes)
	}
	if strings.Contains(result.Observation.SafeMessage, "private") ||
		strings.Contains(strings.Join(diagnosticAttributeValues(result.Observation.Attributes), " "), "private") {
		t.Fatal("diagnostic exposed article content or evidence identifier")
	}
	if result.Observation.EvidenceRef != "" {
		t.Fatalf("unexpected raw EvidenceRef=%q", result.Observation.EvidenceRef)
	}
}

func TestBusinessQualityCheckReportsUnknownSourceWithoutPromotingIt(t *testing.T) {
	check, err := BusinessQualityCheck(
		"observation-2",
		domain.Candidate{Title: "title", RawURL: "https://news.example/item"},
		domain.ContentLayers{SourceSummary: "summary"},
		domain.SourceIdentityUnknown,
	)
	if err != nil {
		t.Fatal(err)
	}
	report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{check})
	if err != nil {
		t.Fatal(err)
	}
	attributes := report.Results[0].Observation.Attributes
	if attributes["business_quality"] != string(domain.BusinessQualityUnknown) &&
		attributes["business_quality"] != string(domain.BusinessQualityDegraded) {
		t.Fatalf("business quality=%q", attributes["business_quality"])
	}
	if attributes["source_quality"] != string(domain.BusinessQualityUnknown) {
		t.Fatalf("source quality=%q", attributes["source_quality"])
	}
	if report.Results[0].Observation.Status != platformdiag.StatusHealthy {
		t.Fatalf("Platform status=%s", report.Results[0].Observation.Status)
	}
}

func TestBusinessQualityCheckRequiresStableEvidenceReference(t *testing.T) {
	if _, err := BusinessQualityCheck("  ", domain.Candidate{}, domain.ContentLayers{}, domain.SourceIdentityUnknown); err == nil {
		t.Fatal("empty evidence reference must be rejected")
	}
}

func diagnosticAttributeValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}
