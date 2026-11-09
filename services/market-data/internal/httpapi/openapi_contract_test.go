package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"gopkg.in/yaml.v3"
)

func TestOpenAPISecurityMatchesDataHTTPAdapterRoutes(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}

	components := document["components"].(map[string]any)
	schemes := components["securitySchemes"].(map[string]any)
	if _, ok := schemes["ApiKey"]; !ok {
		t.Fatal("OpenAPI must define ApiKey for X-API-Key")
	}
	if _, ok := schemes["BearerAuth"]; !ok {
		t.Fatal("OpenAPI must define BearerAuth for Authorization: Bearer")
	}

	rootSecurity, ok := document["security"].([]any)
	if !ok {
		t.Fatal("OpenAPI root security must define the shared Data HTTP Adapter authentication")
	}
	assertSecurity := func(where string, value any, wantAnonymous bool) {
		t.Helper()
		requirements, ok := value.([]any)
		if !ok {
			t.Fatalf("%s security must be an array", where)
		}
		if wantAnonymous {
			if len(requirements) != 0 {
				t.Errorf("%s must be anonymous, got %#v", where, requirements)
			}
			return
		}
		got := map[string]bool{}
		for _, raw := range requirements {
			requirement, ok := raw.(map[string]any)
			if !ok || len(requirement) != 1 {
				t.Errorf("%s has invalid security requirement %#v", where, raw)
				continue
			}
			for name, scopes := range requirement {
				if name != "ApiKey" && name != "BearerAuth" {
					t.Errorf("%s declares unsupported security scheme %q", where, name)
				}
				if scopeList, ok := scopes.([]any); !ok || len(scopeList) != 0 {
					t.Errorf("%s scheme %s must not declare OAuth scopes", where, name)
				}
				got[name] = true
			}
		}
		if len(got) != 2 || !got["ApiKey"] || !got["BearerAuth"] {
			t.Errorf("%s must accept ApiKey OR BearerAuth, got %#v", where, got)
		}
	}
	assertSecurity("root", rootSecurity, false)

	// Keep this route inventory aligned with Server.Handler(). A mismatch means
	// either an HTTP route is undocumented or an OpenAPI operation is stale.
	expected := map[string]map[string]bool{
		"/health/live":                                 {"get": true},
		"/health/ready":                                {"get": true},
		"/api/v1/quotes/{symbol}":                      {"get": true},
		"/api/v1/market/quotes":                        {"get": true},
		"/api/v1/market/stocks":                        {"get": true},
		"/api/v1/bars/{symbol}/daily":                  {"get": true},
		"/api/v1/bundles/{symbol}":                     {"get": true},
		"/api/v1/data-sources":                         {"get": true},
		"/api/v1/data-sources/diagnostics":             {"get": true},
		"/api/v1/data-sources/probe":                   {"post": true},
		"/api/v1/data-sources/probe/history":           {"get": true},
		"/api/v1/market/sectors":                       {"get": true},
		"/api/v1/market/indices":                       {"get": true},
		"/api/v1/market/valuations":                    {"get": true},
		"/api/v1/market/index-catalog":                 {"get": true},
		"/api/v1/market/indices/{symbol}/constituents": {"get": true},
		"/api/v1/market/indices/{symbol}/daily":        {"get": true},
		"/api/v1/market/capital-flow":                  {"get": true},
		"/api/v1/market/breadth":                       {"get": true},
		"/api/v1/market/sentiment":                     {"get": true},
		"/api/v1/market/sentiment/history":             {"get": true},
		"/api/v1/market/hot-stocks":                    {"get": true},
		"/api/v1/market/limit-pools":                   {"get": true},
		"/api/v1/market/dragon-tiger":                  {"get": true},
		"/api/v1/market/trading-calendar":              {"get": true},
		"/api/v1/market/session":                       {"get": true},
		"/api/v1/stocks/search":                        {"get": true},
		"/api/v1/stocks/{symbol}/trading-status":       {"get": true},
		"/api/v1/stocks/{symbol}/price-limit-rule":     {"get": true},
		"/api/v1/stocks/{symbol}/profile":              {"get": true},
		"/api/v1/stocks/{symbol}/boards":               {"get": true},
		"/api/v1/stocks/{symbol}/announcements":        {"get": true},
		"/api/v1/stocks/{symbol}/fundamentals":         {"get": true},
		"/api/v1/stocks/{symbol}/chip-distribution":    {"get": true},
		"/api/v1/stocks/{symbol}/capital-flow":         {"get": true},
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI paths object missing")
	}
	if len(paths) != len(expected) {
		t.Errorf("OpenAPI path count=%d, Server.Handler route count=%d", len(paths), len(expected))
	}
	for path, methods := range expected {
		rawPath, exists := paths[path]
		if !exists {
			t.Errorf("OpenAPI route missing: %s", path)
			continue
		}
		pathItem, ok := rawPath.(map[string]any)
		if !ok {
			t.Errorf("OpenAPI path %s is not an object", path)
			continue
		}
		for method := range methods {
			rawOperation, exists := pathItem[method]
			if !exists {
				t.Errorf("OpenAPI operation missing: %s %s", method, path)
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				t.Errorf("OpenAPI operation %s %s is not an object", method, path)
				continue
			}
			security := any(rootSecurity)
			if override, exists := operation["security"]; exists {
				security = override
			}
			anonymous := path == "/health/live" || path == "/health/ready"
			assertSecurity(method+" "+path, security, anonymous)
		}
		for method := range pathItem {
			if method == "parameters" || method == "summary" || method == "description" || method == "servers" {
				continue
			}
			if !methods[method] {
				t.Errorf("OpenAPI documents an unregistered operation: %s %s", method, path)
			}
		}
	}
}

