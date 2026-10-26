package service

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type announcementsFakeProvider struct {
	snapshot *model.StockAnnouncementSnapshot
}

func (p announcementsFakeProvider) Name() string { return "eastmoney" }

func (p announcementsFakeProvider) FetchStockAnnouncements(context.Context, string, string, string, int, int) (*model.StockAnnouncementSnapshot, error) {
	return p.snapshot, nil
}

func TestStockAnnouncementsKeepsPublishTimeSeparateFromFetchedAt(t *testing.T) {
	now := time.Now().UTC()
	published := time.Date(2026, 10, 23, 10, 30, 0, 0, time.UTC)
	item := model.StockAnnouncement{
		ArticleID: "AN202610231234567890", Symbol: "600036.SH", StockName: "招商银行",
		Title: "招商银行第三季度报告", Category: "定期报告", AnnouncementDate: "2026-10-23",
		PublishedAt: &published, DisplayTime: "2026-10-23 18:30:00",
		SourceURL: "https://data.eastmoney.com/notices/detail/600036/AN202610231234567890.html",
		Source: "eastmoney", SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityValid,
	}
	snapshot := &model.StockAnnouncementSnapshot{
		Symbol: "600036.SH", StartDate: "2026-10-20", EndDate: "2026-10-26", Page: 1, PageSize: 20,
		Total: 1, Pages: 1, Returned: 1, HasData: true, HasMore: false,
		Items: []model.StockAnnouncement{item}, Source: "eastmoney", SourceVersion: "test",
		FetchedAt: now, QualityStatus: model.QualityValid,
	}
	p := announcementsFakeProvider{snapshot: snapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetAnnouncementProviders([]provider.AnnouncementProvider{p})
	got, hit, err := svc.StockAnnouncements(context.Background(), "600036.SH", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil || hit {
		t.Fatalf("StockAnnouncements hit=%v err=%v", hit, err)
	}
	if got.Items[0].PublishedAt == nil || !got.Items[0].PublishedAt.Equal(published) || !got.FetchedAt.Equal(now) {
		t.Fatalf("provider publish time was conflated with fetch time: %+v", got)
	}
}

func TestStockAnnouncementsEmptySourceRemainsUnknownAndDateRangeIsValidated(t *testing.T) {
	now := time.Now().UTC()
	snapshot := &model.StockAnnouncementSnapshot{
		Symbol: "600036.SH", StartDate: "2026-10-20", EndDate: "2026-10-26", Page: 1, PageSize: 20,
		Items: []model.StockAnnouncement{}, Source: "eastmoney", SourceVersion: "test", FetchedAt: now,
		QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_returned_empty_announcement_list_not_confirmed_as_no_announcements"},
	}
	p := announcementsFakeProvider{snapshot: snapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetAnnouncementProviders([]provider.AnnouncementProvider{p})
	got, hit, err := svc.StockAnnouncements(context.Background(), "600036", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil || hit || got.HasData || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("empty announcements should remain unknown: got=%+v hit=%v err=%v", got, hit, err)
	}
	if _, _, err := svc.StockAnnouncements(context.Background(), "600036", "2026-10-27", "2026-10-26", 1, 20); err == nil {
		t.Fatal("reversed announcement date range must be rejected")
	}
}
