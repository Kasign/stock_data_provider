package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/baostock"
)

type pytdxBaoStockEventReport struct {
	GeneratedAt string                      `json:"generated_at"`
	Year        string                      `json:"year"`
	Items       []pytdxBaoStockEventItem    `json:"items"`
}

type pytdxBaoStockEventItem struct {
	Symbol             string  `json:"symbol"`
	EventDate          string  `json:"event_date"`
	CashDividend       float64 `json:"cash_dividend_per_10"`
	RightsPrice        float64 `json:"rights_price"`
	BonusShares        float64 `json:"bonus_shares_per_10"`
	RightsShares       float64 `json:"rights_shares_per_10"`
	PreviousClose      float64 `json:"previous_close"`
	ExpectedPreClose   float64 `json:"pytdx_theoretical_preclose"`
	BaoStockPreClose   float64 `json:"baostock_preclose"`
	FactorDelta          float64 `json:"baostock_back_factor_delta"`
	ForwardFactorDelta   float64 `json:"baostock_forward_factor_delta"`
	ImpliedRatio         float64 `json:"pytdx_implied_adjustment_ratio"`
	PreCloseDiff         float64 `json:"preclose_diff"`
	FactorRatioDiff      float64 `json:"factor_ratio_diff"`
	ForwardBackDeltaDiff float64 `json:"forward_back_factor_delta_diff"`
	Status             string  `json:"status"`
	Error              string  `json:"error,omitempty"`
}

