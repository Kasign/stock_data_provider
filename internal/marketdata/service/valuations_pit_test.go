package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
)

func TestValuationsAsOfRequiresRevisionStorage(t *testing.T) {
	svc := &DataService{}
	_, err := svc.ValuationsAsOf(context.Background(), []string{"600519.SH"}, time.Now().UTC())
	if !errors.Is(err, ErrValuationsAsOfUnavailable) {
		t.Fatalf("expected explicit unavailable-storage error, got %v", err)
	}
}

func TestValuationsAsOfRejectsInvalidCutoffBeforeStorageAccess(t *testing.T) {
	svc := &DataService{}
	_, err := svc.ValuationsAsOf(context.Background(), []string{"600519.SH"}, time.Time{})
	if err == nil || errors.Is(err, ErrValuationsAsOfUnavailable) {
		t.Fatalf("zero as_of must be rejected before storage lookup, got %v", err)
	}
}

func TestValuationSnapshotEligibleAsOfRequiresAllRecordedTimesAtCutoff(t *testing.T) {
	asOf := time.Date(2026, 11, 6, 10, 0, 0, 0, marketLocation())
	providerTime := asOf.Add(-time.Minute)
	observedAt := asOf.Add(-30 * time.Second)
	snapshot := validValuationsForFallbackTest("eastmoney")
	snapshot.FetchedAt = asOf.Add(-20 * time.Second)
	snapshot.Items[0].FetchedAt = asOf.Add(-10 * time.Second)
	snapshot.ProviderTimestamp = &providerTime
	snapshot.ProviderReportDate = asOf.Format("2006-01-02")
	snapshot.SystemObservedAt = &observedAt

	if !valuationSnapshotEligibleAsOf([]string{"600519.SH"}, snapshot, asOf) {
		t.Fatal("snapshot with all evidence at or before as_of should be eligible")
	}

	tests := []struct {
		name   string
		mutate func(*model.ValuationSnapshot)
	}{
		{name: "snapshot fetched after cutoff", mutate: func(value *model.ValuationSnapshot) {
			value.FetchedAt = asOf.Add(time.Nanosecond)
		}},
		{name: "item fetched after cutoff", mutate: func(value *model.ValuationSnapshot) {
			value.Items[0].FetchedAt = asOf.Add(time.Nanosecond)
		}},
		{name: "item fetch time missing", mutate: func(value *model.ValuationSnapshot) {
			value.Items[0].FetchedAt = time.Time{}
		}},
		{name: "system observation after cutoff", mutate: func(value *model.ValuationSnapshot) {
			after := asOf.Add(time.Nanosecond)
			value.SystemObservedAt = &after
		}},
		{name: "provider timestamp after cutoff", mutate: func(value *model.ValuationSnapshot) {
			after := asOf.Add(time.Nanosecond)
			value.ProviderTimestamp = &after
		}},
		{name: "provider report date after cutoff date", mutate: func(value *model.ValuationSnapshot) {
			value.ProviderReportDate = asOf.AddDate(0, 0, 1).Format("2006-01-02")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := *snapshot
			candidate.Items = append([]model.ValuationItem(nil), snapshot.Items...)
			test.mutate(&candidate)
			if valuationSnapshotEligibleAsOf([]string{"600519.SH"}, &candidate, asOf) {
				t.Fatal("snapshot with evidence after or missing at as_of must be excluded")
			}
		})
	}
}

func TestValuationsAsOfReadsHistoricalRevisionFromMariaDBWithoutProviderFallback(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	source := &valuationFallbackTestProvider{
		name: "eastmoney", snapshot: validValuationsForFallbackTest("eastmoney"),
	}
	svc := NewWithCapabilities(
		nil, nil, nil, cache.NewDisabled(), store,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil,
	)
	svc.SetValuationProviders([]provider.ValuationProvider{source})

	first, hit, err := svc.Valuations(ctx, []string{"600519.SH"})
	if err != nil || hit {
		t.Fatalf("first Valuations hit=%v err=%v", hit, err)
	}
	if first.QualityStatus != model.QualityDegraded {
		t.Fatalf("fixture must retain degraded quality, got %s", first.QualityStatus)
	}
	firstValue := *first.Items[0].PETTM
	firstAsOf := time.Now().UTC().Add(time.Millisecond)

	time.Sleep(5 * time.Millisecond)
	revised := validValuationsForFallbackTest("eastmoney")
	changed := firstValue + 1
	revised.Items[0].PETTM = &changed
	source.snapshot = revised
	latest, hit, err := svc.Valuations(ctx, []string{"600519.SH"})
	if err != nil || hit {
		t.Fatalf("revised Valuations hit=%v err=%v", hit, err)
	}
	if latest.Items[0].PETTM == nil || *latest.Items[0].PETTM != changed {
		t.Fatalf("live revised snapshot must expose the updated metric %v, got %+v", changed, latest.Items[0].PETTM)
	}
	latestAsOf := time.Now().UTC().Add(time.Millisecond)

	beforeFirst, err := svc.ValuationsAsOf(ctx, []string{"600519.SH"}, firstAsOf.Add(-time.Minute))
	if !errors.Is(err, ErrValuationsAsOfNotFound) || beforeFirst != nil {
		t.Fatalf("as_of before first observation must be not-found, snapshot=%+v err=%v", beforeFirst, err)
	}
	historical, err := svc.ValuationsAsOf(ctx, []string{"600519.SH"}, firstAsOf)
	if err != nil {
		t.Fatalf("read first valuation revision: %v", err)
	}
	if historical.Items[0].PETTM == nil || *historical.Items[0].PETTM != firstValue {
		t.Fatalf("historical as_of must restore first observed value %v, got %+v", firstValue, historical.Items[0].PETTM)
	}
	if historical.SystemObservedAt == nil || historical.SystemObservedAt.After(firstAsOf) {
		t.Fatalf("historical result must restore its system observation timestamp: %+v", historical.SystemObservedAt)
	}
	if historical.QualityStatus != model.QualityDegraded {
		t.Fatalf("PIT read must preserve degraded source quality, got %s", historical.QualityStatus)
	}

	current, err := svc.ValuationsAsOf(ctx, []string{"600519.SH"}, latestAsOf)
	if err != nil {
		t.Fatalf("read latest valuation revision: %v", err)
	}
	if current.Items[0].PETTM == nil || *current.Items[0].PETTM != changed {
		t.Fatalf("latest as_of must return revised value %v, got %+v", changed, current.Items[0].PETTM)
	}
	if source.calls != 2 {
		t.Fatalf("PIT reads must not call live Provider; total provider calls=%d", source.calls)
	}
}
