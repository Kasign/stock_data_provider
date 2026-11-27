package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func validOrderBookForSnapshot(t *testing.T) model.OrderBook {
	t.Helper()
	observedAt := time.Date(2026, 11, 27, 10, 15, 0, 0, time.UTC)
	return model.OrderBook{
		Symbol: "600519.SH",
		Bids: []model.OrderBookLevel{
			{Price: 1313.10, Volume: 1200}, {Price: 1313.00, Volume: 3500},
			{Price: 1312.90, Volume: 4800}, {Price: 1312.80, Volume: 6100},
			{Price: 1312.70, Volume: 7400},
		},
		Asks: []model.OrderBookLevel{
			{Price: 1313.20, Volume: 900}, {Price: 1313.30, Volume: 1700},
			{Price: 1313.40, Volume: 2500}, {Price: 1313.50, Volume: 3300},
			{Price: 1313.60, Volume: 4100},
		},
		ObservedAt: observedAt, SystemObservedAt: observedAt.Add(2 * time.Second),
		Source: "eastmoney", QualityStatus: model.QualityValid,
	}
}

func orderBookSnapshotTestInput(t *testing.T) (OrderBookFactsPayload, []DailyBarsCloseSeriesSource, time.Time) {
	t.Helper()
	payload, err := BuildOrderBookFactsPayload(validOrderBookForSnapshot(t))
	if err != nil {
		t.Fatalf("BuildOrderBookFactsPayload() error = %v", err)
	}
	sources := []DailyBarsCloseSeriesSource{{
		SourceRef: "eastmoney", SourceVersion: stringPointer("eastmoney-push2-order-book-v1"),
	}}
	return payload, sources, payload.SystemObservedAt.Add(time.Second)
}

func TestBuildOrderBookFactsSnapshotBindsManifestPayloadAndFingerprints(t *testing.T) {
	payload, sources, systemAsOf := orderBookSnapshotTestInput(t)
	snapshot, err := BuildOrderBookFactsSnapshot(payload, sources, "principal:service:researchflow", "risk_review", systemAsOf)
	if err != nil {
		t.Fatalf("BuildOrderBookFactsSnapshot() error = %v", err)
	}
	if err := snapshot.Verify(); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if snapshot.Manifest.Symbol != payload.Symbol ||
		snapshot.Manifest.ObservedAt != payload.ObservedAt.Format(time.RFC3339Nano) ||
		snapshot.Manifest.SystemAsOf != systemAsOf.Format(time.RFC3339Nano) {
		t.Fatalf("manifest does not bind payload identity/time: %+v", snapshot.Manifest)
	}
	if snapshot.Manifest.PurposeCode != "risk_review" || snapshot.Manifest.CallerRef != "principal:service:researchflow" {
		t.Fatalf("manifest lost trusted caller/purpose: %+v", snapshot.Manifest)
	}
	mutated := snapshot
	mutated.Payload.TradableVolumeShares++
	if err := mutated.Verify(); err == nil {
		t.Fatal("payload mutation must invalidate the snapshot fingerprint")
	}
}

func TestBuildOrderBookFactsSnapshotRejectsUntrustedInputs(t *testing.T) {
	payload, sources, systemAsOf := orderBookSnapshotTestInput(t)
	if _, err := BuildOrderBookFactsSnapshot(payload, sources, "", "risk_review", systemAsOf); err == nil {
		t.Fatal("blank caller must be rejected")
	}
	if _, err := BuildOrderBookFactsSnapshot(payload, sources, "principal:service:researchflow", "unknown_purpose", systemAsOf); err == nil {
		t.Fatal("unknown purpose must be rejected")
	}
	if _, err := BuildOrderBookFactsSnapshot(payload, sources, "principal:service:researchflow", "risk_review", systemAsOf.Local()); err == nil {
		t.Fatal("non-UTC system_as_of must be rejected")
	}
	if _, err := BuildOrderBookFactsSnapshot(payload, nil, "principal:service:researchflow", "risk_review", systemAsOf); err == nil {
		t.Fatal("missing source must be rejected")
	}
	if _, err := BuildOrderBookFactsSnapshot(payload, append([]DailyBarsCloseSeriesSource(nil), sources[0], sources[0]), "principal:service:researchflow", "risk_review", systemAsOf); err == nil {
		t.Fatal("duplicate source must be rejected")
	}
	if _, err := BuildOrderBookFactsSnapshot(payload, sources, "principal:service:researchflow", "risk_review", payload.SystemObservedAt.Add(-time.Second)); err == nil {
		t.Fatal("system_as_of earlier than source observation must be rejected")
	}
}

