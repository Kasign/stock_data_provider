package application

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/risk"
)

func validTradingFactsSnapshot() TradingFactsSnapshot {
	return TradingFactsSnapshot{
		SnapshotRef:           "data_snapshot:sha256:" + repeatHex(64, 'a'),
		AuthorizationRef:      "data_authorization:opaque",
		ManifestSchemaVersion: "1.0.0",
		PayloadSchemaVersion:  TradingFactsPayloadSchemaVersion,
		PayloadKind:           TradingFactsPayloadKind,
		CallerRef:             "principal:service:researchflow",
		PurposeCode:           TradingFactsPurposeCode,
		SystemAsOf:            "2026-11-25T15:00:00Z",
		Symbol:                "600519.SH",
		TradeDate:             "2026-11-25",
		QualityStatus:         "valid",
		Facts: []TradingFact{
			{Key: "risk_warning_status", Availability: tradingFactAvailable, Value: "normal"},
			{Key: "suspension_status", Availability: tradingFactAvailable, Value: "no_suspension_event"},
			{Key: "delisting_status", Availability: tradingFactAvailable, Value: "normal"},
			{Key: "listing_date", Availability: tradingFactAvailable, Value: "2001-08-27"},
			{Key: "is_trading_day", Availability: tradingFactAvailable, Value: "true"},
		},
	}
}

func repeatHex(length int, char byte) string {
	buffer := make([]byte, length)
	for index := range buffer {
		buffer[index] = char
	}
	return string(buffer)
}

func TestAssessTradabilityFactsKeepsOnlyPriceLimitMissing(t *testing.T) {
	assessment, err := AssessTradabilityFacts(validTradabilityEvidence())
	if err != nil {
		t.Fatalf("AssessTradabilityFacts() error = %v", err)
	}
	if assessment.Facts.Verified {
		t.Fatal("tradability must not be verified while Data has no price-limit fact")
	}
	want := []risk.TradabilityFact{risk.TradabilityFactPriceLimit}
	if len(assessment.Facts.MissingFacts) != len(want) {
		t.Fatalf("MissingFacts = %v, want %v", assessment.Facts.MissingFacts, want)
	}
	for index := range want {
		if assessment.Facts.MissingFacts[index] != want[index] {
			t.Fatalf("MissingFacts[%d] = %q, want %q", index, assessment.Facts.MissingFacts[index], want[index])
		}
	}
	if len(assessment.BlockingFacts) != 0 {
		t.Fatalf("BlockingFacts = %v, want none", assessment.BlockingFacts)
	}
}

func TestAssessTradabilityFactsRecordsKnownBlockingFacts(t *testing.T) {
	for name, mutate := range map[string]struct {
		mutate func(*TradingFact)
		key    string
	}{
		"risk warning":       {mutate: func(fact *TradingFact) { fact.Value = "risk_warning" }, key: "risk_warning_status"},
		"suspended":          {mutate: func(fact *TradingFact) { fact.Value = "suspended" }, key: "suspension_status"},
		"intraday suspended": {mutate: func(fact *TradingFact) { fact.Value = "intraday_suspension" }, key: "suspension_status"},
		"delisting risk":     {mutate: func(fact *TradingFact) { fact.Value = "delisting_risk" }, key: "delisting_status"},
		"not a trading day":  {mutate: func(fact *TradingFact) { fact.Value = "false" }, key: "is_trading_day"},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := validTradingFactsSnapshot()
			target := -1
			switch name {
			case "risk warning":
				target = 0
			case "suspended", "intraday suspended":
				target = 1
			case "delisting risk":
				target = 2
			case "not a trading day":
				target = 4
			}
			mutate.mutate(&snapshot.Facts[target])
			assessment, err := AssessTradabilityFacts(TradabilityEvidence{
				TradingFacts: snapshot, OrderBookFacts: validOrderBookFactsSnapshot(),
			})
			if err != nil {
				t.Fatalf("AssessTradabilityFacts() error = %v", err)
			}
			if assessment.Facts.Verified {
				t.Fatal("a known non-tradable fact must never be reported as verified")
			}
			if len(assessment.BlockingFacts) != 1 || assessment.BlockingFacts[0] != mutate.key {
				t.Fatalf("BlockingFacts = %v, want [%s]", assessment.BlockingFacts, mutate.key)
			}
		})
	}
}

