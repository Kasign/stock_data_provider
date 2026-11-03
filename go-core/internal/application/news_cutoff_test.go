package application

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

func TestFilterNewsByReference(t *testing.T) {
	cutoff := time.Date(2026, 10, 21, 3, 30, 0, 0, time.UTC)
	items := []domain.NewsItem{
		{Title: "before", PublishedAt: "2026-10-21T03:29:59Z"},
		{Title: "at-cutoff", PublishedAt: "2026-10-21T03:30:00Z"},
		{Title: "future", PublishedAt: "2026-10-21T03:30:01Z"},
		{Title: "unknown"},
		{Title: "invalid", PublishedAt: "yesterday afternoon"},
	}
	strict, warnings := filterNewsByReference(items, cutoff, true)
	if len(strict) != 2 || strict[0].Title != "before" || strict[1].Title != "at-cutoff" {
		t.Fatalf("strict point-in-time filter accepted future/unknown news: %+v", strict)
	}
	if len(warnings) == 0 {
		t.Fatal("expected diagnostics for future and unverified news")
	}
	live, liveWarnings := filterNewsByReference(items, cutoff, false)
	if len(live) != 4 {
		t.Fatalf("live filter should retain unknown-time items as unverified and reject future data, got %+v", live)
	}
	if len(liveWarnings) == 0 {
		t.Fatal("expected live unverified-time warning")
	}
}
