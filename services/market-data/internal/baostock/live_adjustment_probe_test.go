package baostock

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
)

type liveBaoStockAdjustmentReport struct {
	GeneratedAt        string                       `json:"generated_at"`
	StartDate          string                       `json:"start_date"`
	EndDate            string                       `json:"end_date"`
	DateSemanticsNote  string                       `json:"date_semantics_note"`
	AvailabilityAtNote string                       `json:"available_at_note"`
	Items              []liveBaoStockAdjustmentItem `json:"items"`
}

type liveBaoStockAdjustmentItem struct {
	Symbol       string                   `json:"symbol"`
	FactorCount  int                      `json:"factor_count"`
	Factors      []AdjustmentFactor       `json:"factors,omitempty"`
	RawRows      int                      `json:"raw_rows"`
	ForwardRows  int                      `json:"forward_rows"`
	BackwardRows int                      `json:"backward_rows"`
	Mismatches   []liveAdjustmentMismatch `json:"mismatches,omitempty"`
	Error        string                   `json:"error,omitempty"`

	PreFirstEventStatus     string                   `json:"pre_first_event_status,omitempty"`
	PreFirstEventRows       int                      `json:"pre_first_event_rows,omitempty"`
	PreFirstEventFactorDate string                   `json:"pre_first_event_factor_date,omitempty"`
	PreFirstEventMismatches []liveAdjustmentMismatch `json:"pre_first_event_mismatches,omitempty"`
	PreFirstEventError      string                   `json:"pre_first_event_error,omitempty"`
}

type liveAdjustmentMismatch struct {
	TradeDate string  `json:"trade_date"`
	Basis     string  `json:"basis"`
	Field     string  `json:"field"`
	Expected  float64 `json:"expected"`
	Actual    float64 `json:"actual"`
	Diff      float64 `json:"diff"`
}