func TestAssessTradabilityFactsReportsUnavailableGateFactsAsMissing(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	snapshot.Facts[1] = TradingFact{
		Key: "suspension_status", Availability: tradingFactUnavailable, UnavailableReason: "suspension_status_unknown",
	}
	snapshot.QualityStatus = "degraded"
	assessment, err := AssessTradabilityFacts(TradabilityEvidence{
		TradingFacts: snapshot, OrderBookFacts: validOrderBookFactsSnapshot(),
	})
	if err != nil {
		t.Fatalf("AssessTradabilityFacts() error = %v", err)
	}
	want := []risk.TradabilityFact{
		risk.TradabilityFactSuspension, risk.TradabilityFactPriceLimit,
	}
	if len(assessment.Facts.MissingFacts) != len(want) {
		t.Fatalf("MissingFacts = %v, want %v", assessment.Facts.MissingFacts, want)
	}
	for index := range want {
		if assessment.Facts.MissingFacts[index] != want[index] {
			t.Fatalf("MissingFacts[%d] = %q, want %q", index, assessment.Facts.MissingFacts[index], want[index])
		}
	}
}

func TestTradingFactsSnapshotValidateRejectsNonCanonicalInput(t *testing.T) {
	for name, mutate := range map[string]func(*TradingFactsSnapshot){
		"wrong order": func(snapshot *TradingFactsSnapshot) {
			snapshot.Facts[0], snapshot.Facts[1] = snapshot.Facts[1], snapshot.Facts[0]
		},
		"unknown availability": func(snapshot *TradingFactsSnapshot) {
			snapshot.Facts[0].Availability = "partial"
		},
		"available with reason": func(snapshot *TradingFactsSnapshot) {
			snapshot.Facts[0].UnavailableReason = "unexpected"
		},
		"unavailable with value": func(snapshot *TradingFactsSnapshot) {
			snapshot.Facts[0] = TradingFact{Key: "risk_warning_status", Availability: tradingFactUnavailable, Value: "normal", UnavailableReason: "x"}
		},
		"missing authorization ref": func(snapshot *TradingFactsSnapshot) { snapshot.AuthorizationRef = "" },
		"wrong payload kind":        func(snapshot *TradingFactsSnapshot) { snapshot.PayloadKind = "daily_bars_close_series" },
		"wrong quality":             func(snapshot *TradingFactsSnapshot) { snapshot.QualityStatus = "unknown" },
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := validTradingFactsSnapshot()
			mutate(&snapshot)
			if err := snapshot.Validate(); err == nil {
				t.Fatal("Validate() accepted a non-canonical facts snapshot")
			}
		})
	}
}

func TestEvaluateTradabilityDecisionAlwaysRejectsWithoutFullFacts(t *testing.T) {
	ctx := context.Background()
	decision, err := EvaluateTradabilityDecision(
		ctx, "risk-decision-1",
		time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC),
		validTradabilityEvidence(),
	)
	if err != nil {
		t.Fatalf("EvaluateTradabilityDecision() error = %v", err)
	}
	if decision.Status != risk.DecisionReject {
		t.Fatalf("decision status = %q, want reject", decision.Status)
	}
	if len(decision.Reasons) != 1 || decision.Reasons[0].Status != risk.ReasonTriggered ||
		decision.Reasons[0].Code != risk.TradabilityGateCode {
		t.Fatalf("decision reasons = %+v, want triggered tradability gate", decision.Reasons)
	}
	if len(decision.InputRefs) != 4 {
		t.Fatalf("InputRefs = %v, want both snapshot/authorization reference pairs", decision.InputRefs)
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("decision is not contract-valid: %v", err)
	}
}

func TestEvaluateTradabilityDecisionRejectsNonUTCAndInvertedTimes(t *testing.T) {
	evidence := validTradabilityEvidence()
	if _, err := EvaluateTradabilityDecision(context.Background(), "risk-decision-1",
		time.Date(2026, 11, 25, 16, 0, 0, 0, time.FixedZone("CST", 8*3600)), evidence); err == nil {
		t.Fatal("non-UTC evaluated_at was accepted")
	}
	// evaluated_at 早于 system_as_of 时决策不自洽，必须失败而不是产出一个"看起来合法"的拒绝。
	if _, err := EvaluateTradabilityDecision(context.Background(), "risk-decision-1",
		time.Date(2026, 11, 25, 14, 0, 0, 0, time.UTC), evidence); err == nil {
		t.Fatal("evaluated_at before system_as_of was accepted")
	}
}
