package provider

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

type pytdxHistoricalProbeItem struct {
	Symbol             string                `json:"symbol"`
	Adjustment         model.AdjustmentBasis `json:"adjustment"`
	Status             string                `json:"status"`
	Rows               int                   `json:"rows,omitempty"`
	FirstDate          string                `json:"first_date,omitempty"`
	LastDate           string                `json:"last_date,omitempty"`
	AdjustmentMethods  []string              `json:"adjustment_methods,omitempty"`
	AdjustmentRefDates []string              `json:"adjustment_reference_dates,omitempty"`
	QualityCounts      map[string]int        `json:"quality_counts,omitempty"`
	Error              string                `json:"error,omitempty"`
	Bars               []model.DailyBar      `json:"bars,omitempty"`
}

type pytdxHistoricalProbeReport struct {
	GeneratedAt string                     `json:"generated_at"`
	StartDate   string                     `json:"start_date"`
	EndDate     string                     `json:"end_date"`
	Items       []pytdxHistoricalProbeItem `json:"items"`
}

// TestLivePytdxHistoricalDailyBars verifies the existing TDX/GBBQ adjustment
// path on the same historical golden windows. It is opt-in and never gates CI.
func TestLivePytdxHistoricalDailyBars(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_PYTDX_HISTORICAL_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_PYTDX_HISTORICAL_TESTS=1 to probe Pytdx historical bars")
	}
	startDate := strings.TrimSpace(os.Getenv("LIVE_PROBE_DAILY_START_DATE"))
	if startDate == "" {
		startDate = "2025-07-07"
	}
	endDate := strings.TrimSpace(os.Getenv("LIVE_PROBE_DAILY_END_DATE"))
	if endDate == "" {
		endDate = "2025-07-17"
	}
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		t.Fatalf("invalid historical start date: %v", err)
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		t.Fatalf("invalid historical end date: %v", err)
	}
	symbols := []string{"600036.SH", "002982.SZ"}
	if configured := strings.TrimSpace(os.Getenv("LIVE_PROBE_DAILY_SYMBOLS")); configured != "" {
		symbols = strings.Split(configured, ",")
	}
	client := NewPytdx(20 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	report := pytdxHistoricalProbeReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		StartDate:   startDate, EndDate: endDate,
		Items: make([]pytdxHistoricalProbeItem, 0, len(symbols)*3),
	}
	var failures []string
	for _, rawSymbol := range symbols {
		symbol := strings.ToUpper(strings.TrimSpace(rawSymbol))
		if symbol == "" {
			t.Fatalf("LIVE_PROBE_DAILY_SYMBOLS contains an empty symbol")
		}
		for _, adjustment := range []model.AdjustmentBasis{
			model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward,
		} {
			item := pytdxHistoricalProbeItem{Symbol: symbol, Adjustment: adjustment}
			bars, err := client.FetchDailyBars(ctx, symbol, startDate, endDate, 40, adjustment)
			if err != nil {
				_, safe := liveProbeErrorSummary(err)
				item.Error = safe
				item.Status = "failed"
				failures = append(failures, symbol+"/"+string(adjustment)+": "+safe)
				report.Items = append(report.Items, item)
				continue
			}
			if message := liveProbeDailyContractError(bars, "pytdx", "pytdx", symbol, adjustment, startDate, endDate, 40); message != "" {
				item.Status, item.Error = "contract_invalid", message
				failures = append(failures, symbol+"/"+string(adjustment)+": "+message)
				report.Items = append(report.Items, item)
				continue
			}
			item.Status = "returned"
			item.Rows = len(bars)
			item.Bars = bars
			item.QualityCounts = make(map[string]int)
			methods, refDates := make(map[string]struct{}), make(map[string]struct{})
			for _, bar := range bars {
				if item.FirstDate == "" || bar.TradeDate < item.FirstDate {
					item.FirstDate = bar.TradeDate
				}
				if bar.TradeDate > item.LastDate {
					item.LastDate = bar.TradeDate
				}
				item.QualityCounts[string(bar.QualityStatus)]++
				if method := strings.TrimSpace(bar.AdjustmentMethod); method != "" {
					methods[method] = struct{}{}
				}
				if refDate := strings.TrimSpace(bar.AdjustmentRefDate); refDate != "" {
					refDates[refDate] = struct{}{}
				}
			}
			for value := range methods {
				item.AdjustmentMethods = append(item.AdjustmentMethods, value)
			}
			for value := range refDates {
				item.AdjustmentRefDates = append(item.AdjustmentRefDates, value)
			}
			sort.Strings(item.AdjustmentMethods)
			sort.Strings(item.AdjustmentRefDates)
			if adjustment != model.AdjustmentNone {
				wantMethod := "pytdx_qfq"
				if adjustment == model.AdjustmentBackward {
					wantMethod = "pytdx_hfq"
				}
				if len(item.AdjustmentMethods) != 1 || item.AdjustmentMethods[0] != wantMethod {
					item.Status = "contract_invalid"
					item.Error = "adjustment method metadata mismatch"
					failures = append(failures, symbol+"/"+string(adjustment)+": "+item.Error)
				}
				if len(item.AdjustmentRefDates) != 1 {
					item.Status = "contract_invalid"
					item.Error = "adjustment reference date metadata missing or inconsistent"
					failures = append(failures, symbol+"/"+string(adjustment)+": "+item.Error)
				} else if _, dateErr := time.Parse("2006-01-02", item.AdjustmentRefDates[0]); dateErr != nil {
					item.Status = "contract_invalid"
					item.Error = "adjustment reference date metadata is invalid"
					failures = append(failures, symbol+"/"+string(adjustment)+": "+item.Error)
				} else if adjustment == model.AdjustmentForward && item.AdjustmentRefDates[0] < item.LastDate {
					item.Status = "contract_invalid"
					item.Error = "qfq reference date precedes the last returned trading date"
					failures = append(failures, symbol+"/"+string(adjustment)+": "+item.Error)
				} else if adjustment == model.AdjustmentBackward && item.AdjustmentRefDates[0] > item.FirstDate {
					item.Status = "contract_invalid"
					item.Error = "hfq reference date follows the first returned trading date"
					failures = append(failures, symbol+"/"+string(adjustment)+": "+item.Error)
				}
			} else if len(item.AdjustmentRefDates) != 0 {
				item.Status = "contract_invalid"
				item.Error = "unadjusted daily bars must not declare an adjustment reference date"
				failures = append(failures, symbol+"/"+string(adjustment)+": "+item.Error)
			}
			report.Items = append(report.Items, item)
		}
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal Pytdx historical report: %v", err)
	}
	t.Logf("LIVE_PYTDX_HISTORICAL_REPORT_BEGIN\n%s\nLIVE_PYTDX_HISTORICAL_REPORT_END", encoded)
	if outputPath := strings.TrimSpace(os.Getenv("LIVE_PYTDX_HISTORICAL_REPORT_PATH")); outputPath != "" {
		if writeErr := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); writeErr != nil {
			t.Errorf("write Pytdx historical report: %v", writeErr)
		}
	}
	if len(failures) > 0 {
		t.Errorf("Pytdx historical probe failures: %s", strings.Join(failures, " | "))
	}
}