func TestLiveBaoStockAdjustmentFactorConsistency(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_BAOSTOCK_ADJUSTMENT_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_BAOSTOCK_ADJUSTMENT_TESTS=1 to query live BaoStock adjustment factors")
	}

	const startDate, endDate = "2025-07-07", "2025-07-17"
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	factorEndDate := time.Now().In(location).Format("2006-01-02")
	client := NewClient(Config{ConnectTimeout: 8 * time.Second, RequestTimeout: 20 * time.Second})
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	report := liveBaoStockAdjustmentReport{
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		StartDate:          startDate,
		EndDate:            endDate,
		DateSemanticsNote:  "OperateDate is tested as a candidate effective date by matching raw, forward-adjusted and backward-adjusted K-lines; a successful price match is evidence for reconstruction semantics, not proof of the upstream field's legal/business definition.",
		AvailabilityAtNote: "BaoStock query_adjust_factor does not expose a verified first-publication timestamp in this response; AvailableAt must remain unknown.",
		Items:              make([]liveBaoStockAdjustmentItem, 0, 2),
	}
	var failures []string
	for _, symbol := range []string{"sh.600036", "sz.002982"} {
		item := liveBaoStockAdjustmentItem{Symbol: symbol}
		factors, factorErr := client.QueryAdjustmentFactors(ctx, symbol, "2000-01-01", factorEndDate)
		if factorErr != nil {
			item.Error = "query adjustment factors: " + factorErr.Error()
			failures = append(failures, symbol+": "+item.Error)
			report.Items = append(report.Items, item)
			continue
		}
		item.Factors = factors
		item.FactorCount = len(factors)
		if len(factors) == 0 {
			item.Error = "BaoStock returned no adjustment-factor rows for the security history"
			failures = append(failures, symbol+": "+item.Error)
			report.Items = append(report.Items, item)
			continue
		}

		raw, rawErr := client.QueryDailyKLines(ctx, symbol, startDate, endDate, "3")
		forward, forwardErr := client.QueryDailyKLines(ctx, symbol, startDate, endDate, "2")
		backward, backwardErr := client.QueryDailyKLines(ctx, symbol, startDate, endDate, "1")
		if rawErr != nil || forwardErr != nil || backwardErr != nil {
			item.Error = strings.TrimSpace(strings.Join([]string{
				formatLiveBaoStockError("none", rawErr),
				formatLiveBaoStockError("qfq", forwardErr),
				formatLiveBaoStockError("hfq", backwardErr),
			}, "; "))
			failures = append(failures, symbol+": "+item.Error)
			report.Items = append(report.Items, item)
			continue
		}
		item.RawRows, item.ForwardRows, item.BackwardRows = len(raw), len(forward), len(backward)
		if len(raw) != len(forward) || len(raw) != len(backward) {
			item.Error = "raw/qfq/hfq row counts differ"
			failures = append(failures, symbol+": "+item.Error)
			report.Items = append(report.Items, item)
			continue
		}
		for index, rawBar := range raw {
			if forward[index].Date != rawBar.Date || backward[index].Date != rawBar.Date {
				item.Error = "raw/qfq/hfq trade-date sequences differ"
				break
			}
			factor, ok := latestLiveAdjustmentFactor(factors, rawBar.Date)
			if !ok {
				item.Error = "no effective adjustment factor for " + rawBar.Date
				break
			}
			for _, field := range []string{"open", "high", "low", "close"} {
				rawValue, parseErr := parseLiveBaoStockPrice(rawBar, field)
				if parseErr != nil {
					item.Error = parseErr.Error()
					break
				}
				forwardValue, parseErr := parseLiveBaoStockPrice(forward[index], field)
				if parseErr != nil {
					item.Error = parseErr.Error()
					break
				}
				backwardValue, parseErr := parseLiveBaoStockPrice(backward[index], field)
				if parseErr != nil {
					item.Error = parseErr.Error()
					break
				}
				compareLiveAdjustment(&item, rawBar.Date, "qfq", field, rawValue*factor.ForwardFactor, forwardValue)
				compareLiveAdjustment(&item, rawBar.Date, "hfq", field, rawValue*factor.BackwardFactor, backwardValue)
			}
			if item.Error != "" {
				break
			}
		}
		if firstFactor, latestFactor, ok := liveAdjustmentFactorBounds(factors); ok {
			preEnd, parseErr := time.Parse("2006-01-02", firstFactor.OperateDate)
			if parseErr != nil {
				item.PreFirstEventStatus = "unavailable"
				item.PreFirstEventError = "invalid first factor event date: " + parseErr.Error()
			} else {
				preEnd = preEnd.AddDate(0, 0, -1)
				if preEnd.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
					item.PreFirstEventStatus = "not_applicable"
					item.PreFirstEventError = "first factor event is not after the probe history start date"
				} else {
					preEndDate := preEnd.Format("2006-01-02")
					preRaw, preRawErr := client.QueryDailyKLines(ctx, symbol, "2000-01-01", preEndDate, "3")
					preForward, preForwardErr := client.QueryDailyKLines(ctx, symbol, "2000-01-01", preEndDate, "2")
					if preRawErr != nil || preForwardErr != nil {
						item.PreFirstEventStatus = "unavailable"
						item.PreFirstEventError = strings.TrimSpace(strings.Join([]string{
							formatLiveBaoStockError("pre-first-event raw", preRawErr),
							formatLiveBaoStockError("pre-first-event qfq", preForwardErr),
						}, "; "))
					} else if len(preRaw) == 0 {
						item.PreFirstEventStatus = "not_applicable"
						item.PreFirstEventError = "no raw trading bars exist before the first factor event"
					} else if len(preRaw) != len(preForward) {
						item.PreFirstEventStatus = "mismatch"
						item.PreFirstEventError = "pre-first-event raw/qfq row counts differ"
					} else {
						probeItem := liveBaoStockAdjustmentItem{Symbol: symbol}
						start := len(preRaw) - 5
						if start < 0 {
							start = 0
						}
					item.PreFirstEventRows = len(preRaw) - start
					item.PreFirstEventFactorDate = latestFactor.OperateDate
					for index := start; index < len(preRaw); index++ {
						if preRaw[index].Date != preForward[index].Date || preRaw[index].Date >= firstFactor.OperateDate {
							item.PreFirstEventStatus = "mismatch"
							item.PreFirstEventError = "pre-first-event raw/qfq dates are inconsistent with the first factor event"
							break
						}
						for _, field := range []string{"open", "high", "low", "close"} {
							rawValue, rawValueErr := parseLiveBaoStockPrice(preRaw[index], field)
							forwardValue, forwardValueErr := parseLiveBaoStockPrice(preForward[index], field)
							if rawValueErr != nil || forwardValueErr != nil {
								item.PreFirstEventStatus = "unavailable"
								item.PreFirstEventError = strings.TrimSpace(strings.Join([]string{
									formatLiveBaoStockError("pre-first-event raw price", rawValueErr),
									formatLiveBaoStockError("pre-first-event qfq price", forwardValueErr),
								}, "; "))
								break
							}
							compareLiveAdjustment(&probeItem, preRaw[index].Date, "qfq_pre_first_event_latest_factor_fallback", field, rawValue*latestFactor.ForwardFactor, forwardValue)
						}
						if item.PreFirstEventError != "" {
							break
						}
					}
					item.PreFirstEventMismatches = probeItem.Mismatches
					if item.PreFirstEventStatus == "" {
						if len(item.PreFirstEventMismatches) > 0 {
							item.PreFirstEventStatus = "mismatch"
						} else {
							item.PreFirstEventStatus = "matched"
						}
					}
				}
			}
		} else {
			item.PreFirstEventStatus = "unavailable"
			item.PreFirstEventError = "no valid first/latest adjustment-factor event dates"
		}
		if item.Error != "" {
			failures = append(failures, symbol+": "+item.Error)
		}
		if len(item.Mismatches) > 0 {
			failures = append(failures, symbol+": BaoStock adjusted K-lines do not match its published factor series")
		}
		report.Items = append(report.Items, item)
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal BaoStock adjustment report: %v", err)
	}
	t.Logf("LIVE_BAOSTOCK_ADJUSTMENT_REPORT_BEGIN\n%s\nLIVE_BAOSTOCK_ADJUSTMENT_REPORT_END", encoded)
	if outputPath := strings.TrimSpace(os.Getenv("LIVE_BAOSTOCK_ADJUSTMENT_REPORT_PATH")); outputPath != "" {
		if writeErr := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); writeErr != nil {
			t.Errorf("write BaoStock adjustment report: %v", writeErr)
		}
	}
	if len(failures) > 0 {
		t.Errorf("BaoStock adjustment verification failed: %s", strings.Join(failures, " | "))
	}
}

