package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSecurityTradingStatusAsOfRequiresLocalRevisionStorage(t *testing.T) {
	svc := &DataService{}
	asOf := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	_, err := svc.SecurityTradingStatusAsOf(context.Background(), "600519.SH", "2026-10-27", asOf)
	if !errors.Is(err, ErrSecurityTradingStatusAsOfUnavailable) {
		t.Fatalf("expected explicit PIT storage unavailable error, got %v", err)
	}
}

func TestSecurityTradingStatusAsOfValidatesFutureObservationTimeBeforeStorageLookup(t *testing.T) {
	svc := &DataService{}
	_, err := svc.SecurityTradingStatusAsOf(context.Background(), "600519.SH", "2026-10-27", time.Now().UTC().Add(time.Hour))
	if err == nil || errors.Is(err, ErrSecurityTradingStatusAsOfUnavailable) {
		t.Fatalf("future system as_of must be rejected first, got %v", err)
	}
}

func TestSecurityTradingStatusAsOfRejectsFutureTradeDate(t *testing.T) {
	svc := &DataService{}
	futureDate := time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	asOf := time.Now().UTC().Add(-time.Minute)
	_, err := svc.SecurityTradingStatusAsOf(context.Background(), "600519.SH", futureDate, asOf)
	if err == nil || errors.Is(err, ErrSecurityTradingStatusAsOfUnavailable) {
		t.Fatalf("future trade date must be rejected before storage lookup, got %v", err)
	}
}
