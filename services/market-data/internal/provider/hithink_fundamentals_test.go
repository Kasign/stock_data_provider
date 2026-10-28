package provider

import (
	"encoding/json"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func newHithinkFundamentalsTestServer(t *testing.T, abilitiesJSON string) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	statementRequests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-api-key") != "unit-test-key" {
			http.Error(w, "missing API key header", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/a-share/financials/indicators":
			if r.URL.Query().Get("thscode") != "600519.SH" || r.URL.Query().Get("report") != "2026-2" {
				http.Error(w, "unexpected indicators query", http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprintf(w, `{"code":0,"data":{"thscode":"600519.SH","report":"2026-2","abilities":%s}}`, abilitiesJSON)
		case "/api/a-share/financials/income-statements",
			"/api/a-share/financials/balance-sheets",
			"/api/a-share/financials/cash-flow-statements":
			statementRequests.Add(1)
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"item":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, statementRequests
}

func TestHithinkFundamentalsMissingStandardAbilityBecomesUnknown(t *testing.T) {
	abilities := `[
		{"ability":"profitability","indicators":[{"index_id":"roe","value":"0.1"}]},
		{"ability":"operation","indicators":[{"index_id":"turnover_rate","value":"1.2"}]},
		{"ability":"growth","indicators":[{"index_id":"revenue_growth","value":"0.08"}]},
		{"ability":"solvency","indicators":[{"index_id":"current_ratio","value":"1.5"}]},
		{"ability":"cash-flow","indicators":[{"index_id":"cash_flow_ratio","value":"0.7"}]}
	]`
	server, _ := newHithinkFundamentalsTestServer(t, abilities)
	hithink := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key")

	got, err := hithink.FetchFundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil {
		t.Fatalf("FetchFundamentals should preserve the partial snapshot: %v", err)
	}
	if !got.HasData || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("partial fundamentals must be available but degraded: %+v", got)
	}

	var found bool
	for _, section := range got.Sections {
		if section.Category != "dupont" {
			continue
		}
		found = true
		if section.HasData || len(section.Metrics) != 0 || section.QualityStatus != model.QualityUnknown {
			t.Fatalf("missing standard ability must be unknown and empty, got %+v", section)
		}
		if !containsHithinkReason(section.QualityReasons, "provider_fundamental_ability_not_returned") {
			t.Fatalf("missing standard ability reason not recorded: %+v", section.QualityReasons)
		}
	}
	if !found {
		t.Fatalf("missing standard ability should still have an explicit section: %+v", got.Sections)
	}
	if !containsHithinkReason(got.QualityReasons, "provider_fundamental_ability_missing:dupont") {
		t.Fatalf("snapshot-level missing ability reason not recorded: %+v", got.QualityReasons)
	}
}

func TestHithinkFundamentalsRejectsUnknownOrDuplicateAbilityBeforeStatementCalls(t *testing.T) {
	cases := []struct {
		name      string
		abilities string
		wantError string
	}{
		{
			name:      "unknown ability",
			abilities: `[{"ability":"invented-group","indicators":[]}] `,
			wantError: "未知能力分区",
		},
		{
			name: "duplicate normalized category",
			abilities: `[
				{"ability":"operation","indicators":[]},
				{"ability":"operating","indicators":[]}
			]`,
			wantError: "重复能力分区",
		},
		{
			name:      "duplicate metric identifier",
			abilities: `[{"ability":"profitability","indicators":[{"index_id":"roe","value":"0.1"},{"index_id":"ROE","value":"0.2"}]}]`,
			wantError: "重复指标 ID",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, statementRequests := newHithinkFundamentalsTestServer(t, tc.abilities)
			hithink := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key")
			_, err := hithink.FetchFundamentals(context.Background(), "600519.SH", 2026, 2)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
			}
			if got := statementRequests.Load(); got != 0 {
				t.Fatalf("invalid ability response should be rejected before statement calls, got %d calls", got)
			}
		})
	}
}

func containsHithinkReason(reasons []string, expected string) bool {
	for _, reason := range reasons {
		if reason == expected {
			return true
		}
	}
	return false
}


func TestHithinkFundamentalsRejectsDuplicateStatementRowsForTargetReport(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	periodEnd := time.Date(2026, time.June, 30, 0, 0, 0, 0, location).UnixMilli()
	publicationDate := time.Date(2026, time.August, 25, 0, 0, 0, 0, location).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/a-share/financials/indicators":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"thscode":"600519.SH","report":"2026-2","abilities":[]}}`)
		case "/api/a-share/financials/income-statements":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"item": []map[string]any{
					{"thscode":"600519.SH","period_end_ms":periodEnd,"report_date_ms":publicationDate,"operating_income":1000.0},
					{"thscode":"600519.SH","period_end_ms":periodEnd,"report_date_ms":publicationDate,"operating_income":2000.0},
				}},
			})
		case "/api/a-share/financials/balance-sheets", "/api/a-share/financials/cash-flow-statements":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"item":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key")
	got, err := provider.FetchFundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil {
		t.Fatalf("partial fundamentals should remain usable when one statement endpoint is ambiguous: %v", err)
	}
	var income *model.FundamentalSection
	for i := range got.Sections {
		if got.Sections[i].Category == "income_statement" {
			income = &got.Sections[i]
			break
		}
	}
	if income == nil {
		t.Fatalf("income-statement section is missing: %+v", got.Sections)
	}
	if income.HasData || len(income.Metrics) != 0 {
		t.Fatalf("ambiguous duplicate rows must not be used as a statement: %+v", *income)
	}
	if !containsHithinkReason(income.QualityReasons, "provider_statement_unavailable_or_target_period_missing") {
		t.Fatalf("duplicate report rows should be represented as unavailable, got %+v", *income)
	}
	if !containsHithinkReason(got.QualityReasons, "income_statement_provider_unavailable") {
		t.Fatalf("snapshot should preserve statement-unavailable reason, got %+v", got.QualityReasons)
	}
}
