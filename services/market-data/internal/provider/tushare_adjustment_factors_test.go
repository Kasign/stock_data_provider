package provider

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestTushareFetchAdjustmentFactorsRequiresToken(t *testing.T) {
	p := NewTushareConfigured(time.Second, "", "")
	if _, err := p.FetchAdjustmentFactors(context.Background(), "600519.SH", "2026-10-01", "2026-10-30"); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestTushareAdjustmentFactorModelKeepsAvailabilityUnknown(t *testing.T) {
	value := model.AdjustmentFactor{Source:"tushare", SourceVersion:tushareVersion, FetchedAt:time.Now().UTC(), QualityStatus:model.QualityDegraded}
	if value.AvailableAt != nil { t.Fatal("Tushare adj_factor does not expose first-publication timestamp; available_at must remain unknown") }
}