func TestOpenAPIErrorResponsesReferenceRuntimeErrorEnvelope(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	errorSchema := schemas["Error"].(map[string]any)
	errorRequired, ok := errorSchema["required"].([]any)
	if !ok || !containsContractString(errorRequired, "error") {
		t.Fatal("Error schema must require the error envelope")
	}
	errorObject := errorSchema["properties"].(map[string]any)["error"].(map[string]any)
	required, ok := errorObject["required"].([]any)
	if !ok {
		t.Fatal("Error.error.required must be declared")
	}
	for _, field := range []string{"class", "code", "message", "retryable"} {
		if !containsContractString(required, field) {
			t.Errorf("Error.error must require runtime field %q", field)
		}
	}
	if containsContractString(required, "request_id") {
		t.Error("request_id is optional and must not be required")
	}

	paths := document["paths"].(map[string]any)
	checked := 0
	for path, rawPath := range paths {
		pathItem := rawPath.(map[string]any)
		for method, rawOperation := range pathItem {
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				continue
			}
			responses, ok := operation["responses"].(map[string]any)
			if !ok {
				continue
			}
			if path != "/health/live" && path != "/health/ready" {
				if _, exists := responses["401"]; !exists {
					t.Errorf("%s %s is protected by Server.auth but does not declare 401", method, path)
				}
			}
			for status, rawResponse := range responses {
				code, parseErr := strconv.Atoi(status)
				if parseErr != nil || code < 400 {
					continue
				}
				response, ok := rawResponse.(map[string]any)
				if !ok {
					t.Errorf("%s %s response %s must be an object", method, path, status)
					continue
				}
				content, ok := response["content"].(map[string]any)
				if !ok {
					t.Errorf("%s %s response %s has no JSON Error content", method, path, status)
					continue
				}
				jsonContent, ok := content["application/json"].(map[string]any)
				if !ok {
					t.Errorf("%s %s response %s has no application/json schema", method, path, status)
					continue
				}
				schema, ok := jsonContent["schema"].(map[string]any)
				if !ok || schema["$ref"] != "#/components/schemas/Error" {
					t.Errorf("%s %s response %s must reference components.schemas.Error", method, path, status)
					continue
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no error responses were checked")
	}
}

func containsContractString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
		{"DragonTigerSnapshot", "source_version"},
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

func TestAllGoModelFieldsAndRequiredMatchOpenAPISchemas(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	models := []struct {
		name  string
		value any
	}{
		{"StockAnnouncement", model.StockAnnouncement{}},
		{"StockAnnouncementSnapshot", model.StockAnnouncementSnapshot{}},
		{"DragonTigerItem", model.DragonTigerItem{}},
		{"DragonTigerSnapshot", model.DragonTigerSnapshot{}},
		{"MarketSentimentSnapshot", model.MarketSentimentSnapshot{}},
		{"MarketSentimentHistory", model.MarketSentimentHistory{}},
		{"MarketSessionSnapshot", model.MarketSessionSnapshot{}},
		{"MarketStocksSnapshot", model.MarketStocksSnapshot{}},
		{"Quote", model.Quote{}},
		{"DailyBar", model.DailyBar{}},
		{"Bundle", model.Bundle{}},
		{"StockBoard", model.StockBoard{}},
		{"StockBoardSnapshot", model.StockBoardSnapshot{}},
		{"StockBasic", model.StockBasic{}},
		{"StockSearchItem", model.StockSearchItem{}},
		{"StockSearchSnapshot", model.StockSearchSnapshot{}},
		{"SectorRanking", model.SectorRanking{}},
		{"SectorRankingSnapshot", model.SectorRankingSnapshot{}},
		{"MarketIndexQuote", model.MarketIndexQuote{}},
		{"IndexQuoteSnapshot", model.IndexQuoteSnapshot{}},
		{"MarketBreadthSnapshot", model.MarketBreadthSnapshot{}},
		{"LimitPoolItem", model.LimitPoolItem{}},
		{"LimitPoolSnapshot", model.LimitPoolSnapshot{}},
		{"FundamentalMetric", model.FundamentalMetric{}},
		{"FundamentalSection", model.FundamentalSection{}},
		{"FundamentalsSnapshot", model.FundamentalsSnapshot{}},
		{"ValuationItem", model.ValuationItem{}},
		{"ValuationSnapshot", model.ValuationSnapshot{}},
		{"IndexCatalogItem", model.IndexCatalogItem{}},
		{"IndexCatalogSnapshot", model.IndexCatalogSnapshot{}},
		{"IndexConstituent", model.IndexConstituent{}},
		{"IndexConstituentSnapshot", model.IndexConstituentSnapshot{}},
		{"IndexDailyBar", model.IndexDailyBar{}},
		{"IndexDailyBarsSnapshot", model.IndexDailyBarsSnapshot{}},
		{"HotRankingItem", model.HotRankingItem{}},
		{"HotRankingSnapshot", model.HotRankingSnapshot{}},
		{"SourceStatus", model.SourceStatus{}},
		{"ChipDistribution", model.ChipDistribution{}},
		{"MarketCapitalFlowItem", model.MarketCapitalFlowItem{}},
		{"MarketCapitalFlowSnapshot", model.MarketCapitalFlowSnapshot{}},
		{"CapitalFlow", model.CapitalFlow{}},
		{"ProviderProbeFieldIssue", model.ProviderProbeFieldIssue{}},
		{"ProviderProbeCoverageDimension", model.ProviderProbeCoverageDimension{}},
		{"ProviderProbeItem", model.ProviderProbeItem{}},
		{"ProviderProbeCheckResult", model.ProviderProbeCheckResult{}},
		{"ProviderProbeRun", model.ProviderProbeRun{}},
		{"ProviderProbeHistory", model.ProviderProbeHistory{}},
		{"SecurityTradingStatusSnapshot", model.SecurityTradingStatusSnapshot{}},
		{"TradingCalendarDay", model.TradingCalendarDay{}},
		{"TradingCalendarSnapshot", model.TradingCalendarSnapshot{}},
		{"PriceLimitEventEvidence", model.PriceLimitEventEvidence{}},
	}
	if len(models) != 51 {
		t.Fatalf("model contract registry has %d entries, want 51", len(models))
	}

	for _, entry := range models {
		t.Run(entry.name, func(t *testing.T) {
			rawSchema, ok := schemas[entry.name]
			if !ok {
				t.Fatalf("OpenAPI schema %s missing", entry.name)
			}
			schema, ok := rawSchema.(map[string]any)
			if !ok {
				t.Fatalf("OpenAPI schema %s is not an object", entry.name)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("OpenAPI schema %s has no properties", entry.name)
			}
			requiredSet := map[string]bool{}
			if rawRequired, exists := schema["required"]; exists {
				for _, field := range rawRequired.([]any) {
					requiredSet[field.(string)] = true
				}
			}

			typeOfModel := reflect.TypeOf(entry.value)
			expectedFields := map[string]bool{}
			expectedRequired := map[string]bool{}
			for i := 0; i < typeOfModel.NumField(); i++ {
				field := typeOfModel.Field(i)
				if !field.IsExported() {
					continue
				}
				tag := field.Tag.Get("json")
				parts := strings.Split(tag, ",")
				jsonName := parts[0]
				if jsonName == "-" {
					continue
				}
				if jsonName == "" {
					jsonName = field.Name
				}
				expectedFields[jsonName] = true
				omitEmpty := false
				for _, option := range parts[1:] {
					if option == "omitempty" {
						omitEmpty = true
						break
					}
				}
				// encoding/json does not omit zero-valued struct fields with omitempty.
				if !omitEmpty || field.Type.Kind() == reflect.Struct {
					expectedRequired[jsonName] = true
				}
				if field.Type.Kind() == reflect.Pointer && !omitEmpty {
					property, ok := properties[jsonName].(map[string]any)
					if !ok || property["nullable"] != true {
						t.Errorf("%s.%s is a non-omitempty pointer and must be nullable", entry.name, jsonName)
					}
				}
			}

			for field := range expectedFields {
				if _, ok := properties[field]; !ok {
					t.Errorf("%s.%s exists in Go JSON but is missing from OpenAPI", entry.name, field)
				}
			}
			for field := range properties {
				if !expectedFields[field] {
					t.Errorf("%s.%s exists in OpenAPI but not in Go JSON", entry.name, field)
				}
			}
			for field := range expectedRequired {
				if !requiredSet[field] {
					t.Errorf("%s.%s must be required", entry.name, field)
				}
			}
			for field := range requiredSet {
				if !expectedRequired[field] {
					t.Errorf("%s.%s must be optional", entry.name, field)
				}
			}
		})
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

func TestOpenAPIDocumentsHandlerParameterBounds(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI paths object missing")
	}
	checks := []struct {
		path    string
		method  string
		phrases []string
	}{
		{"/api/v1/market/indices/{symbol}/daily", "get", []string{"10 年 / 3660 个自然日"}},
		{"/api/v1/market/trading-calendar", "get", []string{"3660 calendar days"}},
		{"/api/v1/market/sentiment", "get", []string{"historical date is rejected by this endpoint", "Use GET /api/v1/market/sentiment/history to read persisted historical snapshots"}},
		{"/api/v1/market/valuations", "get", []string{"100-token cap applies before deduplication", "normalized duplicate symbols are deduplicated", "empty tokens are rejected"}},
		{"/api/v1/market/indices/{symbol}/constituents", "get", []string{"as_of 仅支持自动选择来源或 source=tushare", "指定 source=hithink 时会被拒绝"}},
		{"/api/v1/market/sentiment/history", "get", []string{"366 calendar days", "1..500", "default of 366"}},
		{"/api/v1/stocks/{symbol}/fundamentals", "get", []string{"2007 through the current calendar year", "single non-future RFC3339 timestamp"}},
		{"/api/v1/market/session", "get", []string{"single RFC3339 timestamp"}},
		{"/api/v1/market/capital-flow", "get", []string{"inclusive and capped at 2000 calendar days", "end must not be later than the current Asia/Shanghai calendar date"}},
		{"/api/v1/data-sources/probe", "post", []string{"8 KiB", "恰好包含一个 JSON 对象", "未知字段拒绝", "必须同时提供", "最多 31 个自然日", "结束日期不能晚于今日"}},
	}
	for _, check := range checks {
		pathItem, ok := paths[check.path].(map[string]any)
		if !ok {
			t.Errorf("OpenAPI path missing: %s", check.path)
			continue
		}
		operation, ok := pathItem[check.method].(map[string]any)
		if !ok {
			t.Errorf("OpenAPI operation missing: %s %s", check.method, check.path)
			continue
		}
		description, ok := operation["description"].(string)
		if !ok {
			t.Errorf("%s %s must document handler parameter constraints", check.method, check.path)
			continue
		}
		for _, phrase := range check.phrases {
			if !strings.Contains(description, phrase) {
				t.Errorf("%s %s description is missing handler constraint %q", check.method, check.path, phrase)
			}
		}
	}
}

func TestOpenAPIParameterInventoryMatchesReviewedHandlerMatrix(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI paths object missing")
	}
	expected := map[string][]string{
		"GET /health/live":                                 {},
		"GET /health/ready":                                {},
		"GET /api/v1/market/quotes":                        {"symbols@query!required"},
		"GET /api/v1/quotes/{symbol}":                      {"symbol@path!required"},
		"GET /api/v1/bars/{symbol}/daily":                  {"symbol@path!required", "start@query!optional", "end@query!optional", "limit@query!optional", "adjustment@query!optional", "as_of@query!optional"},
		"GET /api/v1/bundles/{symbol}":                     {"symbol@path!required", "lookback_days@query!optional", "adjustment@query!optional", "as_of@query!optional"},
		"POST /api/v1/data-sources/probe":                  {},
		"GET /api/v1/data-sources/probe/history":           {"limit@query!optional"},
		"GET /api/v1/data-sources/diagnostics":             {},
		"GET /api/v1/data-sources":                         {},
		"GET /api/v1/stocks/search":                        {"q@query!required", "limit@query!optional"},
		"GET /api/v1/stocks/{symbol}/announcements":        {"symbol@path!required", "start_date@query!optional", "end_date@query!optional", "page@query!optional", "page_size@query!optional"},
		"GET /api/v1/stocks/{symbol}/profile":              {"symbol@path!required"},
		"GET /api/v1/stocks/{symbol}/price-limit-rule":     {"symbol@path!required", "date@query!required"},
		"GET /api/v1/stocks/{symbol}/trading-status":       {"symbol@path!required", "date@query!required", "as_of@query!optional"},
		"GET /api/v1/stocks/{symbol}/fundamentals":         {"symbol@path!required", "year@query!required", "quarter@query!required", "as_of@query!optional"},
		"GET /api/v1/stocks/{symbol}/chip-distribution":    {"symbol@path!required"},
		"GET /api/v1/stocks/{symbol}/capital-flow":         {"symbol@path!required"},
		"GET /api/v1/stocks/{symbol}/boards":               {"symbol@path!required"},
		"GET /api/v1/market/valuations":                    {"symbols@query!required"},
		"GET /api/v1/market/sectors":                       {"type@query!optional", "limit@query!optional", "direction@query!optional", "sort_by@query!optional"},
		"GET /api/v1/market/index-catalog":                 {"tag@query!optional"},
		"GET /api/v1/market/indices/{symbol}/constituents": {"symbol@path!required", "source@query!optional", "as_of@query!optional"},
		"GET /api/v1/market/indices/{symbol}/daily":        {"symbol@path!required", "start@query!required", "end@query!required", "source@query!optional"},
		"GET /api/v1/market/indices":                       {},
		"GET /api/v1/market/trading-calendar":              {"start@query!required", "end@query!required"},
		"GET /api/v1/market/session":                       {"at@query!optional"},
		"GET /api/v1/market/stocks":                        {"page@query!optional", "page_size@query!optional", "include_ma@query!optional", "adjustment@query!optional"},
		"GET /api/v1/market/capital-flow":                  {"start@query!required", "end@query!required"},
		"GET /api/v1/market/sentiment":                     {"date@query!optional"},
		"GET /api/v1/market/sentiment/history":             {"start@query!required", "end@query!required", "limit@query!optional"},
		"GET /api/v1/market/breadth":                       {},
		"GET /api/v1/market/hot-stocks":                    {"type@query!optional", "limit@query!optional"},
		"GET /api/v1/market/dragon-tiger":                  {"date@query!optional", "page@query!optional", "page_size@query!optional"},
		"GET /api/v1/market/limit-pools":                   {"type@query!optional", "date@query!optional", "limit@query!optional"},
	}
	if len(expected) != 35 {
		t.Fatalf("reviewed parameter matrix has %d operations, want 35", len(expected))
	}
	for route, want := range expected {
		parts := strings.SplitN(route, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid route key %q", route)
		}
		pathItem, ok := paths[parts[1]].(map[string]any)
		if !ok {
			t.Errorf("OpenAPI path missing: %s", parts[1])
			continue
		}
		operation, ok := pathItem[strings.ToLower(parts[0])].(map[string]any)
		if !ok {
			t.Errorf("OpenAPI operation missing: %s", route)
			continue
		}
		got := map[string]bool{}
		if rawParameters, exists := operation["parameters"]; exists {
			parameters, ok := rawParameters.([]any)
			if !ok {
				t.Errorf("%s parameters must be an array", route)
				continue
			}
			for _, rawParameter := range parameters {
				parameter, ok := rawParameter.(map[string]any)
				if !ok {
					t.Errorf("%s contains an invalid parameter entry", route)
					continue
				}
				name, nameOK := parameter["name"].(string)
				location, locationOK := parameter["in"].(string)
				if !nameOK || !locationOK {
					t.Errorf("%s parameter must declare name and in", route)
					continue
				}
				required, _ := parameter["required"].(bool)
				state := "optional"
				if required {
					state = "required"
				}
				got[name+"@"+location+"!"+state] = true
			}
		}
		wantSet := map[string]bool{}
		for _, item := range want {
			wantSet[item] = true
		}
		if len(got) != len(wantSet) {
			t.Errorf("%s parameter count=%d, want %d; got=%v", route, len(got), len(wantSet), got)
			continue
		}
		for item := range wantSet {
			if !got[item] {
				t.Errorf("%s missing or misclassified parameter %q", route, item)
			}
		}
		_, hasBody := operation["requestBody"]
		wantBody := route == "POST /api/v1/data-sources/probe"
		if hasBody != wantBody {
			t.Errorf("%s requestBody presence=%v, want %v", route, hasBody, wantBody)
		}
	}
}

func TestOpenAPIParameterDefaultsMatchHandlerDefaults(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	paths := document["paths"].(map[string]any)
	assertParameterSchema := func(path, name string, want map[string]any) {
		t.Helper()
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI path missing: %s", path)
		}
		operation, ok := pathItem["get"].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI GET operation missing: %s", path)
		}
		parameters, _ := operation["parameters"].([]any)
		for _, raw := range parameters {
			parameter, ok := raw.(map[string]any)
			if !ok || parameter["name"] != name {
				continue
			}
			schema, ok := parameter["schema"].(map[string]any)
			if !ok {
				t.Errorf("%s.%s schema missing", path, name)
				return
			}
			for field, expected := range want {
				if !reflect.DeepEqual(schema[field], expected) {
					t.Errorf("%s.%s schema.%s=%#v, want %#v", path, name, field, schema[field], expected)
				}
			}
			return
		}
		t.Errorf("%s parameter %q missing", path, name)
	}

	// 这些默认值与对应 Handler 中的初始化值保持一致。
	assertParameterSchema("/api/v1/market/limit-pools", "limit", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 100, "default": 100,
	})
	assertParameterSchema("/api/v1/market/limit-pools", "type", map[string]any{
		"type": "string", "enum": []any{"up", "down", "broken"}, "default": "up",
	})
	assertParameterSchema("/api/v1/market/stocks", "page", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 10000, "default": 1,
	})
	assertParameterSchema("/api/v1/market/stocks", "page_size", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 100, "default": 50,
	})
	assertParameterSchema("/api/v1/market/stocks", "include_ma", map[string]any{
		"type": "boolean", "default": true,
	})
	assertParameterSchema("/api/v1/market/dragon-tiger", "page_size", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 500, "default": 100,
	})
	assertParameterSchema("/api/v1/market/sentiment/history", "limit", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 500, "default": 366,
	})
	assertParameterSchema("/api/v1/market/sectors", "type", map[string]any{
		"type": "string", "enum": []any{"industry", "concept"}, "default": "industry",
	})
	assertParameterSchema("/api/v1/market/sectors", "limit", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 100, "default": 10,
	})
	assertParameterSchema("/api/v1/market/sectors", "direction", map[string]any{
		"type": "string", "enum": []any{"up", "down"}, "default": "up",
	})
	assertParameterSchema("/api/v1/market/sectors", "sort_by", map[string]any{
		"type": "string", "enum": []any{"change_percent", "amount", "change_5d"}, "default": "change_percent",
	})
	assertParameterSchema("/api/v1/market/hot-stocks", "type", map[string]any{
		"type": "string", "enum": []any{"popular", "surge", "followers"}, "default": "popular",
	})
	assertParameterSchema("/api/v1/market/hot-stocks", "limit", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 100, "default": 10,
	})
	assertParameterSchema("/api/v1/stocks/search", "limit", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 50, "default": 10,
	})
	assertParameterSchema("/api/v1/stocks/{symbol}/announcements", "page", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 1000, "default": 1,
	})
	assertParameterSchema("/api/v1/stocks/{symbol}/announcements", "page_size", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 100, "default": 20,
	})
	assertParameterSchema("/api/v1/data-sources/probe/history", "limit", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 100, "default": 20,
	})
	assertParameterSchema("/api/v1/market/index-catalog", "tag", map[string]any{
		"type": "string", "enum": []any{"cn_concept", "region", "tszs", "industry", "tushare"}, "default": "cn_concept",
	})
	assertParameterSchema("/api/v1/market/dragon-tiger", "page", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 1000, "default": 1,
	})
}

