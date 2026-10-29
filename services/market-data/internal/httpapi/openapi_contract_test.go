package httpapi

import (
	"os"
	"strings"
	"testing"

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
}
