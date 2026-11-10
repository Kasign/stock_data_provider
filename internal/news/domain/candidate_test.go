package domain

import (
	"testing"
	"time"
)

func TestNormalizeCandidateNormalizesFieldsAndUsesFallbackContext(t *testing.T) {
	observedAt := time.Date(2026, 11, 5, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	candidate := Candidate{
		ProviderCode: "  tavily ", ProviderItemID: " provider-item-1 ", SourceItemID: " source-item-1 ", Author: "  编辑   甲  ",
		PublisherName: "  媒体名称  ", PublisherDomain: " EXAMPLE.COM ",
		Title: "  市场   新闻\n标题 ", ProviderSummary: " 摘要  内容 ",
		RawURL: " https://publisher.example/a ", PublishedAtRaw: "Wed, 04 Nov 2026 10:30:00 +0800",
	}
	got := NormalizeCandidate(candidate, observedAt, " market ")
	if got.ProviderCode != "tavily" || got.ProviderItemID != "provider-item-1" || got.SourceItemID != "source-item-1" || got.Author != "编辑 甲" || got.PublisherName != "媒体名称" {
		t.Fatalf("identity fields=%+v", got)
	}
	if got.PublisherDomain != "example.com" || got.Title != "市场 新闻 标题" || got.ProviderSummary != "摘要 内容" {
		t.Fatalf("normalized text=%+v", got)
	}
	if got.RawURL != "https://publisher.example/a" || got.CanonicalURL != "" || got.QueryDimension != "market" {
		t.Fatalf("url/dimension=%+v", got)
	}
	if !got.ObservedAt.Equal(observedAt.UTC()) || got.ObservedAt.Location() != time.UTC {
		t.Fatalf("observed_at=%v", got.ObservedAt)
	}
	want := time.Date(2026, 11, 4, 2, 30, 0, 0, time.UTC)
	if got.PublishedAt == nil || !got.PublishedAt.Equal(want) {
		t.Fatalf("published_at=%v", got.PublishedAt)
	}
	if len(got.MissingFields) != 0 {
		t.Fatalf("missing_fields=%v", got.MissingFields)
	}
}

func TestNormalizeCandidateDoesNotInferPublicationTimezoneOrPublisherDomain(t *testing.T) {
	got := NormalizeCandidate(Candidate{
		Title: "标题", RawURL: "https://article.example/path",
		PublishedAtRaw: "2026-11-04", PublisherName: "媒体",
	}, time.Time{}, "")
	if got.PublishedAt != nil || len(got.MissingFields) != 1 || got.MissingFields[0] != "published_at" {
		t.Fatalf("candidate=%+v", got)
	}
	if got.PublisherDomain != "" {
		t.Fatalf("publisher domain must not be inferred: %q", got.PublisherDomain)
	}
	if !got.ObservedAt.IsZero() {
		t.Fatalf("zero fallback must remain zero: %v", got.ObservedAt)
	}
}

func TestNormalizeCandidateRebuildsMissingFieldsInStableOrder(t *testing.T) {
	publishedAt := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	got := NormalizeCandidate(Candidate{
		MissingFields: []string{"stale", "title"},
		PublishedAt:   &publishedAt,
	}, time.Time{}, "")
	want := []string{"title", "url", "publisher"}
	if len(got.MissingFields) != len(want) {
		t.Fatalf("missing_fields=%v", got.MissingFields)
	}
	for i := range want {
		if got.MissingFields[i] != want[i] {
			t.Fatalf("missing_fields=%v want=%v", got.MissingFields, want)
		}
	}
	if got.PublishedAt.Location() != time.UTC {
		t.Fatalf("published_at location=%v", got.PublishedAt.Location())
	}
}

func TestNormalizeCandidateDoesNotMutateCallerMissingFields(t *testing.T) {
	original := make([]string, 2, 4)
	original[0] = "caller-value-1"
	original[1] = "caller-value-2"
	candidate := Candidate{MissingFields: original}

	got := NormalizeCandidate(candidate, time.Time{}, "")

	if original[0] != "caller-value-1" || original[1] != "caller-value-2" {
		t.Fatalf("caller-owned missing fields were modified: %v", original)
	}
	if len(got.MissingFields) != 4 {
		t.Fatalf("normalized missing fields=%v", got.MissingFields)
	}
	got.MissingFields[0] = "changed-result"
	if original[0] != "caller-value-1" {
		t.Fatalf("normalized result still aliases caller slice: %v", original)
	}
}

func TestNormalizeCandidatePreservesUnknownTimestampText(t *testing.T) {
	raw := "not-a-date"
	got := NormalizeCandidate(Candidate{PublishedAtRaw: raw}, time.Time{}, "")
	if got.PublishedAtRaw != raw || got.PublishedAt != nil {
		t.Fatalf("candidate=%+v", got)
	}
}

func TestNormalizeCandidateNormalizesFetchAndNormalizationTimes(t *testing.T) {
	fetchedAt := time.Date(2026, 11, 6, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	normalizedAt := fetchedAt.Add(time.Second)
	got := NormalizeCandidate(Candidate{
		FetchedAt: &fetchedAt, NormalizedAt: &normalizedAt,
	}, time.Time{}, "")
	if got.FetchedAt == nil || got.FetchedAt.Location() != time.UTC ||
		got.NormalizedAt == nil || got.NormalizedAt.Location() != time.UTC {
		t.Fatalf("fetch/normalize times were not normalized: %+v", got)
	}
	if !got.FetchedAt.Equal(fetchedAt.UTC()) || !got.NormalizedAt.Equal(normalizedAt.UTC()) {
		t.Fatalf("fetch/normalize times changed: fetched=%v normalized=%v", got.FetchedAt, got.NormalizedAt)
	}
}
