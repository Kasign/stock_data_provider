package repository

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
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

func TestAdjustmentFactorHashIncludesBasis(t *testing.T) {
	value := model.AdjustmentFactor{Symbol: "600036.SH", TradeDate: "2025-06-30", Factor: 1.25, Source: "baostock", Basis: model.AdjustmentFactorForward}
	forwardHash, err := adjustmentFactorHash(value)
	if err != nil {
		t.Fatal(err)
	}
	value.Basis = model.AdjustmentFactorBackward
	backwardHash, err := adjustmentFactorHash(value)
	if err != nil {
		t.Fatal(err)
	}
	if forwardHash == backwardHash {
		t.Fatal("different factor bases must create independent content revisions")
	}
	value.Basis = ""
	emptyHash, err := adjustmentFactorHash(value)
	if err != nil {
		t.Fatal(err)
	}
	value.Basis = model.AdjustmentFactorShared
	sharedHash, err := adjustmentFactorHash(value)
	if err != nil {
		t.Fatal(err)
	}
	if emptyHash != sharedHash {
		t.Fatal("an omitted single-factor basis must normalize to shared")
	}
}

func TestValidateAdjustmentFactorRecordRequiresSourceAndValidTimeEvidence(t *testing.T) {
	fetched := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	available := fetched.Add(-time.Hour)
	providerTime := fetched.Add(-time.Minute)
	base := model.AdjustmentFactor{
		Symbol: "600519.SH", TradeDate: "2026-11-06", Factor: 1.25,
		Basis: model.AdjustmentFactorShared, Source: "tushare", SourceVersion: "tushare-adj-factor-v1",
		FetchedAt: fetched, AvailableAt: &available, ProviderTime: &providerTime,
		QualityStatus: model.QualityDegraded,
	}
	if err := validateAdjustmentFactorRecord(base); err != nil {
		t.Fatalf("valid factor record rejected: %v", err)
	}
	missingVersion := base
	missingVersion.SourceVersion = ""
	if err := validateAdjustmentFactorRecord(missingVersion); err == nil {
		t.Fatal("factor record without source version must be rejected at persistence boundary")
	}
	futureAvailable := base
	afterFetch := fetched.Add(time.Second)
	futureAvailable.AvailableAt = &afterFetch
	if err := validateAdjustmentFactorRecord(futureAvailable); err != nil {
		t.Fatalf("repository must preserve future availability revisions for PIT visibility filtering: %v", err)
	}
	zeroProviderTime := base
	zero := time.Time{}
	zeroProviderTime.ProviderTime = &zero
	if err := validateAdjustmentFactorRecord(zeroProviderTime); err == nil {
		t.Fatal("zero provider_time must be rejected when supplied")
	}
}

func TestAdjustmentFactorHashCanonicalizesTimestampLocation(t *testing.T) {
	instant := time.Date(2026, 11, 10, 8, 30, 0, 123456789, time.UTC)
	utc := instant
	local := instant.In(time.FixedZone("UTC+8", 8*60*60))
	base := model.AdjustmentFactor{
		Symbol: "600519.SH", TradeDate: "2026-11-09", Factor: 1.25,
		Source: "tushare", SourceVersion: "v1", FetchedAt: instant,
		ProviderTime: &utc, AvailableAt: &utc, QualityStatus: model.QualityValid,
	}
	first, err := adjustmentFactorHash(base)
	if err != nil {
		t.Fatal(err)
	}
	base.ProviderTime, base.AvailableAt = &local, &local
	second, err := adjustmentFactorHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equivalent instants in different time zones must have the same factor revision hash")
	}
	later := instant.Add(time.Second)
	base.AvailableAt = &later
	third, err := adjustmentFactorHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("availability timestamp changes must create a different factor revision hash")
	}
}

func TestCorporateActionHashCanonicalizesAvailabilityLocation(t *testing.T) {
	instant := time.Date(2026, 11, 10, 8, 30, 0, 123456789, time.UTC)
	utc := instant
	local := instant.In(time.FixedZone("UTC+8", 8*60*60))
	base := model.CorporateAction{
		Symbol: "600519.SH", ProviderEventID: "event-2", EventType: "cash_dividend",
		ExDate: "2026-11-09", Source: "tushare", AvailableAt: &utc,
	}
	first, err := corporateActionHash(base)
	if err != nil {
		t.Fatal(err)
	}
	base.AvailableAt = &local
	second, err := corporateActionHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equivalent availability instants in different time zones must have the same corporate-action revision hash")
	}
}

func TestAdjustmentFactorRevisionIdentityIncludesSourceVersion(t *testing.T) {
	base := model.AdjustmentFactor{
		Symbol: "600519.SH", TradeDate: "2026-11-09", Factor: 1.25,
		Basis: model.AdjustmentFactorShared, Source: "tushare", SourceVersion: "v1",
	}
	upgraded := base
	upgraded.SourceVersion = "v2"
	first, err := adjustmentFactorHash(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adjustmentFactorHash(upgraded)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("source-version changes must produce independent factor content identities")
	}
	identity := func(value model.AdjustmentFactor) string {
		return value.Symbol + "|" + value.TradeDate + "|" + value.Source + "|" + value.SourceVersion + "|" + string(value.Basis)
	}
	if identity(base) == identity(upgraded) {
		t.Fatal("different source versions must use distinct persistence identities")
	}
}