func TestProviderProbeRequestSchemaMatchesHandlerValidation(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	requestSchema, ok := schemas["ProviderProbeRequest"].(map[string]any)
	if !ok {
		t.Fatal("ProviderProbeRequest schema missing")
	}
	if requestSchema["type"] != "object" || requestSchema["additionalProperties"] != false {
		t.Fatalf("ProviderProbeRequest must be a closed JSON object, got %#v", requestSchema)
	}
	properties, ok := requestSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("ProviderProbeRequest properties missing")
	}
	assertProperty := func(name string, want map[string]any) {
		t.Helper()
		property, ok := properties[name].(map[string]any)
		if !ok {
			t.Errorf("ProviderProbeRequest.%s missing or invalid", name)
			return
		}
		for field, expected := range want {
			if !reflect.DeepEqual(property[field], expected) {
				t.Errorf("ProviderProbeRequest.%s.%s=%#v, want %#v", name, field, property[field], expected)
			}
		}
	}
	assertArrayEnum := func(name string, want []string) {
		t.Helper()
		property, ok := properties[name].(map[string]any)
		if !ok {
			t.Errorf("ProviderProbeRequest.%s missing or invalid", name)
			return
		}
		items, ok := property["items"].(map[string]any)
		if !ok {
			t.Errorf("ProviderProbeRequest.%s.items missing", name)
			return
		}
		if items["type"] != "string" {
			t.Errorf("ProviderProbeRequest.%s.items.type=%v, want string", name, items["type"])
		}
		got, ok := items["enum"].([]any)
		if !ok || len(got) != len(want) {
			t.Errorf("ProviderProbeRequest.%s.items.enum=%#v, want %v", name, items["enum"], want)
			return
		}
		for _, expected := range want {
			if !containsContractString(got, expected) {
				t.Errorf("ProviderProbeRequest.%s.items.enum is missing %q", name, expected)
			}
		}
	}

	assertProperty("scope", map[string]any{
		"type": "string", "enum": []any{"all", "core"}, "default": "all",
	})
	assertProperty("markets", map[string]any{"type": "array", "maxItems": 1})
	assertArrayEnum("markets", []string{"SH_MAIN", "SZ_MAIN", "CHINEXT", "STAR", "BSE"})
	assertProperty("symbols", map[string]any{"type": "array", "maxItems": 1})
	symbolsProperty, ok := properties["symbols"].(map[string]any)
	if !ok {
		t.Fatal("ProviderProbeRequest.symbols missing or invalid")
	}
	symbolItems, ok := symbolsProperty["items"].(map[string]any)
	if !ok {
		t.Fatal("ProviderProbeRequest.symbols.items missing")
	}
	if symbolItems["type"] != "string" || symbolItems["pattern"] != `^\d{6}\.(SH|SZ|BJ)$` {
		t.Errorf("ProviderProbeRequest.symbols.items must be a six-digit SH/SZ/BJ symbol string, got %#v", symbolItems)
	}
	assertProperty("checks", map[string]any{"type": "array", "uniqueItems": true})
	assertArrayEnum("checks", []string{"connectivity", "contract", "quality", "freshness", "cross_provider", "storage"})
	assertProperty("capabilities", map[string]any{"type": "array", "uniqueItems": true})
	capabilityItems := properties["capabilities"].(map[string]any)["items"].(map[string]any)
	capabilityEnums, ok := capabilityItems["enum"].([]any)
	if !ok || len(capabilityEnums) != len(model.ProviderProbeCapabilities()) {
		t.Fatalf("ProviderProbeRequest.capabilities enum=%#v, want model capability inventory", capabilityItems["enum"])
	}
	for _, capability := range model.ProviderProbeCapabilities() {
		if !containsContractString(capabilityEnums, capability) {
			t.Errorf("ProviderProbeRequest.capabilities enum is missing %q", capability)
		}
	}
	assertProperty("daily_bar_adjustments", map[string]any{"type": "array", "uniqueItems": true, "maxItems": 3})
	adjustments := properties["daily_bar_adjustments"].(map[string]any)["items"].(map[string]any)
	if adjustments["$ref"] != "#/components/schemas/RequestAdjustmentBasis" {
		t.Errorf("daily_bar_adjustments.items.$ref=%v, want RequestAdjustmentBasis", adjustments["$ref"])
	}
	assertProperty("daily_bar_start_date", map[string]any{"type": "string", "format": "date"})
	assertProperty("daily_bar_end_date", map[string]any{"type": "string", "format": "date"})
	assertProperty("timeout_seconds", map[string]any{
		"type": "integer", "minimum": 1, "maximum": 30, "default": 12,
	})

	paths := document["paths"].(map[string]any)
	probeOperation := paths["/api/v1/data-sources/probe"].(map[string]any)["post"].(map[string]any)
	requestBody := probeOperation["requestBody"].(map[string]any)
	if requestBody["required"] != false {
		t.Error("Provider Probe request body must remain optional; an empty body runs the default all-capability probe")
	}
	bodyContent := requestBody["content"].(map[string]any)
	jsonContent := bodyContent["application/json"].(map[string]any)
	bodySchema := jsonContent["schema"].(map[string]any)
	if bodySchema["$ref"] != "#/components/schemas/ProviderProbeRequest" {
		t.Errorf("Probe requestBody schema ref=%v, want ProviderProbeRequest", bodySchema["$ref"])
	}
}

