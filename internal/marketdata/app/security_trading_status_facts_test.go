package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const tradingFactsSchemaFile = "DATA-CTR-007-security-trading-status-facts-payload-1.0.0.schema.json"

func tradingFactsSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileDailyBarsCloseSeriesSchema(t, tradingFactsSchemaFile)
}

func readTradingFactsGolden(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "整体任务规划", "golden",
		"DATA-CTR-007-security-trading-status-facts-payload.golden.json")
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return document
}

func allAvailableFacts() []SecurityTradingStatusFact {
	return []SecurityTradingStatusFact{
		{Key: "risk_warning_status", Availability: TradingFactAvailable, Value: "normal"},
		{Key: "suspension_status", Availability: TradingFactAvailable, Value: "trading"},
		{Key: "delisting_status", Availability: TradingFactAvailable, Value: "listed"},
		{Key: "listing_date", Availability: TradingFactAvailable, Value: "1999-11-10"},
		{Key: "is_trading_day", Availability: TradingFactAvailable, Value: "true"},
	}
}

func validTradingFactsPayload() SecurityTradingStatusFactsPayload {
	return SecurityTradingStatusFactsPayload{
		SchemaVersion: SecurityTradingStatusFactsPayloadSchemaVersion,
		PayloadKind:   SecurityTradingStatusFactsPayloadKind,
		Symbol:        "600000.SH",
		TradeDate:     "2026-11-26",
		QualityStatus: QualityValid,
		Facts:         allAvailableFacts(),
	}
}

func tradingFactsSources() []DailyBarsCloseSeriesSource {
	return []DailyBarsCloseSeriesSource{{SourceRef: "tushare:security_trading_status:v2"}}
}

func TestSecurityTradingStatusFactsGoldenMatchesSchemaAndBuilder(t *testing.T) {
	document := readTradingFactsGolden(t)
	schema := tradingFactsSchema(t)
	if err := validateDailyBarsCloseSeriesSchemaInstance(schema, document); err != nil {
		t.Fatalf("golden violates %s: %v", tradingFactsSchemaFile, err)
	}
	var payload SecurityTradingStatusFactsPayload
	if err := json.Unmarshal(document, &payload); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("golden fails payload validation: %v", err)
	}
	if payload.QualityStatus != QualityValid {
		t.Fatalf("golden quality = %q, want valid", payload.QualityStatus)
	}
}

