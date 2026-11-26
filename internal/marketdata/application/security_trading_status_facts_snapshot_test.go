package application

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func validTradingStatusRecord() *model.SecurityTradingStatusSnapshot {
	return &model.SecurityTradingStatusSnapshot{
		Symbol:             "600519.SH",
		TradeDate:          "2026-11-25",
		RiskWarningStatus:  model.RiskWarningNormal,
		DelistingStatus:    model.DelistingStatusNormal,
		ListingDate:        "2001-08-27",
		ListingStatusKnown: true,
		IsTradingDay:       true,
		TradingDayKnown:    true,
		SuspensionStatus:   model.SuspensionStatusNoEvent,
		Source:             "tushare",
		SourceVersion:      "tushare-status-v1",
		FetchedAt:          time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC),
		SystemObservedAt:   utcPointer(time.Date(2026, 11, 25, 9, 0, 5, 0, time.UTC)),
		QualityStatus:      model.QualityValid,
	}
}

func utcPointer(value time.Time) *time.Time { return &value }

func TestBuildTradingStatusFactsPayloadMapsAllAvailableFacts(t *testing.T) {
	payload, err := buildTradingStatusFactsPayload(validTradingStatusRecord(), "600519.SH", "2026-11-25")
	if err != nil {
		t.Fatalf("buildTradingStatusFactsPayload() error = %v", err)
	}
	if payload.QualityStatus != app.QualityValid || len(payload.Facts) != 5 {
		t.Fatalf("unexpected payload quality/facts: %+v", payload)
	}
	wantKeys := []string{"risk_warning_status", "suspension_status", "delisting_status", "listing_date", "is_trading_day"}
	wantValues := []string{"normal", "no_suspension_event", "normal", "2001-08-27", "true"}
	for index, fact := range payload.Facts {
		if fact.Key != wantKeys[index] || fact.Availability != app.TradingFactAvailable || fact.Value != wantValues[index] {
			t.Fatalf("fact %d = %+v, want key=%q value=%q", index, fact, wantKeys[index], wantValues[index])
		}
	}
}

func TestBuildTradingStatusFactsPayloadMarksUnknownEnumsAsUnavailable(t *testing.T) {
	record := validTradingStatusRecord()
	record.RiskWarningStatus = model.RiskWarningUnknown
	record.SuspensionStatus = model.SuspensionStatusUnknown
	record.DelistingStatus = model.DelistingStatusUnknown
	record.ListingStatusKnown = false
	record.ListingDate = ""
	record.TradingDayKnown = false

	payload, err := buildTradingStatusFactsPayload(record, "600519.SH", "2026-11-25")
	if err != nil {
		t.Fatalf("buildTradingStatusFactsPayload() error = %v", err)
	}
	if payload.QualityStatus != app.QualityDegraded {
		t.Fatalf("quality_status = %q, want degraded", payload.QualityStatus)
	}
	wantReasons := []string{
		"risk_warning_status_unknown", "suspension_status_unknown", "delisting_status_unknown",
		"listing_status_unknown", "trading_day_unknown",
	}
	for index, fact := range payload.Facts {
		if fact.Availability != app.TradingFactUnavailable || fact.UnavailableReason != wantReasons[index] || fact.Value != "" {
			t.Fatalf("fact %d = %+v, want unavailable with reason %q", index, fact, wantReasons[index])
		}
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("derived payload must satisfy its own validator: %v", err)
	}
}

func TestBuildTradingStatusFactsPayloadKeepsNonTradingDayAvailable(t *testing.T) {
	record := validTradingStatusRecord()
	record.IsTradingDay = false
	payload, err := buildTradingStatusFactsPayload(record, "600519.SH", "2026-11-25")
	if err != nil {
		t.Fatalf("buildTradingStatusFactsPayload() error = %v", err)
	}
	last := payload.Facts[4]
	// "不是交易日"是已核验的事实，不是缺失：写成 unavailable 会让下游误以为没查过。
	if last.Availability != app.TradingFactAvailable || last.Value != "false" {
		t.Fatalf("is_trading_day = %+v, want available/false", last)
	}
	if payload.QualityStatus != app.QualityValid {
		t.Fatalf("quality_status = %q, want valid", payload.QualityStatus)
	}
}

func TestBuildTradingStatusFactsPayloadRejectsUnusableRecords(t *testing.T) {
	for name, mutate := range map[string]func(*model.SecurityTradingStatusSnapshot){
		"invalid quality":     func(record *model.SecurityTradingStatusSnapshot) { record.QualityStatus = model.QualityInvalid },
		"unknown quality":     func(record *model.SecurityTradingStatusSnapshot) { record.QualityStatus = model.QualityUnknown },
		"missing source":      func(record *model.SecurityTradingStatusSnapshot) { record.Source = "" },
		"symbol mismatch":     func(record *model.SecurityTradingStatusSnapshot) { record.Symbol = "000001.SZ" },
		"trade date mismatch": func(record *model.SecurityTradingStatusSnapshot) { record.TradeDate = "2026-11-24" },
	} {
		t.Run(name, func(t *testing.T) {
			record := validTradingStatusRecord()
			mutate(record)
			if _, err := buildTradingStatusFactsPayload(record, "600519.SH", "2026-11-25"); err == nil {
				t.Fatal("buildTradingStatusFactsPayload() accepted an unusable source record")
			}
		})
	}
	if _, err := buildTradingStatusFactsPayload(nil, "600519.SH", "2026-11-25"); err == nil {
		t.Fatal("buildTradingStatusFactsPayload() accepted a nil record")
	}
}

// Data 判定为 degraded、但五项事实恰好齐全时不得输出 valid：
// 那等于把 Data 的降级结论替它改判成可用。
func TestBuildTradingStatusFactsPayloadRejectsDegradedRecordWithCompleteFacts(t *testing.T) {
	record := validTradingStatusRecord()
	record.QualityStatus = model.QualityDegraded
	if _, err := buildTradingStatusFactsPayload(record, "600519.SH", "2026-11-25"); err == nil {
		t.Fatal("degraded record with complete facts was upgraded to a valid payload")
	}
}

func TestBuildTradingStatusFactsPayloadNeverDerivesPriceLimitOrTradableVolume(t *testing.T) {
	payload, err := buildTradingStatusFactsPayload(validTradingStatusRecord(), "600519.SH", "2026-11-25")
	if err != nil {
		t.Fatalf("buildTradingStatusFactsPayload() error = %v", err)
	}
	for _, fact := range payload.Facts {
		if fact.Key == "price_limit" || fact.Key == "tradable_volume" {
			t.Fatalf("payload must not carry derived fact %q", fact.Key)
		}
	}
}

func TestTradingStatusFactsSourcesKeepOnlyRecordedSource(t *testing.T) {
	record := validTradingStatusRecord()
	record.SourceVersion = ""
	sources := tradingStatusFactsSources(record)
	if len(sources) != 1 || sources[0].SourceRef != "tushare" || sources[0].SourceVersion != nil {
		t.Fatalf("sources = %+v, want source ref without invented version", sources)
	}
}
