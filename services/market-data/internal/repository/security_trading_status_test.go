package repository

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestSecurityTradingStatusRevisionHashIgnoresObservationTimestamps(t *testing.T) {
	fetchedAt := time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC)
	observedAt := time.Date(2026, 10, 28, 2, 0, 2, 0, time.UTC)
	snapshot := model.SecurityTradingStatusSnapshot{
		Symbol: "600519.SH", TradeDate: "2026-10-27",
		SecurityName: "贵州茅台", NameEffectiveFrom: "2001-08-27",
		RiskWarningStatus: model.RiskWarningNormal, DelistingStatus: model.DelistingStatusNormal,
		ListingDate: "2001-08-27", ListingTradingDayNumber: 6000, ListingStatusKnown: true,
		IsTradingDay: true, TradingDayKnown: true, SuspensionStatus: model.SuspensionStatusNoEvent,
		Source: "tushare", SourceVersion: "tushare-namechange-suspend-d-trade-cal-v1",
		FetchedAt: fetchedAt, SystemObservedAt: &observedAt, QualityStatus: model.QualityValid,
	}
	first, err := securityTradingStatusRevisionHash(snapshot)
	if err != nil { t.Fatal(err) }
	snapshot.FetchedAt = fetchedAt.Add(5 * time.Minute)
	laterObserved := observedAt.Add(5 * time.Minute)
	snapshot.SystemObservedAt = &laterObserved
	second, err := securityTradingStatusRevisionHash(snapshot)
	if err != nil { t.Fatal(err) }
	if first != second {
		t.Fatal("fetch/record timestamps alone must not create a new content revision")
	}
	snapshot.SuspensionStatus = model.SuspensionStatusSuspended
	third, err := securityTradingStatusRevisionHash(snapshot)
	if err != nil { t.Fatal(err) }
	if first == third {
		t.Fatal("changed suspension evidence must create a new content revision")
	}
}
