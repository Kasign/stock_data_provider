package httpapi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIDataRoutesDeclareValidAPIKeySecurity(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	text := string(spec)
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	if !strings.Contains(text, "    ApiKey:\n      type: apiKey\n      in: header\n      name: X-API-Key") {
		t.Fatal("OpenAPI must define the ApiKey scheme used by the HTTP server")
	}
	if strings.Contains(text, "ApiKeyAuth: []") {
		t.Fatal("OpenAPI references undefined ApiKeyAuth; use the declared ApiKey scheme")
	}

	paths := []string{
		"/api/v1/data-sources/probe:",
		"/api/v1/market/index-catalog:",
		"/api/v1/market/indices/{symbol}/constituents:",
		"/api/v1/market/indices/{symbol}/daily:",
		"/api/v1/market/session:",
		"/api/v1/stocks/{symbol}/price-limit-rule:",
		"/api/v1/stocks/{symbol}/trading-status:",
	}
	for _, path := range paths {
		start := strings.Index(text, "  "+path)
		if start < 0 {
			t.Errorf("OpenAPI path missing: %s", path)
			continue
		}
		next := strings.Index(text[start+len(path):], "\n  /api/")
		end := len(text)
		if next >= 0 {
			end = start + len(path) + next
		}
		section := text[start:end]
		securityCount := strings.Count(section, "      security:")
		if securityCount != 1 {
			t.Errorf("OpenAPI path %s must declare security exactly once; found %d declarations", path, securityCount)
		}
		if !strings.Contains(section, "security:\n        - ApiKey: []") {
			t.Errorf("OpenAPI path %s must declare the X-API-Key security used by the server", path)
		}
	}

	sessionStart := strings.Index(text, "    MarketSessionSnapshot:")
	if sessionStart < 0 {
		t.Fatal("MarketSessionSnapshot schema missing")
	}
	sessionEnd := strings.Index(text[sessionStart+len("    MarketSessionSnapshot:"):], "\n    StockAnnouncement:")
	if sessionEnd < 0 {
		sessionEnd = len(text) - sessionStart
	}
	sessionSchema := text[sessionStart : sessionStart+len("    MarketSessionSnapshot:")+sessionEnd]
	for _, field := range []string{"server_time:", "time_zone:", "Asia/Shanghai"} {
		if !strings.Contains(sessionSchema, field) {
			t.Errorf("MarketSessionSnapshot schema missing %q", field)
		}
	}

	constituentsStart := strings.Index(text, "    IndexConstituentSnapshot:")
	if constituentsStart < 0 {
		t.Fatal("IndexConstituentSnapshot schema missing")
	}
	constituentsEnd := strings.Index(text[constituentsStart+len("    IndexConstituentSnapshot:"):], "\n    IndexDailyBar:")
	if constituentsEnd < 0 {
		t.Fatal("IndexConstituentSnapshot schema boundary missing")
	}
	constituentsSchema := text[constituentsStart : constituentsStart+len("    IndexConstituentSnapshot:")+constituentsEnd]
	if !strings.Contains(constituentsSchema, "required: [index_symbol, items, total, provider_returned, source, fetched_at, quality_status]") {
		t.Error("IndexConstituentSnapshot must require provider_returned because the Go model always serializes it")
	}

	if !strings.Contains(text, "special_exception_evidence: {$ref: '#/components/schemas/PriceLimitEventEvidence', nullable: true}") {
		t.Error("PriceLimitRuleSnapshot must reference the structured special-event evidence schema")
	}
	evidenceStart := strings.Index(text, "    PriceLimitEventEvidence:")
	if evidenceStart < 0 {
		t.Fatal("PriceLimitEventEvidence schema missing")
	}
	evidenceEnd := strings.Index(text[evidenceStart+len("    PriceLimitEventEvidence:"):], "\n    SecurityTradingStatusSnapshot:")
	if evidenceEnd < 0 {
		t.Fatal("PriceLimitEventEvidence schema boundary missing")
	}
	evidenceSchema := text[evidenceStart : evidenceStart+len("    PriceLimitEventEvidence:")+evidenceEnd]
	for _, field := range []string{"effective_from:", "source_version:", "quality_status:", "source_url:", "observed_at:", "coverage_confirmed:"} {
		if !strings.Contains(evidenceSchema, field) {
			t.Errorf("PriceLimitEventEvidence schema missing %q", field)
		}
	}
	probeRequestStart := strings.Index(text, "    ProviderProbeRequest:")
	if probeRequestStart < 0 {
		t.Fatal("ProviderProbeRequest schema missing")
	}
	probeRequestEnd := strings.Index(text[probeRequestStart+len("    ProviderProbeRequest:"):], "\n    ProviderProbeRun:")
	if probeRequestEnd < 0 {
		t.Fatal("ProviderProbeRequest schema boundary missing")
	}
	probeRequestSchema := text[probeRequestStart : probeRequestStart+len("    ProviderProbeRequest:")+probeRequestEnd]
	for _, field := range []string{"scope:", "capabilities:", "timeout_seconds:", "uniqueItems: true", "maximum: 30"} {
		if !strings.Contains(probeRequestSchema, field) {
			t.Errorf("ProviderProbeRequest schema missing %q", field)
		}
	}

}