func TestBuildSecurityTradingStatusFactsSnapshotAllAvailable(t *testing.T) {
	snapshot, err := BuildSecurityTradingStatusFactsSnapshot(
		validTradingFactsPayload(), tradingFactsSources(), "principal:quant", "quant_analysis",
		time.Date(2026, 11, 26, 7, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.HasPrefix(snapshot.Manifest.SnapshotRef, "data_snapshot:sha256:") {
		t.Fatalf("snapshot_ref = %q, want data_snapshot prefix", snapshot.Manifest.SnapshotRef)
	}
	if err := snapshot.Verify(); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if snapshot.Manifest.SystemAsOf != "2026-11-26T07:05:00Z" {
		t.Fatalf("system_as_of = %q", snapshot.Manifest.SystemAsOf)
	}
}

func TestSecurityTradingStatusFactsMarksUnavailableAsDegraded(t *testing.T) {
	payload := validTradingFactsPayload()
	payload.Facts[2] = SecurityTradingStatusFact{
		Key: "delisting_status", Availability: TradingFactUnavailable,
		UnavailableReason: "provider_did_not_return_delisting_status",
	}
	payload.QualityStatus = QualityDegraded
	if err := payload.Validate(); err != nil {
		t.Fatalf("partial-unavailable payload rejected: %v", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDailyBarsCloseSeriesSchemaInstance(tradingFactsSchema(t), encoded); err != nil {
		t.Fatalf("partial-unavailable payload violates schema: %v", err)
	}

	// 同一事实标为 valid 必须被两侧同时拒绝。
	payload.QualityStatus = QualityValid
	if err := payload.Validate(); err == nil {
		t.Fatal("DTO accepted degraded facts marked valid")
	}
	encoded, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDailyBarsCloseSeriesSchemaInstance(tradingFactsSchema(t), encoded); err == nil {
		t.Fatal("Schema accepted degraded facts marked valid")
	}
}

func TestSecurityTradingStatusFactsAllUnavailableIsLegalButDegraded(t *testing.T) {
	payload := validTradingFactsPayload()
	for index := range payload.Facts {
		payload.Facts[index] = SecurityTradingStatusFact{
			Key: payload.Facts[index].Key, Availability: TradingFactUnavailable,
			UnavailableReason: "provider_fact_missing",
		}
	}
	payload.QualityStatus = QualityDegraded
	if err := payload.Validate(); err != nil {
		t.Fatalf("all-unavailable payload rejected: %v", err)
	}
}

// 涨跌停与可成交量不在封闭枚举内：它们从未被 Provider 提供，
// 写入等于把推导当事实。Schema 与 DTO 必须同时拒绝。
func TestSecurityTradingStatusFactsRejectsDerivedFactKeys(t *testing.T) {
	schema := tradingFactsSchema(t)
	for _, key := range []string{"price_limit", "tradable_volume"} {
		t.Run(key, func(t *testing.T) {
			payload := validTradingFactsPayload()
			payload.Facts[0].Key = key
			if err := payload.Validate(); err == nil {
				t.Fatalf("DTO accepted derived fact key %q", key)
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDailyBarsCloseSeriesSchemaInstance(schema, encoded); err == nil {
				t.Fatalf("Schema accepted derived fact key %q", key)
			}
		})
	}
}

func TestSecurityTradingStatusFactsValidateMatchesSchema(t *testing.T) {
	schema := tradingFactsSchema(t)
	mutations := map[string]func(*SecurityTradingStatusFactsPayload){
		"swapped order": func(p *SecurityTradingStatusFactsPayload) {
			p.Facts[0], p.Facts[1] = p.Facts[1], p.Facts[0]
		},
		"missing fact": func(p *SecurityTradingStatusFactsPayload) { p.Facts = p.Facts[:4] },
		"available with reason": func(p *SecurityTradingStatusFactsPayload) {
			p.Facts[0].UnavailableReason = "should not be here"
		},
		"unavailable with value": func(p *SecurityTradingStatusFactsPayload) {
			p.Facts[1] = SecurityTradingStatusFact{
				Key: "suspension_status", Availability: TradingFactUnavailable,
				Value: "trading", UnavailableReason: "conflict",
			}
			p.QualityStatus = QualityDegraded
		},
		"unavailable without reason": func(p *SecurityTradingStatusFactsPayload) {
			p.Facts[1] = SecurityTradingStatusFact{Key: "suspension_status", Availability: TradingFactUnavailable}
			p.QualityStatus = QualityDegraded
		},
		"unknown availability": func(p *SecurityTradingStatusFactsPayload) {
			p.Facts[0].Availability = "partial"
		},
		"blank symbol":   func(p *SecurityTradingStatusFactsPayload) { p.Symbol = " " },
		"bad trade date": func(p *SecurityTradingStatusFactsPayload) { p.TradeDate = "2026/11/26" },
		"wrong payload kind": func(p *SecurityTradingStatusFactsPayload) {
			p.PayloadKind = "daily_bars_close_series"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			payload := validTradingFactsPayload()
			mutate(&payload)
			if err := payload.Validate(); err == nil {
				t.Fatal("DTO accepted invalid facts payload")
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDailyBarsCloseSeriesSchemaInstance(schema, encoded); err == nil {
				t.Fatal("Schema accepted invalid facts payload")
			}
		})
	}
}

func TestSecurityTradingStatusFactsSchemaRejectsUnknownField(t *testing.T) {
	payload := validTradingFactsPayload()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var instance map[string]any
	if err := json.Unmarshal(encoded, &instance); err != nil {
		t.Fatal(err)
	}
	instance["price_limit_state"] = "limit_up"
	if err := validateDailyBarsCloseSeriesSchemaInstance(tradingFactsSchema(t), mustJSON(t, instance)); err == nil {
		t.Fatal("Schema accepted undeclared field")
	}
}

func TestBuildSecurityTradingStatusFactsSnapshotRejectsUntrustedInput(t *testing.T) {
	asOf := time.Date(2026, 11, 26, 7, 5, 0, 0, time.UTC)
	cases := map[string]func() (SecurityTradingStatusFactsSnapshot, error){
		"blank caller": func() (SecurityTradingStatusFactsSnapshot, error) {
			return BuildSecurityTradingStatusFactsSnapshot(validTradingFactsPayload(), tradingFactsSources(), " ", "quant_analysis", asOf)
		},
		"unknown purpose": func() (SecurityTradingStatusFactsSnapshot, error) {
			return BuildSecurityTradingStatusFactsSnapshot(validTradingFactsPayload(), tradingFactsSources(), "principal:quant", "unknown", asOf)
		},
		"non-utc as-of": func() (SecurityTradingStatusFactsSnapshot, error) {
			location := time.FixedZone("CST", 8*3600)
			return BuildSecurityTradingStatusFactsSnapshot(validTradingFactsPayload(), tradingFactsSources(), "principal:quant", "quant_analysis", asOf.In(location))
		},
		"no sources": func() (SecurityTradingStatusFactsSnapshot, error) {
			return BuildSecurityTradingStatusFactsSnapshot(validTradingFactsPayload(), nil, "principal:quant", "quant_analysis", asOf)
		},
		"duplicate source": func() (SecurityTradingStatusFactsSnapshot, error) {
			sources := append(tradingFactsSources(), tradingFactsSources()...)
			return BuildSecurityTradingStatusFactsSnapshot(validTradingFactsPayload(), sources, "principal:quant", "quant_analysis", asOf)
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := build(); err == nil {
				t.Fatal("Build() error = nil, want rejection")
			}
		})
	}
}

func TestSecurityTradingStatusFactsSnapshotRejectsPayloadDrift(t *testing.T) {
	snapshot, err := BuildSecurityTradingStatusFactsSnapshot(
		validTradingFactsPayload(), tradingFactsSources(), "principal:quant", "quant_analysis",
		time.Date(2026, 11, 26, 7, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	drifted := snapshot
	drifted.Payload = cloneTradingStatusFactsPayload(snapshot.Payload)
	drifted.Payload.Facts[0].Value = "st"
	if err := drifted.Verify(); err == nil {
		t.Fatal("Verify() accepted drifted payload")
	}

	identityDrift := snapshot
	identityDrift.Manifest = snapshot.Manifest
	identityDrift.Manifest.Symbol = "000001.SZ"
	if err := identityDrift.Verify(); err == nil {
		t.Fatal("Verify() accepted manifest identity drift")
	}
}

func TestBuildSecurityTradingStatusFactsSnapshotCopiesFacts(t *testing.T) {
	payload := validTradingFactsPayload()
	snapshot, err := BuildSecurityTradingStatusFactsSnapshot(
		payload, tradingFactsSources(), "principal:quant", "quant_analysis",
		time.Date(2026, 11, 26, 7, 5, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	payload.Facts[0].Value = "mutated"
	if snapshot.Payload.Facts[0].Value != "normal" {
		t.Fatalf("snapshot facts aliased caller slice: %v", snapshot.Payload.Facts[0])
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return encoded
}