func TestOrderBookFactsSnapshotVerifyRejectsIdentityAndTimeDrift(t *testing.T) {
	payload, sources, systemAsOf := orderBookSnapshotTestInput(t)
	snapshot, err := BuildOrderBookFactsSnapshot(payload, sources, "principal:service:researchflow", "risk_review", systemAsOf)
	if err != nil {
		t.Fatal(err)
	}
	wrongSymbol := snapshot
	wrongSymbol.Manifest.Symbol = "000001.SZ"
	if err := wrongSymbol.Verify(); err == nil {
		t.Fatal("manifest symbol drift must be rejected")
	}
	wrongTime := snapshot
	wrongTime.Manifest.SystemAsOf = payload.SystemObservedAt.Add(-time.Second).Format(time.RFC3339Nano)
	if err := wrongTime.Verify(); err == nil {
		t.Fatal("system_as_of before payload observation must be rejected")
	}
}

func TestOrderBookPayloadSchemaMatchesBuilderAndGolden(t *testing.T) {
	payload, sources, _ := orderBookSnapshotTestInput(t)
	snapshot, err := BuildOrderBookFactsSnapshot(payload, sources, "principal:service:researchflow", "risk_review", payload.SystemObservedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	schema := compileOrderBookPayloadSchema(t)
	encoded, err := json.Marshal(snapshot.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOrderBookPayloadSchema(schema, encoded); err != nil {
		t.Fatalf("builder output does not satisfy payload schema: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("..", "..", "..", "doc", "整体任务规划", "golden", "DATA-CTR-007-order-book-payload.golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if err := validateOrderBookPayloadSchema(schema, golden); err != nil {
		t.Fatalf("golden does not satisfy payload schema: %v", err)
	}
	var goldenPayload OrderBookFactsPayload
	if err := json.Unmarshal(golden, &goldenPayload); err != nil {
		t.Fatal(err)
	}
	if err := goldenPayload.Validate(); err != nil {
		t.Fatalf("golden is not a valid DTO: %v", err)
	}
}

func TestOrderBookPayloadSchemaRejectsUnknownAndInvalidFields(t *testing.T) {
	schema := compileOrderBookPayloadSchema(t)
	payload, _, _ := orderBookSnapshotTestInput(t)
	base, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(map[string]any){
		"unknown field":   func(object map[string]any) { object["participation_rate"] = 0.1 },
		"zero volume":     func(object map[string]any) { object["tradable_volume_shares"] = float64(0) },
		"four bid levels": func(object map[string]any) { object["bid_levels"] = float64(4) },
		"wrong kind":      func(object map[string]any) { object["payload_kind"] = "security_trading_status_facts" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(base, &object); err != nil {
				t.Fatal(err)
			}
			mutate(object)
			encoded, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateOrderBookPayloadSchema(schema, encoded); err == nil {
				t.Fatal("schema accepted an invalid order-book payload")
			}
		})
	}
}

func compileOrderBookPayloadSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "整体任务规划", "schema", "DATA-CTR-007-order-book-payload-1.0.0.schema.json")
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	compiler.LoadURL = func(uri string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("external schema reference is disabled: %s", uri)
	}
	const schemaURL = "https://market-data.invalid/order-book-schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return schema
}

func validateOrderBookPayloadSchema(schema *jsonschema.Schema, encoded []byte) error {
	var instance any
	if err := json.Unmarshal(encoded, &instance); err != nil {
		return fmt.Errorf("decode schema instance: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}
	return nil
}

func stringPointer(value string) *string { return &value }
