package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type tushareIndexWeightRequest struct {
	APIName string            `json:"api_name"`
	Params  map[string]string `json:"params"`
}

func writeTushareTestTable(w http.ResponseWriter, fields []string, items [][]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": map[string]any{"fields": fields, "items": items},
	})
}

func tushareTestMonthWindow(now time.Time, offset int) (string, string) {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -offset, 0)
	last := first.AddDate(0, 1, -1)
	return first.Format("20060102"), last.Format("20060102")
}

func TestTushareIndexConstituentsQueriesOnlyNewestAvailableMonth(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	wantStart, wantEnd := tushareTestMonthWindow(now, 0)
	var weightRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch request.APIName {
		case "index_weight":
			weightRequests++
			if request.Params["index_code"] != "000300.SH" || request.Params["start_date"] != wantStart || request.Params["end_date"] != wantEnd {
				t.Errorf("unexpected index_weight request params: %#v", request.Params)
			}
			writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{
				{"000300.SH", "600519.SH", now.Format("20060102"), 3.5},
			})
		case "stock_basic":
			writeTushareTestTable(w, []string{"ts_code", "name"}, [][]any{{"600519.SH", "贵州茅台"}})
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected api", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := provider.FetchIndexConstituents(context.Background(), "000300.SH")
	if err != nil {
		t.Fatal(err)
	}
	if weightRequests != 1 {
		t.Fatalf("expected one monthly index_weight request, got %d", weightRequests)
	}
	if got.ProviderReportDate != now.Format("2006-01-02") || got.Total != 1 || got.Items[0].Symbol != "600519.SH" {
		t.Fatalf("unexpected index constituent snapshot: %+v", got)
	}
}

func TestTushareIndexConstituentsFallsBackToPreviousMonth(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	currentStart, currentEnd := tushareTestMonthWindow(now, 0)
	previousStart, previousEnd := tushareTestMonthWindow(now, 1)
	previousMonthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location).AddDate(0, 0, -1)
	var weightStarts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch request.APIName {
		case "index_weight":
			weightStarts = append(weightStarts, request.Params["start_date"])
			switch request.Params["start_date"] {
			case currentStart:
				if request.Params["end_date"] != currentEnd {
					t.Errorf("current month end = %q, want %q", request.Params["end_date"], currentEnd)
				}
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{})
			case previousStart:
				if request.Params["end_date"] != previousEnd {
					t.Errorf("previous month end = %q, want %q", request.Params["end_date"], previousEnd)
				}
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{
					{"000300.SH", "600519.SH", previousMonthEnd.Format("20060102"), 3.25},
				})
			default:
				t.Errorf("unexpected index_weight start date %q", request.Params["start_date"])
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{})
			}
		case "stock_basic":
			writeTushareTestTable(w, []string{"ts_code", "name"}, [][]any{{"600519.SH", "贵州茅台"}})
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected api", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := provider.FetchIndexConstituents(context.Background(), "000300.SH")
	if err != nil {
		t.Fatal(err)
	}
	if len(weightStarts) != 2 || weightStarts[0] != currentStart || weightStarts[1] != previousStart {
		t.Fatalf("expected newest-first monthly requests, got %v", weightStarts)
	}
	if got.ProviderReportDate != previousMonthEnd.Format("2006-01-02") || got.Total != 1 {
		t.Fatalf("unexpected previous-month constituent snapshot: %+v", got)
	}
}
