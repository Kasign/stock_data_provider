package provider

import (
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