func TestProviderProbeValidationSchemaIsDocumented(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	text := string(spec)
	itemStart := strings.Index(text, "    ProviderProbeItem:")
	if itemStart < 0 {
		t.Fatal("ProviderProbeItem schema missing")
	}
	itemEnd := strings.Index(text[itemStart+len("    ProviderProbeItem:"):], "\n    ProviderProbeRequest:")
	if itemEnd < 0 {
		t.Fatal("ProviderProbeItem schema boundary missing")
	}
	itemSchema := text[itemStart : itemStart+len("    ProviderProbeItem:")+itemEnd]
	for _, field := range []string{
		"validation_total_count:",
		"validation_error_count:",
		"validation_warning_count:",
		"validation_info_count:",
		"validation_truncated:",
		"validation_issues:",
		"coverage_dimensions:",
		"maxItems: 100",
	} {
		if !strings.Contains(itemSchema, field) {
			t.Errorf("ProviderProbeItem schema is missing %q", field)
		}
	}
	if !strings.Contains(text, "    ProviderProbeFieldIssue:") {
		t.Fatal("ProviderProbeFieldIssue schema missing")
	}
}

func TestSectorRankingFiveDayChangeSchemaIsDocumented(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	text := string(spec)
	start := strings.Index(text, "    SectorRanking:")
	if start < 0 {
		t.Fatal("SectorRanking schema missing")
	}
	end := strings.Index(text[start+len("    SectorRanking:"):], "\n    SectorRankingSnapshot:")
	if end < 0 {
		t.Fatal("SectorRanking schema boundary missing")
	}
	schema := text[start : start+len("    SectorRanking:")+end]
	if !strings.Contains(schema, "change_percent_5d:") {
		t.Error("SectorRanking schema must document change_percent_5d")
	}
}

func TestOpenAPIDataModelFieldsMatchKnownJSONContract(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	props := func(name string) map[string]any {
		t.Helper()
		v, ok := schemas[name].(map[string]any)
		if !ok {
			t.Fatalf("schema %s missing", name)
		}
		p, ok := v["properties"].(map[string]any)
		if !ok {
			t.Fatalf("schema %s properties missing", name)
		}
		return p
	}
	required := func(name string) map[string]bool {
		t.Helper()
		v := schemas[name].(map[string]any)
		items, _ := v["required"].([]any)
		out := map[string]bool{}
		for _, item := range items {
			if field, ok := item.(string); ok {
				out[field] = true
			}
		}
		return out
	}
	for _, item := range []struct{ schema, field string }{
		{"ProviderProbeCheckResult", "left_adjustment_method"},
		{"ProviderProbeCheckResult", "right_adjustment_method"},
		{"ProviderProbeCheckResult", "left_adjustment_reference_date"},
		{"ProviderProbeCheckResult", "right_adjustment_reference_date"},
		{"SectorRanking", "provider_report_date"},
		{"SectorRankingSnapshot", "excluded_non_a_share_count"},
		{"MarketBreadthSnapshot", "excluded_non_a_share_count"},
		{"LimitPoolItem", "seal_money"},
		{"LimitPoolItem", "max_seal_money"},
		{"FundamentalSection", "available_at"},
		{"IndexConstituent", "effective_from"},
		{"IndexConstituent", "effective_to"},
		{"IndexConstituent", "available_at"},
		{"IndexConstituent", "system_observed_at"},
		{"SourceStatus", "market_capital_flow_capable"},
	} {
		if _, ok := props(item.schema)[item.field]; !ok {
			t.Errorf("%s.%s missing", item.schema, item.field)
		}
	}
	for _, item := range []struct{ schema, field string }{
		{"StockAnnouncementSnapshot", "source_version"},
		{"MarketSentimentSnapshot", "limit_up_count"},
		{"MarketSentimentSnapshot", "limit_down_count"},
		{"MarketSentimentSnapshot", "broken_board_count"},
		{"Quote", "change"},
		{"Quote", "change_percent"},
		{"Quote", "amount"},
		{"DailyBar", "amount"},
		{"DailyBar", "change_percent"},
		{"MarketBreadthSnapshot", "total_amount_cny"},
		{"SourceStatus", "historical_index_constituents_capable"},
	} {
		if !required(item.schema)[item.field] {
			t.Errorf("%s.%s must be required", item.schema, item.field)
		}
	}
	for _, item := range []struct{ schema, field string }{
		{"Bundle", "warnings"},
		{"HotRankingItem", "source_version"},
		{"HotRankingSnapshot", "source_version"},
		{"MarketSentimentSnapshot", "source_version"},
	} {
		if required(item.schema)[item.field] {
			t.Errorf("%s.%s must be optional", item.schema, item.field)
		}
	}
	for _, item := range []struct{ schema, field string }{
		{"Quote", "validation_total_count"},
		{"Quote", "validation_error_count"},
		{"Quote", "validation_warning_count"},
		{"Quote", "validation_info_count"},
		{"Quote", "validation_truncated"},
		{"Quote", "validation_issues"},
		{"StockBoard", "change_percent_5d"},
		{"StockBoard", "provider_report_date"},
		{"PriceLimitEventEvidence", "adjustment_basis"},
	} {
		if _, ok := props(item.schema)[item.field]; ok {
			t.Errorf("%s.%s has no current Go response field", item.schema, item.field)
		}
	}
	if p := props("LimitPoolItem")["change_percent"].(map[string]any); p["nullable"] != true {
		t.Error("LimitPoolItem.change_percent must be nullable")
	}
	for _, field := range []string{"name", "rank_trend"} {
		if props("HotRankingItem")[field].(map[string]any)["nullable"] == true {
			t.Errorf("HotRankingItem.%s must not be nullable", field)
		}
	}
}

