package provider

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

// TestLiveProviderProbe is an explicitly enabled, read-only online smoke test.
// It never persists Provider output and never logs candidate content or credentials.
func TestLiveProviderProbe(t *testing.T) {
	if os.Getenv("NEWS_LIVE_PROBE") != "1" {
		t.Skip("NEWS_LIVE_PROBE is not enabled")
	}

	cfg, err := newsconfig.Load(platformconfig.NewEnv())
	if err != nil {
		t.Fatalf("load News Provider configuration: %v", err)
	}
	client, err := httpclient.New(httpclient.Config{Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("create Platform HTTP client: %v", err)
	}
	defer client.CloseIdleConnections()

	set, err := NewPlatformProviderSet(cfg, client)
	if err != nil {
		t.Fatalf("create Platform-bound Provider set: %v", err)
	}
	providers := set.All()
	if len(providers) == 0 {
		t.Skip("no News Provider credentials/endpoints are configured")
	}

	query := strings.TrimSpace(os.Getenv("NEWS_LIVE_QUERY"))
	if query == "" {
		query = "A股上市公司公告"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	for _, current := range providers {
		provider := current
		t.Run(provider.Code(), func(t *testing.T) {
			started := time.Now()
			candidates, searchErr := provider.Search(ctx, domain.SearchRequest{
				Query: query, QueryDimension: "live_probe", Limit: 3,
			})
			duration := time.Since(started)
			if searchErr != nil {
				var providerErr *app.ProviderError
				if errors.As(searchErr, &providerErr) {
					t.Fatalf("live Provider probe failed: provider=%s code=%s duration=%s", provider.Code(), providerErr.Code, duration)
				}
				t.Fatalf("live Provider probe failed: provider=%s duration=%s", provider.Code(), duration)
			}
			stats := summarizePublicationTimes(candidates, time.Now().UTC())
			t.Logf("live Provider probe responded: provider=%s candidate_count=%d duration=%s", provider.Code(), len(candidates), duration)
			t.Logf("live Provider publication-time stats: provider=%s total=%d raw_present=%d raw_missing=%d parsed=%d date_only=%d local_datetime_without_timezone=%d unparseable=%d explicit_timezone=%d future_over_5m=%d",
				provider.Code(), stats.Total, stats.RawPresent, stats.RawMissing, stats.Parsed, stats.DateOnly,
				stats.LocalDateTimeWithoutTimezone, stats.Unparseable, stats.ExplicitTimezone, stats.FutureOver5m)
		})
	}
}

type publicationTimeStats struct {
	Total                        int
	RawPresent                   int
	RawMissing                   int
	Parsed                       int
	DateOnly                     int
	LocalDateTimeWithoutTimezone int
	Unparseable                  int
	ExplicitTimezone             int
	FutureOver5m                 int
}

// summarizePublicationTimes only returns aggregate counts. It never emits raw
// timestamps, titles, URLs, snippets, credentials, or response bodies.
func summarizePublicationTimes(candidates []domain.Candidate, observedAt time.Time) publicationTimeStats {
	stats := publicationTimeStats{Total: len(candidates)}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observedAt = observedAt.UTC()

	for _, candidate := range candidates {
		raw := strings.TrimSpace(candidate.PublishedAtRaw)
		if raw == "" {
			stats.RawMissing++
			continue
		}
		stats.RawPresent++

		if isPublicationDateOnly(raw) {
			stats.DateOnly++
		} else if hasExplicitPublicationTimezone(raw) {
			stats.ExplicitTimezone++
		} else if isLocalPublicationDateTime(raw) {
			stats.LocalDateTimeWithoutTimezone++
		}

		normalized := domain.NormalizeCandidate(candidate, observedAt, candidate.QueryDimension)
		if normalized.PublishedAt == nil {
			if !isPublicationDateOnly(raw) && !isLocalPublicationDateTime(raw) {
				stats.Unparseable++
			}
			continue
		}
		stats.Parsed++
		if normalized.PublishedAt.After(observedAt.Add(5 * time.Minute)) {
			stats.FutureOver5m++
		}
	}
	return stats
}

func isPublicationDateOnly(raw string) bool {
	for _, layout := range []string{"2006-01-02", "2006/01/02"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil && parsed.Format(layout) == raw {
			return true
		}
	}
	return false
}

func hasExplicitPublicationTimezone(raw string) bool {
	for _, layout := range []string{time.RFC3339Nano, time.RFC1123Z, time.RFC822Z} {
		if _, err := time.Parse(layout, raw); err == nil {
			return true
		}
	}
	return false
}

func isLocalPublicationDateTime(raw string) bool {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
	} {
		if _, err := time.Parse(layout, raw); err == nil {
			return true
		}
	}
	return false
}

func TestSummarizePublicationTimesUsesOnlyAggregateCounts(t *testing.T) {
	observedAt := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	candidates := []domain.Candidate{
		{PublishedAtRaw: "2026-11-16T10:30:00+08:00"},
		{PublishedAtRaw: "Mon, 16 Nov 2026 10:30:00 +0800"},
		{PublishedAtRaw: "2026-11-16"},
		{PublishedAtRaw: "2026-11-16 10:30:00"},
		{PublishedAtRaw: "not-a-timestamp"},
		{PublishedAtRaw: "2026-11-16T10:06:00Z"},
		{},
	}

	got := summarizePublicationTimes(candidates, observedAt)
	want := publicationTimeStats{
		Total: 7, RawPresent: 6, RawMissing: 1, Parsed: 3, DateOnly: 1,
		LocalDateTimeWithoutTimezone: 1, Unparseable: 1, ExplicitTimezone: 3, FutureOver5m: 1,
	}
	if got != want {
		t.Fatalf("publication-time stats=%+v want %+v", got, want)
	}
}
