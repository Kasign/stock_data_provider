package newssearch

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type liveNewsSample struct {
	Title       string `json:"title"`
	Source      string `json:"source"`
	URL         string `json:"url"`
	PublishedAt string `json:"published_at"`
}

type liveNewsReport struct {
	Status      string           `json:"status"`
	Providers   []string         `json:"providers,omitempty"`
	WindowDays  int              `json:"window_days"`
	ElapsedMS   int64            `json:"elapsed_ms"`
	ResultCount int              `json:"result_count"`
	Samples     []liveNewsSample `json:"samples,omitempty"`
	Error       string           `json:"error,omitempty"`
}

func TestLiveNativeNewsSearch(t *testing.T) {
	if os.Getenv("GO_NATIVE_NEWS_LIVE_TESTS") != "1" {
		t.Skip("set GO_NATIVE_NEWS_LIVE_TESTS=1 to query configured native news providers")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	searcher := NewFromEnv(nil, logger)
	report := liveNewsReport{Status: "skipped", Providers: searcher.ProviderNames(), WindowDays: searcher.windowDays}
	if !searcher.Available() {
		report.Error = "no BOCHA_API_KEYS or valid SEARXNG_BASE_URLS configured"
		writeLiveNewsReport(t, report)
		t.Logf("LIVE_NEWS_RESULT %s", marshalLiveNewsReport(report))
		t.Skip("no native news-provider credentials configured; real search was not attempted")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := time.Now()
	items, err := searcher.searchNative(ctx, "贵州茅台 600519.SH 股票 最新消息", "600519.SH", "贵州茅台", 5)
	report.ElapsedMS = time.Since(started).Milliseconds()
	if err != nil {
		report.Status, report.Error = "failed", err.Error()
		writeLiveNewsReport(t, report)
		t.Logf("LIVE_NEWS_RESULT %s", marshalLiveNewsReport(report))
		t.Fatalf("native news provider probe failed: %v", err)
	}
	report.Status = "empty"
	if len(items) > 0 {
		report.Status = "returned"
	}
	report.ResultCount = len(items)
	for _, item := range items {
		report.Samples = append(report.Samples, liveNewsSample{
			Title: item.Title, Source: item.Source, URL: item.URL, PublishedAt: item.PublishedAt,
		})
	}
	writeLiveNewsReport(t, report)
	t.Logf("LIVE_NEWS_RESULT %s", marshalLiveNewsReport(report))
	if err := ctx.Err(); err != nil {
		t.Fatalf("native news probe context ended before completion: %v", err)
	}
}

func marshalLiveNewsReport(report liveNewsReport) string {
	data, err := json.Marshal(report)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func writeLiveNewsReport(t *testing.T, report liveNewsReport) {
	t.Helper()
	path := os.Getenv("GO_NEWS_LIVE_REPORT_PATH")
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Errorf("create live news report directory: %v", err)
		return
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Errorf("encode live news report: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Errorf("write live news report: %v", err)
	}
}
