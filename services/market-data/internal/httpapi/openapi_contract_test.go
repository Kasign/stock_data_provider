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
		"/health/live":                                  {"get": true},
		"/health/ready":                                 {"get": true},
		"/api/v1/quotes/{symbol}":                       {"get": true},
		"/api/v1/market/quotes":                         {"get": true},
		"/api/v1/market/stocks":                         {"get": true},
		"/api/v1/bars/{symbol}/daily":                   {"get": true},
		"/api/v1/bundles/{symbol}":                      {"get": true},
		"/api/v1/data-sources":                          {"get": true},
		"/api/v1/data-sources/diagnostics":              {"get": true},
		"/api/v1/data-sources/probe":                    {"post": true},
		"/api/v1/data-sources/probe/history":            {"get": true},
		"/api/v1/market/sectors":                        {"get": true},
		"/api/v1/market/indices":                        {"get": true},
		"/api/v1/market/valuations":                     {"get": true},
		"/api/v1/market/index-catalog":                  {"get": true},
		"/api/v1/market/indices/{symbol}/constituents":  {"get": true},
		"/api/v1/market/indices/{symbol}/daily":         {"get": true},
		"/api/v1/market/capital-flow":                   {"get": true},
		"/api/v1/market/breadth":                        {"get": true},
		"/api/v1/market/sentiment":                      {"get": true},
		"/api/v1/market/sentiment/history":              {"get": true},
		"/api/v1/market/hot-stocks":                     {"get": true},
		"/api/v1/market/limit-pools":                    {"get": true},
		"/api/v1/market/dragon-tiger":                   {"get": true},
		"/api/v1/market/trading-calendar":               {"get": true},
		"/api/v1/market/session":                        {"get": true},
		"/api/v1/stocks/search":                         {"get": true},
		"/api/v1/stocks/{symbol}/trading-status":        {"get": true},
		"/api/v1/stocks/{symbol}/price-limit-rule":      {"get": true},
		"/api/v1/stocks/{symbol}/profile":               {"get": true},
		"/api/v1/stocks/{symbol}/boards":                {"get": true},
		"/api/v1/stocks/{symbol}/announcements":         {"get": true},
		"/api/v1/stocks/{symbol}/fundamentals":          {"get": true},
		"/api/v1/stocks/{symbol}/chip-distribution":     {"get": true},
		"/api/v1/stocks/{symbol}/capital-flow":          {"get": true},
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
