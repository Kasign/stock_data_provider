package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type dragonTigerFakeProvider struct {
	snapshot *model.DragonTigerSnapshot
}

func (p dragonTigerFakeProvider) Name() string { return "eastmoney" }

func (p dragonTigerFakeProvider) FetchDragonTiger(context.Context, string, int, int) (*model.DragonTigerSnapshot, error) {
	return p.snapshot, nil
}

func TestDragonTigerValidatesDateAndPreservesSignedNetBuy(t *testing.T) {
	now := time.Now().UTC()
	change, closePrice, netBuy := 2.5, 10.25, -12345.0
	item := model.DragonTigerItem{
		Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", TradeDate: "2026-10-22",
		ChangePercent: &change, ClosePrice: &closePrice, NetBuyAmountCNY: &netBuy,
		Reason: "测试上榜原因", Source: "eastmoney", SourceVersion: "test", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
	snapshot := &model.DragonTigerSnapshot{
		TradeDate: "2026-10-22", ProviderReportDate: "2026-10-22", Page: 1, PageSize: 100,
		Total: 1, Pages: 1, Returned: 1, SourceReturned: 1, ExcludedNonAShareCount: 0, HasData: true, HasMore: false,
		Items: []model.DragonTigerItem{item}, Source: "eastmoney", SourceVersion: "test",
		FetchedAt: now, QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
	versionMismatch := *snapshot
	versionMismatch.Items = append([]model.DragonTigerItem(nil), snapshot.Items...)
	versionMismatch.Items[0].SourceVersion = "older-version"
	if err := validateDragonTigerSnapshot("2026-10-22", 1, 100, &versionMismatch); err == nil {
		t.Fatal("dragon-tiger item source version must match parent snapshot")
	}
	qualityMismatch := *snapshot
	qualityMismatch.QualityStatus = model.QualityValid
	if err := validateDragonTigerSnapshot("2026-10-22", 1, 100, &qualityMismatch); err == nil {
		t.Fatal("dragon-tiger snapshot must not be valid when its item is degraded")
	}
	p := dragonTigerFakeProvider{snapshot: snapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000,
		ResilienceConfig{Retries: 0, CircuitFailureThreshold: 3, CircuitCooldown: time.Second}, nil)
	svc.SetDragonTigerProviders([]provider.DragonTigerProvider{p})
	got, hit, err := svc.DragonTiger(context.Background(), "2026-10-22", 1, 100)
	if err != nil || hit {
		t.Fatalf("DragonTiger hit=%v err=%v", hit, err)
	}
	if got.Items[0].NetBuyAmountCNY == nil || *got.Items[0].NetBuyAmountCNY != -12345 {
		t.Fatalf("negative net buy must survive: %+v", got.Items[0])
	}
	if _, _, err = svc.DragonTiger(context.Background(), "2026-10-21", 1, 100); err == nil {
		t.Fatal("must reject provider data for a different requested date")
	}
}

func TestValidateDragonTigerSnapshotAllowsBondOnlyPageAsUnknown(t *testing.T) {
	now := time.Now().UTC()
	snapshot := &model.DragonTigerSnapshot{
		TradeDate: "2026-10-23", ProviderReportDate: "2026-10-23", Page: 1, PageSize: 100,
		Total: 1, Pages: 1, Returned: 0, SourceReturned: 1, ExcludedNonAShareCount: 1,
		HasData: false, HasMore: false, Items: []model.DragonTigerItem{},
		Source: "eastmoney", SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityUnknown,
		QualityReasons: []string{"provider_non_a_share_security_rows_excluded", "provider_page_contains_no_supported_a_share_rows"},
	}
	if err := validateDragonTigerSnapshot("2026-10-23", 1, 100, snapshot); err != nil {
		t.Fatalf("all-non-A-share page should remain a valid unknown page: %v", err)
	}
}

func TestValidateDragonTigerSnapshotRejectsMissingRowsForNonzeroTotal(t *testing.T) {
	now := time.Now().UTC()
	snapshot := &model.DragonTigerSnapshot{
		TradeDate: "2026-10-22", ProviderReportDate: "2026-10-22", Page: 1, PageSize: 100,
		Total: 5, Pages: 1, Returned: 0, HasData: true, Items: []model.DragonTigerItem{},
		Source: "eastmoney", SourceVersion: "test", FetchedAt: now, QualityStatus: model.QualityDegraded,
	}
	if err := validateDragonTigerSnapshot("2026-10-22", 1, 100, snapshot); err == nil {
		t.Fatal("nonzero total and empty page must be rejected")
	}
}

func TestValidateDragonTigerSnapshotRejectsUnderfilledSourcePage(t *testing.T) {
	now := time.Now().UTC()
	change, closePrice := 1.5, 10.0
	item := model.DragonTigerItem{
		Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", TradeDate: "2026-10-22",
		ChangePercent: &change, ClosePrice: &closePrice,
		Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
	}
	snapshot := &model.DragonTigerSnapshot{
		TradeDate: "2026-10-22", ProviderReportDate: "2026-10-22", Page: 1, PageSize: 2,
		Total: 3, Pages: 2, Returned: 1, SourceReturned: 1, ExcludedNonAShareCount: 0,
		HasData: true, HasMore: true, Items: []model.DragonTigerItem{item},
		Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
	}
	if err := validateDragonTigerSnapshot("2026-10-22", 1, 2, snapshot); err == nil || !strings.Contains(err.Error(), "source row count") {
		t.Fatalf("dragon-tiger source page below the count-implied size must be rejected, got %v", err)
	}
}