func TestLiveAdjustmentFactorBoundsFindsEarliestAndLatestEvent(t *testing.T) {
	factors := []AdjustmentFactor{
		{OperateDate: "2025-06-01", ForwardFactor: 0.9},
		{OperateDate: "2024-06-01", ForwardFactor: 0.8},
		{OperateDate: "2026-06-01", ForwardFactor: 1},
	}
	first, latest, ok := liveAdjustmentFactorBounds(factors)
	if !ok || first.OperateDate != "2024-06-01" || latest.OperateDate != "2026-06-01" {
		t.Fatalf("liveAdjustmentFactorBounds() = (%+v, %+v, %t)", first, latest, ok)
	}
}

func TestLatestLiveAdjustmentFactorUsesLatestEffectiveEvent(t *testing.T) {
	factors := []AdjustmentFactor{
		{OperateDate: "2025-06-01", ForwardFactor: 0.9, BackwardFactor: 1.4},
		{OperateDate: "2024-06-01", ForwardFactor: 0.8, BackwardFactor: 1.25},
		{OperateDate: "2026-06-01", ForwardFactor: 1, BackwardFactor: 1.55},
	}
	tests := []struct {
		name      string
		tradeDate string
		wantDate  string
		wantFound bool
	}{
		{name: "before first event", tradeDate: "2024-05-31", wantFound: false},
		{name: "event date is effective", tradeDate: "2025-06-01", wantDate: "2025-06-01", wantFound: true},
		{name: "between events uses prior event", tradeDate: "2025-12-31", wantDate: "2025-06-01", wantFound: true},
		{name: "after latest event", tradeDate: "2026-07-01", wantDate: "2026-06-01", wantFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := latestLiveAdjustmentFactor(factors, tt.tradeDate)
			if found != tt.wantFound || (found && got.OperateDate != tt.wantDate) {
				t.Fatalf("latestLiveAdjustmentFactor(%q) = (%+v, %t), want date=%q found=%t", tt.tradeDate, got, found, tt.wantDate, tt.wantFound)
			}
		})
	}
}

func liveAdjustmentFactorBounds(factors []AdjustmentFactor) (AdjustmentFactor, AdjustmentFactor, bool) {
	var first, latest AdjustmentFactor
	for _, factor := range factors {
		if _, err := time.Parse("2006-01-02", factor.OperateDate); err != nil {
			continue
		}
		if first.OperateDate == "" || factor.OperateDate < first.OperateDate {
			first = factor
		}
		if latest.OperateDate == "" || factor.OperateDate > latest.OperateDate {
			latest = factor
		}
	}
	return first, latest, first.OperateDate != "" && latest.OperateDate != ""
}

func latestLiveAdjustmentFactor(factors []AdjustmentFactor, tradeDate string) (AdjustmentFactor, bool) {
	var selected AdjustmentFactor
	found := false
	for _, factor := range factors {
		if factor.OperateDate <= tradeDate && (!found || factor.OperateDate > selected.OperateDate) {
			selected = factor
			found = true
		}
	}
	return selected, found
}

func parseLiveBaoStockPrice(bar DailyKLine, field string) (float64, error) {
	value := ""
	switch field {
	case "open":
		value = bar.Open
	case "high":
		value = bar.High
	case "low":
		value = bar.Low
	case "close":
		value = bar.Close
	default:
		return 0, fmt.Errorf("unsupported BaoStock price field %q", field)
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid BaoStock %s price %q on %s", field, value, bar.Date)
	}
	return parsed, nil
}

func compareLiveAdjustment(item *liveBaoStockAdjustmentItem, tradeDate, basis, field string, expected, actual float64) {
	diff := math.Abs(expected - actual)
	if diff > 0.0002 {
		item.Mismatches = append(item.Mismatches, liveAdjustmentMismatch{
			TradeDate: tradeDate, Basis: basis, Field: field,
			Expected: expected, Actual: actual, Diff: diff,
		})
	}
}

func formatLiveBaoStockError(name string, err error) string {
	if err == nil {
		return ""
	}
	return name + ": " + err.Error()
}
