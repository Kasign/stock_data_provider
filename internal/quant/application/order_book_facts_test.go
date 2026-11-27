package application

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/risk"
)

func validOrderBookFactsSnapshot() OrderBookFactsSnapshot {
	return OrderBookFactsSnapshot{
		SnapshotRef:           "data_snapshot:sha256:" + repeatHex(64, 'b'),
		AuthorizationRef:      "data_authorization:order-book-opaque",
		ManifestSchemaVersion: "1.0.0",
		PayloadSchemaVersion:  OrderBookFactsPayloadSchemaVersion,
		PayloadKind:           OrderBookFactsPayloadKind,
		CallerRef:             "principal:service:researchflow",
		PurposeCode:           TradingFactsPurposeCode,
		Symbol:                "600519.SH",
		ObservedAt:            "2026-11-25T14:59:58Z",
		SystemAsOf:            "2026-11-25T15:00:00Z",
		TradableVolumeShares:  23000,
		BidLevels:             5,
		AskLevels:             5,
		Source:                "eastmoney",
		QualityStatus:         "valid",
	}
}

func validLimitPoolFactsSnapshot() LimitPoolFactsSnapshot {
	return LimitPoolFactsSnapshot{
		SnapshotRef:           "data_snapshot:sha256:" + repeatHex(64, 'c'),
		AuthorizationRef:      "data_authorization:limit-pool-opaque",
		ManifestSchemaVersion: "1.0.0",
		PayloadSchemaVersion:  LimitPoolFactsPayloadSchemaVersion,
		PayloadKind:           LimitPoolFactsPayloadKind,
		CallerRef:             "principal:service:researchflow",
		PurposeCode:           TradingFactsPurposeCode,
		Symbol:                "600519.SH",
		TradeDate:             "2026-11-25",
		ObservedAt:            "2026-11-25T07:01:00Z",
		SystemAsOf:            "2026-11-25T15:00:00Z",
		LimitStatus:           LimitPoolStatusUnknown,
		UpPoolTotal:           10,
		DownPoolTotal:         0,
		UpPoolComplete:        false,
		DownPoolComplete:      true,
		Source:                "eastmoney",
		QualityStatus:         "valid",
	}
}

func validTradabilityEvidence() TradabilityEvidence {
	return TradabilityEvidence{
		TradingFacts:   validTradingFactsSnapshot(),
		OrderBookFacts: validOrderBookFactsSnapshot(),
		LimitPoolFacts: validLimitPoolFactsSnapshot(),
	}
}

func TestAssessTradabilityFactsUsesAuthorizedOrderBookForTradableVolume(t *testing.T) {
	assessment, err := AssessTradabilityFacts(validTradabilityEvidence())
	if err != nil {
		t.Fatalf("AssessTradabilityFacts() error = %v", err)
	}
	if assessment.Facts.Verified {
		t.Fatal("price_limit is still unavailable, so tradability must remain unverified")
	}
	want := []risk.TradabilityFact{risk.TradabilityFactPriceLimit}
	if len(assessment.Facts.MissingFacts) != len(want) {
		t.Fatalf("MissingFacts = %v, want %v", assessment.Facts.MissingFacts, want)
	}
	if assessment.Facts.MissingFacts[0] != want[0] {
		t.Fatalf("MissingFacts[0] = %q, want %q", assessment.Facts.MissingFacts[0], want[0])
	}
	wantRefs := []string{
		validTradingFactsSnapshot().SnapshotRef,
		validTradingFactsSnapshot().AuthorizationRef,
		validOrderBookFactsSnapshot().SnapshotRef,
		validOrderBookFactsSnapshot().AuthorizationRef,
		validLimitPoolFactsSnapshot().SnapshotRef,
		validLimitPoolFactsSnapshot().AuthorizationRef,
	}
	if len(assessment.Facts.EvidenceRefs) != len(wantRefs) {
		t.Fatalf("EvidenceRefs = %v, want %v", assessment.Facts.EvidenceRefs, wantRefs)
	}
	for index := range wantRefs {
		if assessment.Facts.EvidenceRefs[index] != wantRefs[index] {
			t.Fatalf("EvidenceRefs[%d] = %q, want %q", index, assessment.Facts.EvidenceRefs[index], wantRefs[index])
		}
	}
	if len(assessment.BlockingFacts) != 0 {
		t.Fatalf("BlockingFacts = %v, want none", assessment.BlockingFacts)
	}
}

func TestAssessTradabilityFactsFailsClosedOnOrderBookEvidenceMismatch(t *testing.T) {
	for name, mutate := range map[string]func(*OrderBookFactsSnapshot){
		"wrong symbol": func(book *OrderBookFactsSnapshot) { book.Symbol = "000001.SZ" },
		"malformed system as of": func(book *OrderBookFactsSnapshot) {
			book.SystemAsOf = "not-a-time"
		},
		"future observation": func(book *OrderBookFactsSnapshot) {
			book.ObservedAt = "2026-11-25T15:00:01Z"
		},
		"wrong payload kind": func(book *OrderBookFactsSnapshot) {
			book.PayloadKind = "security_trading_status_facts"
		},
		"non-positive volume": func(book *OrderBookFactsSnapshot) { book.TradableVolumeShares = 0 },
		"incomplete book":     func(book *OrderBookFactsSnapshot) { book.BidLevels = 4 },
	} {
		t.Run(name, func(t *testing.T) {
			evidence := validTradabilityEvidence()
			mutate(&evidence.OrderBookFacts)
			if _, err := AssessTradabilityFacts(evidence); err == nil {
				t.Fatal("AssessTradabilityFacts() accepted a mismatched order-book evidence")
			}
		})
	}
}

