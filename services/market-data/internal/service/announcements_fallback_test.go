package service

import (
	"strings"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type announcementFallbackTestProvider struct {
	name string
	snapshot *model.StockAnnouncementSnapshot
	err error
	calls int
}

func (p *announcementFallbackTestProvider) Name() string { return p.name }

func (p *announcementFallbackTestProvider) FetchStockAnnouncements(_ context.Context, symbol, startDate, endDate string, page, pageSize int) (*model.StockAnnouncementSnapshot, error) {
	p.calls++
	if p.err != nil { return nil, p.err }
	if p.snapshot == nil { return nil, errors.New("test announcement snapshot is nil") }
	out := *p.snapshot
	out.Symbol, out.StartDate, out.EndDate, out.Page, out.PageSize = symbol, startDate, endDate, page, pageSize
	out.Items = append([]model.StockAnnouncement(nil), p.snapshot.Items...)
	for i := range out.Items { out.Items[i].Symbol = symbol }
	return &out, nil
}

func unknownAnnouncementsForFallbackTest(source string) *model.StockAnnouncementSnapshot {
	return &model.StockAnnouncementSnapshot{
		Symbol: "600519.SH", StartDate: "2026-10-20", EndDate: "2026-10-26", Page: 1, PageSize: 20,
		Total: 0, Pages: 0, Returned: 0, HasData: false, HasMore: false, Items: []model.StockAnnouncement{},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_empty_announcements_unconfirmed"},
	}
}

func validAnnouncementsForFallbackTest(source string) *model.StockAnnouncementSnapshot {
	now := time.Now().UTC()
	return &model.StockAnnouncementSnapshot{
		Symbol: "600519.SH", StartDate: "2026-10-20", EndDate: "2026-10-26", Page: 1, PageSize: 20,
		Total: 1, Pages: 1, Returned: 1, HasData: true, HasMore: false,
		Items: []model.StockAnnouncement{{
			ArticleID: "test-20261026-001", Symbol: "600519.SH", StockName: "贵州茅台",
			Title: "测试公告", Category: "公司公告", AnnouncementDate: "2026-10-26",
			SourceURL: "https://example.invalid/notice/test-20261026-001",
			Source: source, SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_publish_time_not_exposed"},
		}},
		Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_publish_time_not_exposed"},
	}
}

func TestStockAnnouncementsFallsBackFromUnknownQualityRows(t *testing.T) {
	firstSnapshot := validAnnouncementsForFallbackTest("eastmoney")
	firstSnapshot.QualityStatus = model.QualityUnknown
	firstSnapshot.QualityReasons = []string{"provider_announcement_quality_unknown"}
	first := &announcementFallbackTestProvider{name: "eastmoney", snapshot: firstSnapshot}
	fallback := &announcementFallbackTestProvider{name: "hithink", snapshot: validAnnouncementsForFallbackTest("hithink")}
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetAnnouncementProviders([]provider.AnnouncementProvider{first, fallback})
	got, hit, err := svc.StockAnnouncements(context.Background(), "600519.SH", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil || hit { t.Fatalf("StockAnnouncements returned hit=%v err=%v", hit, err) }
	if got == nil || got.Source != "hithink" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected degraded fallback rather than unknown announcement rows, got %+v", got)
	}
	if first.calls != 1 || fallback.calls != 1 { t.Fatalf("expected both providers to be attempted, calls=%d/%d", first.calls, fallback.calls) }
}

func TestStockAnnouncementsReturnsUnknownRowsOnlyAfterFallbackExhaustion(t *testing.T) {
	candidate := validAnnouncementsForFallbackTest("eastmoney")
	candidate.QualityStatus = model.QualityUnknown
	candidate.QualityReasons = []string{"provider_announcement_quality_unknown"}
	first := &announcementFallbackTestProvider{name: "eastmoney", snapshot: candidate}
	failed := &announcementFallbackTestProvider{name: "hithink", err: errors.New("test provider unavailable")}
	mem := newMemoryCache()
	svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetAnnouncementProviders([]provider.AnnouncementProvider{first, failed})
	got, hit, err := svc.StockAnnouncements(context.Background(), "600519.SH", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil || hit { t.Fatalf("StockAnnouncements returned hit=%v err=%v", hit, err) }
	if got == nil || !got.HasData || got.QualityStatus != model.QualityUnknown ||
		!strings.Contains(strings.Join(got.QualityReasons, ","), "announcements_fallback_exhausted_without_usable_quality") {
		t.Fatalf("expected retained unknown rows after fallback exhaustion, got %+v", got)
	}
	if first.calls != 1 || failed.calls != 1 { t.Fatalf("expected all providers to be attempted, calls=%d/%d", first.calls, failed.calls) }
}

func TestStockAnnouncementsFallsBackAfterUnknownEmptyProvider(t *testing.T) {
	first := &announcementFallbackTestProvider{name: "eastmoney", snapshot: unknownAnnouncementsForFallbackTest("eastmoney")}
	second := &announcementFallbackTestProvider{name: "hithink", snapshot: validAnnouncementsForFallbackTest("hithink")}
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetAnnouncementProviders([]provider.AnnouncementProvider{first, second})
	got, hit, err := svc.StockAnnouncements(context.Background(), "600519.SH", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil || hit { t.Fatalf("StockAnnouncements returned hit=%v err=%v", hit, err) }
	if got == nil || !got.HasData || got.Source != "hithink" || first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected fallback announcement, snapshot=%+v calls=%d/%d", got, first.calls, second.calls)
	}
}

func TestStockAnnouncementsReturnsUnknownOnlyAfterFallbackExhaustion(t *testing.T) {
	first := &announcementFallbackTestProvider{name: "eastmoney", snapshot: unknownAnnouncementsForFallbackTest("eastmoney")}
	second := &announcementFallbackTestProvider{name: "hithink", err: errors.New("test provider unavailable")}
	mem := newMemoryCache()
	svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetAnnouncementProviders([]provider.AnnouncementProvider{first, second})
	got, hit, err := svc.StockAnnouncements(context.Background(), "600519.SH", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil || hit { t.Fatalf("StockAnnouncements returned hit=%v err=%v", hit, err) }
	if got == nil || got.HasData || got.QualityStatus != model.QualityUnknown { t.Fatalf("expected unknown empty response, got %+v", got) }
	found := false
	for _, reason := range got.QualityReasons { if reason == "announcements_fallback_exhausted_without_confirmed_data" { found = true } }
	if !found || first.calls != 1 || second.calls != 1 { t.Fatalf("missing fallback exhaustion marker or incomplete fallback: reasons=%v calls=%d/%d", got.QualityReasons, first.calls, second.calls) }
}


func TestValidateStockAnnouncementSnapshotRejectsUnderfilledPage(t *testing.T) {
	snapshot := validAnnouncementsForFallbackTest("eastmoney")
	snapshot.Total = 21
	snapshot.Pages = 2
	snapshot.HasMore = true

	err := validateStockAnnouncementSnapshot("600519.SH", "2026-10-20", "2026-10-26", 1, 20, snapshot)
	if err == nil || !strings.Contains(err.Error(), "returned rows are inconsistent") {
		t.Fatalf("announcement page with fewer rows than source total requires must be rejected, got %v", err)
	}
}
