package repository

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestValidISODate(t *testing.T) {
	for _, tc := range []struct {
		value    string
		optional bool
		want     bool
	}{
		{"2026-10-30", false, true}, {"20261030", false, false},
		{"2026-02-30", false, false}, {"", true, true}, {"", false, false},
	} {
		if got := validISODate(tc.value, tc.optional); got != tc.want {
			t.Errorf("validISODate(%q, %t) = %t, want %t", tc.value, tc.optional, got, tc.want)
		}
	}
}

func TestAdjustmentFactorHashIgnoresObservationMetadata(t *testing.T) {
	now := time.Now().UTC()
	value := model.AdjustmentFactor{Symbol: "600519.SH", TradeDate: "2026-10-30", Factor: 1.25, Source: "tushare", FetchedAt: now}
	first, err := adjustmentFactorHash(value)
	if err != nil {
		t.Fatal(err)
	}
	later := value
	later.FetchedAt = now.Add(time.Minute)
	later.SystemObservedAt = &now
	second, err := adjustmentFactorHash(later)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("fetch/observation timestamps must not create a new content revision")
	}
	later.Factor = 1.3
	third, err := adjustmentFactorHash(later)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("factor changes must create a new content revision")
	}
}

func TestCorporateActionHashIgnoresObservationMetadata(t *testing.T) {
	now := time.Now().UTC()
	cash := 0.5
	value := model.CorporateAction{Symbol: "600519.SH", ProviderEventID: "event-1", EventType: "cash_dividend", ExDate: "2026-10-30", Source: "tushare", FetchedAt: now, CashDividendCNY: &cash}
	first, err := corporateActionHash(value)
	if err != nil {
		t.Fatal(err)
	}
	later := value
	later.FetchedAt = now.Add(time.Minute)
	later.SystemObservedAt = &now
	second, err := corporateActionHash(later)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("fetch/observation timestamps must not create a new content revision")
	}
	changedCash := 0.6
	later.CashDividendCNY = &changedCash
	third, err := corporateActionHash(later)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("corporate-action term changes must create a new content revision")
	}
}
