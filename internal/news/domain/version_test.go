package domain

import (
	"testing"
	"time"
)

func TestBuildVersionFingerprintIgnoresObservationAndProviderSummary(t *testing.T) {
	published := time.Date(2026, 11, 5, 9, 30, 0, 0, time.UTC)
	first := Candidate{
		Title: "  市场 观察 ", PublisherName: "媒体", PublishedAt: &published,
		ProviderSummary: "provider summary A", ObservedAt: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC),
	}
	second := first
	second.ProviderSummary = "provider summary B"
	second.ObservedAt = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	if a, b := BuildVersionFingerprint(first, ContentLayers{}), BuildVersionFingerprint(second, ContentLayers{}); a != b {
		t.Fatalf("non-version fields changed fingerprint: %s != %s", a, b)
	}
}

func TestBuildVersionFingerprintChangesWithSourceContent(t *testing.T) {
	candidate := Candidate{Title: "标题"}
	first := ContentLayers{
		SourceSummary: "来源摘要",
		SourceSummaryOrigin: ContentOriginFeedSummary,
		Body: &ContentPayload{Origin: ContentOriginFeedBody, Format: ContentFormatUnknown, Kind: ContentFullText, Text: "第一段\n第二段"},
	}
	second := first
	body := *first.Body
	body.Text = "第一段\n第二段\n第三段"
	second.Body = &body
	if BuildVersionFingerprint(candidate, first) == BuildVersionFingerprint(candidate, second) {
		t.Fatal("body change must produce a new fingerprint")
	}

	third := first
	third.SourceSummary = "修订后的摘要"
	if BuildVersionFingerprint(candidate, first) == BuildVersionFingerprint(candidate, third) {
		t.Fatal("source summary change must produce a new fingerprint")
	}
}

func TestBuildVersionFingerprintNormalizesMetadataAndPreservesBodyMeaning(t *testing.T) {
	publishedA := time.Date(2026, 11, 5, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	publishedB := publishedA.UTC()
	first := Candidate{Title: " 标题  内容 ", PublishedAt: &publishedA}
	second := Candidate{Title: "标题 内容", PublishedAt: &publishedB}
	contentA := ContentLayers{SourceSummary: " 摘要  内容 "}
	contentB := ContentLayers{SourceSummary: "摘要 内容"}
	if BuildVersionFingerprint(first, contentA) != BuildVersionFingerprint(second, contentB) {
		t.Fatal("equivalent normalized metadata should share a fingerprint")
	}
}

func TestBuildVersionFingerprintUsesRawUnknownPublishedTime(t *testing.T) {
	first := Candidate{Title: "标题", PublishedAtRaw: "2026-11-05"}
	second := Candidate{Title: "标题", PublishedAtRaw: "2026-11-06"}
	if BuildVersionFingerprint(first, ContentLayers{}) == BuildVersionFingerprint(second, ContentLayers{}) {
		t.Fatal("unknown publication time text change must be represented")
	}
}

func TestBuildVersionFingerprintExcludesProviderSummaryLayer(t *testing.T) {
	candidate := Candidate{Title: "标题"}
	first := ContentLayers{ProviderSummary: "搜索摘要 A", SourceSummary: "来源摘要"}
	second := ContentLayers{ProviderSummary: "搜索摘要 B", SourceSummary: "来源摘要"}
	if BuildVersionFingerprint(candidate, first) != BuildVersionFingerprint(candidate, second) {
		t.Fatal("provider search summary must not create source content versions")
	}
}