func TestAssessTradabilityFactsProjectsDegradedOrderBookAsDegradedInput(t *testing.T) {
	evidence := validTradabilityEvidence()
	evidence.OrderBookFacts.QualityStatus = "degraded"
	assessment, err := AssessTradabilityFacts(evidence)
	if err != nil {
		t.Fatalf("AssessTradabilityFacts() error = %v", err)
	}
	if len(assessment.Facts.MissingFacts) != 1 ||
		assessment.Facts.MissingFacts[0] != risk.TradabilityFactPriceLimit {
		t.Fatalf("MissingFacts = %v, want only price_limit", assessment.Facts.MissingFacts)
	}
}

func TestEvaluateTradabilityDecisionCitesBothAuthorizedSnapshots(t *testing.T) {
	evidence := validTradabilityEvidence()
	systemAsOf := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	decision, err := EvaluateTradabilityDecision(
		context.Background(), "risk-decision-order-book", systemAsOf, systemAsOf, evidence,
	)
	if err != nil {
		t.Fatalf("EvaluateTradabilityDecision() error = %v", err)
	}
	want := []string{
		evidence.TradingFacts.SnapshotRef, evidence.TradingFacts.AuthorizationRef,
		evidence.OrderBookFacts.SnapshotRef, evidence.OrderBookFacts.AuthorizationRef,
		evidence.LimitPoolFacts.SnapshotRef, evidence.LimitPoolFacts.AuthorizationRef,
	}
	if len(decision.InputRefs) != len(want) {
		t.Fatalf("InputRefs = %v, want %v", decision.InputRefs, want)
	}
	for index := range want {
		if decision.InputRefs[index] != want[index] {
			t.Fatalf("InputRefs[%d] = %q, want %q", index, decision.InputRefs[index], want[index])
		}
	}
	if len(decision.Reasons) != 1 || decision.Reasons[0].Status != risk.ReasonTriggered {
		t.Fatalf("decision reasons = %+v, want a triggered tradability gate", decision.Reasons)
	}
}

func limitPoolWithStatus(snapshot LimitPoolFactsSnapshot, status string, complete bool) LimitPoolFactsSnapshot {
	snapshot.LimitStatus = status
	snapshot.UpPoolComplete = complete
	snapshot.DownPoolComplete = complete
	if complete {
		snapshot.UpPoolTotal = 0
		snapshot.DownPoolTotal = 0
	}
	return snapshot
}

func TestAssessTradabilityFactsBlocksOnLimitUpAndLimitDown(t *testing.T) {
	for _, status := range []string{LimitPoolStatusUp, LimitPoolStatusDown} {
		evidence := validTradabilityEvidence()
		evidence.LimitPoolFacts = limitPoolWithStatus(evidence.LimitPoolFacts, status, true)
		assessment, err := AssessTradabilityFacts(evidence)
		if err != nil {
			t.Fatalf("%s: AssessTradabilityFacts() error = %v", status, err)
		}
		if len(assessment.Facts.MissingFacts) != 0 {
			t.Fatalf("%s: missing facts = %v, want none", status, assessment.Facts.MissingFacts)
		}
		if len(assessment.BlockingFacts) != 1 || assessment.BlockingFacts[0] != "price_limit" {
			t.Fatalf("%s: blocking facts = %v, want [price_limit]", status, assessment.BlockingFacts)
		}
	}
}

func TestAssessTradabilityFactsVerifiesNoneOnlyWithCompletePools(t *testing.T) {
	evidence := validTradabilityEvidence()
	evidence.LimitPoolFacts = limitPoolWithStatus(evidence.LimitPoolFacts, LimitPoolStatusNone, true)
	assessment, err := AssessTradabilityFacts(evidence)
	if err != nil {
		t.Fatalf("AssessTradabilityFacts() error = %v", err)
	}
	if len(assessment.Facts.MissingFacts) != 0 || len(assessment.BlockingFacts) != 0 {
		t.Fatalf("missing=%v blocking=%v, want both empty", assessment.Facts.MissingFacts, assessment.BlockingFacts)
	}
	if !assessment.Facts.Verified {
		t.Fatal("all three facts verified but gate reported Verified=false")
	}

	// none 但池不完整：Payload 自洽性校验必须拒绝，不能被当成可交易。
	broken := validTradabilityEvidence()
	broken.LimitPoolFacts = limitPoolWithStatus(broken.LimitPoolFacts, LimitPoolStatusNone, false)
	if _, err := AssessTradabilityFacts(broken); err == nil {
		t.Fatal("none with an incomplete pool must be rejected")
	}
}

func TestAssessTradabilityFactsTreatsUnknownLimitPoolAsMissing(t *testing.T) {
	assessment, err := AssessTradabilityFacts(validTradabilityEvidence())
	if err != nil {
		t.Fatalf("AssessTradabilityFacts() error = %v", err)
	}
	if len(assessment.Facts.MissingFacts) != 1 || assessment.Facts.MissingFacts[0] != risk.TradabilityFactPriceLimit {
		t.Fatalf("missing facts = %v, want [price_limit]", assessment.Facts.MissingFacts)
	}
	if len(assessment.BlockingFacts) != 0 {
		t.Fatalf("blocking facts = %v, want none", assessment.BlockingFacts)
	}
	if assessment.Facts.Verified {
		t.Fatal("unknown limit pool must never produce Verified=true")
	}
}
