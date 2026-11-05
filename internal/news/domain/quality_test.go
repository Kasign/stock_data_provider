package domain

import (
	"testing"
	"time"
)

func TestAssessBusinessQualitySeparatesDimensions(t *testing.T) {
	publishedAt := time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC)
	result := AssessBusinessQuality(
		Candidate{Title: "标题", RawURL: "https://news.example/item", PublishedAt: &publishedAt},
		ContentLayers{SourceSummary: "来源摘要"},
		SourceIdentityResolved,
	)
	if result.Status != BusinessQualityDegraded {
		t.Fatalf("status=%s", result.Status)
	}
	if len(result.Dimensions) != 3 || result.Dimensions[0].Status != BusinessQualityValid {
		t.Fatalf("dimensions=%+v", result.Dimensions)
	}
	if result.Dimensions[1].Status != BusinessQualityDegraded || result.Dimensions[2].Status != BusinessQualityValid {
		t.Fatalf("dimensions=%+v", result.Dimensions)
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != "body_missing_summary_only" {
		t.Fatalf("issues=%+v", result.Issues)
	}
}

func TestAssessBusinessQualityDoesNotTreatPublisherTextAsResolvedIdentity(t *testing.T) {
	result := AssessBusinessQuality(
		Candidate{Title: "标题", PublisherName: "某媒体"},
		ContentLayers{ProviderSummary: "搜索摘要"},
		SourceIdentityUnknown,
	)
	if result.Status != BusinessQualityDegraded {
		t.Fatalf("status=%s", result.Status)
	}
	if result.Dimensions[2].Status != BusinessQualityUnknown {
		t.Fatalf("source=%+v", result.Dimensions[2])
	}
	if result.Dimensions[1].Issues[0].Code != "source_content_missing_provider_summary_only" {
		t.Fatalf("content issues=%+v", result.Dimensions[1].Issues)
	}
}

func TestAssessBusinessQualityMarksMissingTitleInvalidAndIdentityConflictInvalid(t *testing.T) {
	result := AssessBusinessQuality(
		Candidate{RawURL: "https://news.example/item"},
		ContentLayers{Body: &ContentPayload{Text: "正文", Format: ContentFormatPlainText}},
		SourceIdentityConflict,
	)
	if result.Status != BusinessQualityInvalid {
		t.Fatalf("status=%s", result.Status)
	}
	if result.Dimensions[0].Status != BusinessQualityInvalid || result.Dimensions[2].Status != BusinessQualityInvalid {
		t.Fatalf("dimensions=%+v", result.Dimensions)
	}
}

func TestAssessBusinessQualityDoesNotPromoteProviderSummaryToSourceContent(t *testing.T) {
	result := AssessBusinessQuality(
		Candidate{Title: "标题", RawURL: "https://news.example/item"},
		ContentLayers{ProviderSummary: "Provider摘要"},
		SourceIdentityResolved,
	)
	if result.Dimensions[1].Status != BusinessQualityDegraded || result.Dimensions[1].Issues[0].Code != "source_content_missing_provider_summary_only" {
		t.Fatalf("content=%+v", result.Dimensions[1])
	}
}