func TestDataModelJSONSerializationMatchesOpenAPIRequiredAndNullableSemantics(t *testing.T) {
	assertFields := func(name string, value any, present, omitted, nullFields []string) {
		t.Helper()
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s JSON marshal: %v", name, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatalf("%s JSON decode: %v", name, err)
		}
		for _, field := range present {
			if _, ok := fields[field]; !ok {
				t.Errorf("%s.%s must be emitted; JSON=%s", name, field, encoded)
			}
		}
		for _, field := range omitted {
			if _, ok := fields[field]; ok {
				t.Errorf("%s.%s must be omitted; JSON=%s", name, field, encoded)
			}
		}
		for _, field := range nullFields {
			if raw, ok := fields[field]; !ok || string(raw) != "null" {
				t.Errorf("%s.%s must serialize as null; got %s", name, field, raw)
			}
		}
	}

	assertFields("Quote", model.Quote{},
		[]string{"symbol", "change", "change_percent", "amount", "current_price", "quote_time"},
		[]string{"name", "validation_total_count", "validation_issues"},
		[]string{"change", "change_percent", "amount", "current_price", "quote_time"})
	assertFields("DailyBar", model.DailyBar{},
		[]string{"symbol", "trade_date", "amount", "change_percent", "ma5"},
		[]string{"is_partial", "adjustment_method"},
		[]string{"amount", "change_percent", "ma5"})
	assertFields("Bundle", model.Bundle{},
		[]string{"symbol", "daily_bars", "sources", "snapshot_id"},
		[]string{"warnings", "name", "quote"},
		nil)
	assertFields("HotRankingItem", model.HotRankingItem{},
		[]string{"rank", "symbol", "code"},
		[]string{"source_version", "name", "rank_trend"},
		nil)
	assertFields("MarketSentimentSnapshot", model.MarketSentimentSnapshot{},
		[]string{"market", "trade_date", "limit_up_count", "limit_down_count", "broken_board_count"},
		[]string{"source_version", "provider_report_date"},
		[]string{"limit_up_count", "limit_down_count", "broken_board_count"})
	assertFields("LimitPoolItem", model.LimitPoolItem{},
		[]string{"symbol", "change_percent"},
		[]string{"seal_money", "max_seal_money"},
		[]string{"change_percent"})
	assertFields("SourceStatus", model.SourceStatus{},
		[]string{"name", "enabled", "quote_capable", "daily_bar_capable", "historical_index_constituents_capable"},
		[]string{"market_capital_flow_capable"},
		nil)
	assertFields("StockAnnouncementSnapshot", model.StockAnnouncementSnapshot{},
		[]string{"symbol", "source_version", "items"},
		nil,
		nil)
}
