package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type securityTradingStatusTestProvider struct {
	name     string
	snapshot *model.SecurityTradingStatusSnapshot
	err      error
	calls    int
}

func (p *securityTradingStatusTestProvider) Name() string { return p.name }

func (p *securityTradingStatusTestProvider) FetchSecurityTradingStatus(context.Context, string, string) (*model.SecurityTradingStatusSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, errors.New("test status is nil")
	}
	out := *p.snapshot
	out.QualityReasons = append([]string(nil), p.snapshot.QualityReasons...)
	return &out, nil
}

var _ provider.SecurityTradingStatusProvider = (*securityTradingStatusTestProvider)(nil)

func validSecurityTradingStatusFixture(source string) *model.SecurityTradingStatusSnapshot {
	return &model.SecurityTradingStatusSnapshot{
		Symbol: "600519.SH", TradeDate: "2026-10-22",
		SecurityName: "*ST贵州", NameEffectiveFrom: "2026-10-20",
		RiskWarningStatus: model.RiskWarningApplied, DelistingStatus: model.DelistingStatusNormal,
		ListingDate: "2026-10-20", ListingTradingDayNumber: 3, ListingStatusKnown: true,
		IsTradingDay: true, TradingDayKnown: true,
		SuspensionStatus: model.SuspensionStatusIntraday, SuspensionTiming: "09:30-10:00", SuspensionType: "S",
		Source: source, SourceVersion: "fixture-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityValid,
	}
}

func TestSecurityTradingStatusReturnsValidatedHistoricalEvidence(t *testing.T) {
	first := &securityTradingStatusTestProvider{name: "bad-source", snapshot: validSecurityTradingStatusFixture("different-source")}
	second := &securityTradingStatusTestProvider{name: "good-source", snapshot: validSecurityTradingStatusFixture("good-source")}
	svc := &DataService{securityTradingStatusProviders: []provider.SecurityTradingStatusProvider{first, second}}
	got, err := svc.SecurityTradingStatus(context.Background(), "600519.SH", "2026-10-22")
	if err != nil {
		t.Fatalf("SecurityTradingStatus: %v", err)
	}
	if got.Source != "good-source" || got.RiskWarningStatus != model.RiskWarningApplied ||
		got.SuspensionStatus != model.SuspensionStatusIntraday || got.ListingTradingDayNumber != 3 {
		t.Fatalf("unexpected status result: %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected invalid provider to fall back to the next provider, calls=%d/%d", first.calls, second.calls)
	}
}

func TestSecurityTradingStatusKeepsUnknownEvidenceUnknown(t *testing.T) {
	snapshot := validSecurityTradingStatusFixture("unknown-source")
	snapshot.RiskWarningStatus = model.RiskWarningUnknown
	snapshot.DelistingStatus = model.DelistingStatusUnknown
	snapshot.SecurityName = ""
	snapshot.NameEffectiveFrom = ""
	snapshot.ListingStatusKnown = false
	snapshot.ListingTradingDayNumber = 0
	snapshot.SuspensionStatus = model.SuspensionStatusUnknown
	snapshot.QualityStatus = model.QualityUnknown
	p := &securityTradingStatusTestProvider{name: "unknown-source", snapshot: snapshot}
	svc := &DataService{securityTradingStatusProviders: []provider.SecurityTradingStatusProvider{p}}
	got, err := svc.SecurityTradingStatus(context.Background(), "600519.SH", "2026-10-22")
	if err != nil {
		t.Fatalf("incomplete evidence should remain an explicit result, got %v", err)
	}
	if got.RiskWarningStatus != model.RiskWarningUnknown || got.SuspensionStatus != model.SuspensionStatusUnknown ||
		got.QualityStatus != model.QualityUnknown {
		t.Fatalf("unknown inputs were promoted to normal/valid: %+v", got)
	}
}

func TestSecurityTradingStatusRejectsFutureDateAndMissingProvider(t *testing.T) {
	svc := &DataService{}
	_, err := svc.SecurityTradingStatus(context.Background(), "600519.SH", "2026-10-22")
	if err == nil || !strings.Contains(err.Error(), "no enabled") {
		t.Fatalf("missing provider should fail explicitly, got %v", err)
	}
	pastSnapshot := validSecurityTradingStatusFixture("future-date-provider")
	futureProvider := &securityTradingStatusTestProvider{name: "future-date-provider", snapshot: pastSnapshot}
	svc.SetSecurityTradingStatusProviders([]provider.SecurityTradingStatusProvider{futureProvider})
	futureDate := time.Now().In(marketLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	_, err = svc.SecurityTradingStatus(context.Background(), "600519.SH", futureDate)
	if err == nil || !strings.Contains(err.Error(), "must not be in the future") {
		t.Fatalf("future date must be rejected before provider calls, got %v", err)
	}
	if futureProvider.calls != 0 {
		t.Fatalf("future date must not call provider, calls=%d", futureProvider.calls)
	}
}