// TestLivePytdxBaoStockCorporateActionConsistency compares source event inputs,
// not qfq/hfq prices: Pytdx applies an affine transform while BaoStock exposes
// cumulative scalar adjustment factors.
func TestLivePytdxBaoStockCorporateActionConsistency(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_PYTDX_HISTORICAL_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_PYTDX_HISTORICAL_TESTS=1 to compare Pytdx and BaoStock corporate actions")
	}

	const year = "2025"
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	report := pytdxBaoStockEventReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Year:        year,
		Items:       make([]pytdxBaoStockEventItem, 0, 8),
	}
	var failures []string

	pytdxClient := NewPytdx(20 * time.Second)
	baostockClient := baostock.NewClient(baostock.Config{
		ConnectTimeout: 8 * time.Second,
		RequestTimeout: 20 * time.Second,
	})
	defer func() { _ = baostockClient.Close() }()

	for _, symbol := range []string{"600036.SH", "002982.SZ"} {
		if err := pytdxClient.waitForRateLimit(ctx); err != nil {
			failures = append(failures, symbol+": rate limit: "+err.Error())
			continue
		}
		tdxClient, err := pytdxClient.dailyClient(ctx)
		if err != nil {
			failures = append(failures, symbol+": connect Pytdx: "+err.Error())
			continue
		}
		gbbq, gbbqErr := tdxClient.GetGbbq(pytdxWireSymbol(symbol))
		closeErr := tdxClient.Close()
		if gbbqErr != nil {
			failures = append(failures, symbol+": query GBBQ: "+gbbqErr.Error())
			continue
		}
		if closeErr != nil {
			failures = append(failures, symbol+": close Pytdx: "+closeErr.Error())
			continue
		}
		if gbbq == nil {
			failures = append(failures, symbol+": empty GBBQ response")
			continue
		}

		baostockCode := strings.ToLower(symbol[7:]) + "." + symbol[:6]
		factors, factorErr := baostockClient.QueryAdjustmentFactors(ctx, baostockCode, "2000-01-01", time.Now().In(location).Format("2006-01-02"))
		if factorErr != nil {
			failures = append(failures, symbol+": query BaoStock factors: "+factorErr.Error())
			continue
		}

		matchedEvents := 0
		for _, rawEvent := range gbbq.List {
			if rawEvent == nil || !rawEvent.IsXRXD() {
				continue
			}
			event := rawEvent.XRXD()
			if event == nil {
				continue
			}
			eventDate := event.Time.In(location).Format("2006-01-02")
			if !strings.HasPrefix(eventDate, year+"-") {
				continue
			}
			matchedEvents++
			item := pytdxBaoStockEventItem{
				Symbol: symbol, EventDate: eventDate,
				CashDividend: event.Fenhong, RightsPrice: event.Peigujia,
				BonusShares: event.Songzhuangu, RightsShares: event.Peigu,
				Status: "matched",
			}

			factorIndex := -1
			for index := range factors {
				if factors[index].OperateDate == eventDate {
					factorIndex = index
					break
				}
			}
			if factorIndex < 0 {
				item.Status = "mismatch"
				item.Error = "Pytdx GBBQ event date has no BaoStock adjustment-factor row"
				report.Items = append(report.Items, item)
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
				continue
			}

			factor := factors[factorIndex]
			previousBackFactor, previousForwardFactor := 1.0, 1.0
			if factorIndex > 0 {
				previousBackFactor = factors[factorIndex-1].BackwardFactor
				previousForwardFactor = factors[factorIndex-1].ForwardFactor
			}
			if previousBackFactor <= 0 || previousForwardFactor <= 0 {
				item.Status = "invalid"
				item.Error = "BaoStock previous cumulative back factor is not positive"
				report.Items = append(report.Items, item)
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
				continue
			}
			item.FactorDelta = factor.BackwardFactor / previousBackFactor
			item.ForwardFactorDelta = factor.ForwardFactor / previousForwardFactor

			date, parseErr := time.ParseInLocation("2006-01-02", eventDate, location)
			if parseErr != nil {
				item.Status = "invalid"
				item.Error = "invalid GBBQ event date: " + parseErr.Error()
				report.Items = append(report.Items, item)
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
				continue
			}
			bars, barsErr := baostockClient.QueryDailyKLines(ctx, baostockCode, date.AddDate(0, 0, -10).Format("2006-01-02"), date.AddDate(0, 0, 10).Format("2006-01-02"), "3")
			if barsErr != nil {
				item.Status = "failed"
				item.Error = "query BaoStock raw bars: " + barsErr.Error()
				report.Items = append(report.Items, item)
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
				continue
			}
			var previous, exDate *baostock.DailyKLine
			for index := range bars {
				bar := &bars[index]
				if bar.Date < eventDate {
					previous = bar
					continue
				}
				if bar.Date >= eventDate {
					exDate = bar
					break
				}
			}
			if previous == nil || exDate == nil {
				item.Status = "not_comparable"
				item.Error = "BaoStock raw bars do not bracket the corporate-action date"
				report.Items = append(report.Items, item)
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
				continue
			}
			previousClose, previousErr := strconv.ParseFloat(previous.Close, 64)
			baostockPreClose, preCloseErr := strconv.ParseFloat(exDate.PreClose, 64)
			if previousErr != nil || preCloseErr != nil || previousClose <= 0 || baostockPreClose <= 0 {
				item.Status = "invalid"
				item.Error = fmt.Sprintf("invalid BaoStock close/preclose values: previous=%q exdate=%q", previous.Close, exDate.PreClose)
				report.Items = append(report.Items, item)
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
				continue
			}
			m := (10 + event.Songzhuangu + event.Peigu) / 10
			c := (event.Fenhong - event.Peigu*event.Peigujia) / 10
			if m == 0 {
				item.Status = "invalid"
				item.Error = "Pytdx corporate-action multiplier is zero"
				report.Items = append(report.Items, item)
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
				continue
			}
			item.PreviousClose = previousClose
			item.ExpectedPreClose = (previousClose - c) / m
			if item.ExpectedPreClose <= 0 || math.IsNaN(item.ExpectedPreClose) || math.IsInf(item.ExpectedPreClose, 0) {
				item.Status = "invalid"
				item.Error = "Pytdx event terms produced a non-positive theoretical ex-date preclose"
				report.Items = append(report.Items, item)
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
				continue
			}
			item.BaoStockPreClose = baostockPreClose
			item.ImpliedRatio = previousClose / item.ExpectedPreClose
			item.PreCloseDiff = math.Abs(item.ExpectedPreClose - item.BaoStockPreClose)
			item.FactorRatioDiff = math.Abs(item.FactorDelta - item.ImpliedRatio)
			item.ForwardBackDeltaDiff = math.Abs(item.ForwardFactorDelta - item.FactorDelta)
			if item.PreCloseDiff > 0.01 || item.FactorRatioDiff > 0.0005 || item.ForwardBackDeltaDiff > 0.0005 {
				item.Status = "mismatch"
				item.Error = "Pytdx event terms, BaoStock ex-date preclose, and BaoStock factor delta do not reconcile"
				failures = append(failures, symbol+"/"+eventDate+": "+item.Error)
			}
			report.Items = append(report.Items, item)
		}
		if matchedEvents == 0 {
			failures = append(failures, symbol+": no 2025 Pytdx GBBQ ex-rights/ex-dividend events were returned")
		}
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal Pytdx/BaoStock event report: %v", err)
	}
	t.Logf("LIVE_PYTDX_BAOSTOCK_EVENT_REPORT_BEGIN\n%s\nLIVE_PYTDX_BAOSTOCK_EVENT_REPORT_END", encoded)
	if outputPath := strings.TrimSpace(os.Getenv("LIVE_PYTDX_BAOSTOCK_EVENT_REPORT_PATH")); outputPath != "" {
		if writeErr := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); writeErr != nil {
			t.Errorf("write Pytdx/BaoStock event report: %v", writeErr)
		}
	}
	if len(failures) > 0 {
		t.Errorf("Pytdx/BaoStock corporate-action consistency failed: %s", strings.Join(failures, " | "))
	}
}
