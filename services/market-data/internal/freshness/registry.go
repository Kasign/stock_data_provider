package freshness

import (
	"strings"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

const (
	DomainQuote         = "quote"
	DomainIndex         = "index"
	DomainMarketBreadth = "market_breadth"
	DomainSector        = "sector"
	DomainFundFlow      = "fund_flow"
)

var (
	realTimeAuctionPhases = []string{
		model.MarketSessionOpeningAuction,
		model.MarketSessionContinuousAM,
		model.MarketSessionContinuousPM,
		model.MarketSessionClosingAuction,
	}
	continuousMarketPhases = []string{
		model.MarketSessionContinuousAM,
		model.MarketSessionContinuousPM,
	}
)

// PolicyForDomain returns the centrally maintained initial SLA for a data domain.
// The returned phase slice is copied so callers cannot mutate the registry.
func PolicyForDomain(domain string) (Policy, bool) {
	var policy Policy
	switch strings.TrimSpace(domain) {
	case DomainQuote:
		policy = Policy{Domain: DomainQuote, MaxAge: 90 * time.Second, MaxFutureSkew: 2 * time.Minute,
			ActivePhases: realTimeAuctionPhases, RequireTradingCalendar: true, IgnoreWhenSecurityPaused: true}
	case DomainIndex:
		policy = Policy{Domain: DomainIndex, MaxAge: 120 * time.Second, MaxFutureSkew: 2 * time.Minute,
			ActivePhases: realTimeAuctionPhases, RequireTradingCalendar: true}
	case DomainMarketBreadth:
		policy = Policy{Domain: DomainMarketBreadth, MaxAge: 5 * time.Minute, MaxFutureSkew: 2 * time.Minute,
			ActivePhases: continuousMarketPhases, RequireTradingCalendar: true}
	case DomainSector:
		policy = Policy{Domain: DomainSector, MaxAge: 5 * time.Minute, MaxFutureSkew: 2 * time.Minute,
			ActivePhases: continuousMarketPhases, RequireTradingCalendar: true}
	case DomainFundFlow:
		policy = Policy{Domain: DomainFundFlow, MaxAge: 5 * time.Minute, MaxFutureSkew: 2 * time.Minute,
			ActivePhases: continuousMarketPhases, RequireTradingCalendar: true}
	default:
		return Policy{}, false
	}
	policy.ActivePhases = append([]string(nil), policy.ActivePhases...)
	return policy, true
}

// QuoteEvidence maps only the provider's explicit quote snapshot time.
func QuoteEvidence(quote *model.Quote) Evidence {
	if quote == nil {
		return Evidence{}
	}
	return Evidence{DataTime: quote.QuoteTime, DataTimeType: strings.TrimSpace(quote.QuoteTimeSemantics),
		FetchedAt: timePointer(quote.FetchedAt)}
}

// IndexQuoteEvidence maps the index provider's explicit quote snapshot time.
func IndexQuoteEvidence(quote *model.MarketIndexQuote) Evidence {
	if quote == nil {
		return Evidence{}
	}
	return Evidence{DataTime: quote.QuoteTime, DataTimeType: strings.TrimSpace(quote.QuoteTimeSemantics),
		FetchedAt: timePointer(quote.FetchedAt)}
}

// MarketBreadthEvidence uses the oldest covered quote timestamp to avoid hiding stale members.
func MarketBreadthEvidence(snapshot *model.MarketBreadthSnapshot) Evidence {
	if snapshot == nil {
		return Evidence{}
	}
	return Evidence{DataTime: snapshot.OldestQuoteTime, DataTimeType: "oldest_component_quote_time",
		FetchedAt: timePointer(snapshot.FetchedAt)}
}

// SectorEvidence does not convert ProviderReportDate into a precise source timestamp.
func SectorEvidence(snapshot *model.SectorRankingSnapshot) Evidence {
	if snapshot == nil {
		return Evidence{}
	}
	return Evidence{FetchedAt: timePointer(snapshot.FetchedAt)}
}

// FundFlowEvidence does not convert TradeDate into a precise source timestamp.
func FundFlowEvidence(snapshot *model.MarketCapitalFlowSnapshot) Evidence {
	if snapshot == nil {
		return Evidence{}
	}
	return Evidence{FetchedAt: timePointer(snapshot.FetchedAt)}
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}
