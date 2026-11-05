package provider

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/baostock"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

type pytdxRawCrossProviderField struct {
	Field      string  `json:"field"`
	Status     string  `json:"status"`
	Left       float64 `json:"left,omitempty"`
	Right      float64 `json:"right,omitempty"`
	Difference float64 `json:"difference,omitempty"`
	Tolerance  float64 `json:"tolerance,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

type pytdxRawCrossProviderResult struct {
	Status                string                       `json:"status"`
	Reason                string                       `json:"reason,omitempty"`
	PytdxQualityStatus    string                       `json:"pytdx_quality_status,omitempty"`
	BaoStockQualityStatus string                       `json:"baostock_quality_status,omitempty"`
	BaoStockRows          int                          `json:"baostock_rows,omitempty"`
	BaoStockFirstDate     string                       `json:"baostock_first_date,omitempty"`
	BaoStockLastDate      string                       `json:"baostock_last_date,omitempty"`
	ComparedDates         []string                     `json:"compared_dates,omitempty"`
	Fields                []pytdxRawCrossProviderField `json:"fields,omitempty"`
	Error                 string                       `json:"error,omitempty"`
}
type pytdxHistoricalProbeItem struct {
	Symbol             string                       `json:"symbol"`
	Adjustment         model.AdjustmentBasis        `json:"adjustment"`
	Status             string                       `json:"status"`
	Rows               int                          `json:"rows,omitempty"`
	FirstDate          string                       `json:"first_date,omitempty"`
	LastDate           string                       `json:"last_date,omitempty"`
	AdjustmentMethods  []string                     `json:"adjustment_methods,omitempty"`
	AdjustmentRefDates []string                     `json:"adjustment_reference_dates,omitempty"`
	QualityCounts      map[string]int               `json:"quality_counts,omitempty"`
	Error              string                       `json:"error,omitempty"`
	RawCrossProvider   *pytdxRawCrossProviderResult `json:"raw_cross_provider,omitempty"`
	Bars               []model.DailyBar             `json:"bars,omitempty"`
}

type pytdxHistoricalProbeReport struct {
	GeneratedAt string                     `json:"generated_at"`
	StartDate   string                     `json:"start_date"`
	EndDate     string                     `json:"end_date"`
	Items       []pytdxHistoricalProbeItem `json:"items"`
}

// TestLivePytdxHistoricalDailyBars verifies the existing TDX/GBBQ adjustment
// path on the same historical golden windows. It is opt-in and never gates CI.
func comparePytdxAndBaoStockRawBars(ctx context.Context, symbol, startDate, endDate string, pytdxBars []model.DailyBar, baostockProvider *Baostock) *pytdxRawCrossProviderResult {
	result := &pytdxRawCrossProviderResult{Status: CrossProviderDailyNotComparable}
	if baostockProvider == nil {
		result.Reason = "baostock_provider_unavailable"
		result.Error = "BaoStock provider is not configured"
		return result
	}
	baoBars, err := baostockProvider.FetchDailyBars(ctx, symbol, startDate, endDate, 40, model.AdjustmentNone)
	if err != nil {
		_, safe := liveProbeErrorSummary(err)
		result.Reason = "baostock_provider_unavailable"
		result.Error = safe
		return result
	}
	if message := liveProbeDailyContractError(baoBars, "baostock", "baostock", symbol, model.AdjustmentNone, startDate, endDate, 40); message != "" {
		result.Reason = "baostock_contract_invalid"
		result.Error = message
		return result
	}
	result.BaoStockRows = len(baoBars)
	for _, bar := range baoBars {
		if result.BaoStockFirstDate == "" || bar.TradeDate < result.BaoStockFirstDate {
			result.BaoStockFirstDate = bar.TradeDate
		}
		if bar.TradeDate > result.BaoStockLastDate {
			result.BaoStockLastDate = bar.TradeDate
		}
	}
	left := crossProviderSnapshotFromDailyBars(pytdxBars)
	right := crossProviderSnapshotFromDailyBars(baoBars)
	result.PytdxQualityStatus = left.QualityStatus
	result.BaoStockQualityStatus = right.QualityStatus
	comparison := CompareCrossProviderDailyBars(left, right)
	result.Status = comparison.Status
	result.Reason = comparison.Reason
	result.ComparedDates = comparison.ComparedDates
	for _, field := range comparison.Fields {
		result.Fields = append(result.Fields, pytdxRawCrossProviderField{
			Field: field.Field, Status: field.Status, Left: field.Left, Right: field.Right,
			Difference: field.Difference, Tolerance: field.Tolerance, Reason: field.Reason,
		})
	}
	return result
}

func crossProviderSnapshotFromDailyBars(bars []model.DailyBar) CrossProviderDailySnapshot {
	snapshot := CrossProviderDailySnapshot{QualityStatus: string(model.QualityUnknown)}
	if len(bars) == 0 {
		return snapshot
	}
	first := bars[0]
	snapshot.Provider = first.Source
	snapshot.SourceVersion = first.SourceVersion
	snapshot.Symbol = first.Symbol
	snapshot.AdjustmentBasis = string(first.AdjustmentBasis)
	snapshot.QualityStatus = string(model.QualityValid)
	for _, bar := range bars {
		if bar.QualityStatus != model.QualityValid {
			snapshot.QualityStatus = string(model.QualityDegraded)
		}
		snapshot.Bars = append(snapshot.Bars, CrossProviderDailyBar{
			TradeDate: bar.TradeDate, Open: bar.Open, High: bar.High, Low: bar.Low, Close: bar.Close,
			Volume: valueOrZero(bar.Volume), VolumeUnit: bar.VolumeUnit, VolumeGranularity: CrossProviderVolumeGranularity(bar.RawVolumeUnit), Amount: bar.Amount, AmountUnit: bar.AmountUnit,
			AdjustmentMethod: bar.AdjustmentMethod, AdjustmentRefDate: bar.AdjustmentRefDate,
		})
	}
	return snapshot
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return -1
	}
	return *value
}
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
	baostockClient := baostock.NewClient(baostock.Config{ConnectTimeout: 8 * time.Second, RequestTimeout: 20 * time.Second})
	defer func() { _ = baostockClient.Close() }()
	baostockProvider := NewBaostock(baostockClient)
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
			} else {
				item.RawCrossProvider = comparePytdxAndBaoStockRawBars(ctx, symbol, startDate, endDate, bars, baostockProvider)
				if item.RawCrossProvider.Status == CrossProviderDailyMismatch {
					failures = append(failures, symbol+"/raw cross-provider mismatch")
				}
				if item.RawCrossProvider.Error != "" && item.RawCrossProvider.Reason == "baostock_contract_invalid" {
					failures = append(failures, symbol+"/BaoStock raw contract invalid: "+item.RawCrossProvider.Error)
				}
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