func TestStockSearchQueryDocumentsTrimmedValueSemantics(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	paths := document["paths"].(map[string]any)
	operation := paths["/api/v1/stocks/search"].(map[string]any)["get"].(map[string]any)
	parameters := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, ok := raw.(map[string]any)
		if !ok || parameter["name"] != "q" {
			continue
		}
		schema := parameter["schema"].(map[string]any)
		if schema["type"] != "string" || schema["minLength"] != 1 || schema["maxLength"] != 64 {
			t.Errorf("stocks/search q schema=%#v, want string length 1..64", schema)
		}
		description, _ := parameter["description"].(string)
		for _, phrase := range []string{"去除首尾空白后不能为空", "64 个 Unicode 字符"} {
			if !strings.Contains(description, phrase) {
				t.Errorf("stocks/search q description missing %q", phrase)
			}
		}
		return
	}
	t.Fatal("stocks/search q parameter missing")
}

func TestOpenAPIParameterTypesAndFormatsMatchReviewedHandlerMatrix(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	paths := document["paths"].(map[string]any)
	expected := map[string]map[string]map[string]any{
		"GET /health/live":                     {},
		"GET /health/ready":                    {},
		"POST /api/v1/data-sources/probe":      {},
		"GET /api/v1/data-sources/diagnostics": {},
		"GET /api/v1/data-sources":             {},
		"GET /api/v1/market/indices":           {},
		"GET /api/v1/market/breadth":           {},
		"GET /api/v1/market/quotes":            {"symbols": {"type": "string"}},
		"GET /api/v1/quotes/{symbol}":          {"symbol": {"type": "string"}},
		"GET /api/v1/bars/{symbol}/daily": {
			"symbol": {"type": "string"}, "start": {"type": "string", "format": "date"},
			"end": {"type": "string", "format": "date"}, "limit": {"type": "integer"},
			"adjustment": {"$ref": "#/components/schemas/RequestAdjustmentBasis"}, "as_of": {"type": "string", "format": "date-time"},
		},
		"GET /api/v1/bundles/{symbol}": {
			"symbol": {"type": "string"}, "lookback_days": {"type": "integer"},
			"adjustment": {"$ref": "#/components/schemas/RequestAdjustmentBasis"}, "as_of": {"type": "string", "format": "date-time"},
		},
		"GET /api/v1/data-sources/probe/history": {"limit": {"type": "integer"}},
		"GET /api/v1/stocks/search":              {"q": {"type": "string"}, "limit": {"type": "integer"}},
		"GET /api/v1/stocks/{symbol}/announcements": {
			"symbol": {"type": "string"}, "start_date": {"type": "string", "format": "date"},
			"end_date": {"type": "string", "format": "date"}, "page": {"type": "integer"}, "page_size": {"type": "integer"},
		},
		"GET /api/v1/stocks/{symbol}/profile":          {"symbol": {"type": "string"}},
		"GET /api/v1/stocks/{symbol}/price-limit-rule": {"symbol": {"type": "string"}, "date": {"type": "string", "format": "date"}},
		"GET /api/v1/stocks/{symbol}/trading-status": {
			"symbol": {"type": "string"}, "date": {"type": "string", "format": "date"}, "as_of": {"type": "string", "format": "date-time"},
		},
		"GET /api/v1/stocks/{symbol}/fundamentals": {
			"symbol": {"type": "string"}, "year": {"type": "integer"}, "quarter": {"type": "integer"}, "as_of": {"type": "string", "format": "date-time"},
		},
		"GET /api/v1/stocks/{symbol}/chip-distribution": {"symbol": {"type": "string"}},
		"GET /api/v1/stocks/{symbol}/capital-flow":      {"symbol": {"type": "string"}},
		"GET /api/v1/stocks/{symbol}/boards":            {"symbol": {"type": "string"}},
		"GET /api/v1/market/valuations":                 {"symbols": {"type": "string"}},
		"GET /api/v1/market/sectors": {
			"type": {"type": "string"}, "limit": {"type": "integer"}, "direction": {"type": "string"}, "sort_by": {"type": "string"},
		},
		"GET /api/v1/market/index-catalog": {"tag": {"type": "string"}},
		"GET /api/v1/market/indices/{symbol}/constituents": {
			"symbol": {"type": "string"}, "source": {"type": "string"}, "as_of": {"type": "string", "format": "date"},
		},
		"GET /api/v1/market/indices/{symbol}/daily": {
			"symbol": {"type": "string"}, "start": {"type": "string", "format": "date"},
			"end": {"type": "string", "format": "date"}, "source": {"type": "string"},
		},
		"GET /api/v1/market/trading-calendar": {"start": {"type": "string", "format": "date"}, "end": {"type": "string", "format": "date"}},
		"GET /api/v1/market/session":          {"at": {"type": "string", "format": "date-time"}},
		"GET /api/v1/market/stocks": {
			"page": {"type": "integer"}, "page_size": {"type": "integer"}, "include_ma": {"type": "boolean"},
			"adjustment": {"$ref": "#/components/schemas/RequestAdjustmentBasis"},
		},
		"GET /api/v1/market/capital-flow": {"start": {"type": "string", "format": "date"}, "end": {"type": "string", "format": "date"}},
		"GET /api/v1/market/sentiment":    {"date": {"type": "string", "format": "date"}},
		"GET /api/v1/market/sentiment/history": {
			"start": {"type": "string", "format": "date"}, "end": {"type": "string", "format": "date"}, "limit": {"type": "integer"},
		},
		"GET /api/v1/market/hot-stocks": {"type": {"type": "string"}, "limit": {"type": "integer"}},
		"GET /api/v1/market/dragon-tiger": {
			"date": {"type": "string", "format": "date"}, "page": {"type": "integer"}, "page_size": {"type": "integer"},
		},
		"GET /api/v1/market/limit-pools": {
			"type": {"type": "string"}, "date": {"type": "string", "format": "date"}, "limit": {"type": "integer"},
		},
	}
	if len(expected) != 35 {
		t.Fatalf("reviewed parameter schema matrix has %d operations, want 35", len(expected))
	}
	for route, wantParameters := range expected {
		parts := strings.SplitN(route, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid route key %q", route)
		}
		pathItem, ok := paths[parts[1]].(map[string]any)
		if !ok {
			t.Errorf("OpenAPI path missing: %s", parts[1])
			continue
		}
		operation, ok := pathItem[strings.ToLower(parts[0])].(map[string]any)
		if !ok {
			t.Errorf("OpenAPI operation missing: %s", route)
			continue
		}
		parameters, _ := operation["parameters"].([]any)
		gotParameters := make(map[string]map[string]any, len(parameters))
		for _, raw := range parameters {
			parameter, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := parameter["name"].(string)
			schema, _ := parameter["schema"].(map[string]any)
			if name != "" && schema != nil {
				gotParameters[name] = schema
			}
		}
		for name, wantSchema := range wantParameters {
			gotSchema, ok := gotParameters[name]
			if !ok {
				t.Errorf("%s parameter %q schema missing", route, name)
				continue
			}
			for field, expectedValue := range wantSchema {
				if !reflect.DeepEqual(gotSchema[field], expectedValue) {
					t.Errorf("%s parameter %q schema.%s=%#v, want %#v", route, name, field, gotSchema[field], expectedValue)
				}
			}
		}
		if len(gotParameters) != len(wantParameters) {
			t.Errorf("%s parameter schema count=%d, reviewed count=%d", route, len(gotParameters), len(wantParameters))
		}
	}
}

func TestBatchQuotesParameterDocumentsHandlerConstraints(t *testing.T) {
	spec, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI specification: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(spec, &document); err != nil {
		t.Fatalf("OpenAPI must be valid YAML: %v", err)
	}
	paths := document["paths"].(map[string]any)
	operation := paths["/api/v1/market/quotes"].(map[string]any)["get"].(map[string]any)
	parameters := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, ok := raw.(map[string]any)
		if !ok || parameter["name"] != "symbols" {
			continue
		}
		schema := parameter["schema"].(map[string]any)
		if schema["type"] != "string" {
			t.Errorf("market/quotes symbols type=%v, want string", schema["type"])
		}
		description, _ := parameter["description"].(string)
		for _, phrase := range []string{"最多 50 个", "不允许重复", "去除首尾空白再规范化"} {
			if !strings.Contains(description, phrase) {
				t.Errorf("market/quotes symbols description missing %q", phrase)
			}
		}
		return
	}
	t.Fatal("market/quotes symbols parameter missing")
}
