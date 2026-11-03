package marketdata

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

// NormalizationStats makes dropped or duplicated provider rows visible to the
// caller instead of silently feeding inconsistent date order to indicators or
// to the LLM context.
type NormalizationStats struct {
	DroppedInvalid int
	DuplicateDates int
}

// NormalizeDailyBars validates OHLC/date integrity, keeps the last valid row
// for each trade date, and returns rows in ascending date order. Date values
// must be ISO-8601 calendar dates (YYYY-MM-DD); provider adapters should
// normalize their own source-specific formats before crossing the bridge.
func NormalizeDailyBars(input []domain.DailyBar) ([]domain.DailyBar, NormalizationStats) {
	byDate := make(map[string]domain.DailyBar, len(input))
	stats := NormalizationStats{}
	for _, row := range input {
		dateText := strings.TrimSpace(row.Date)
		parsed, err := time.Parse("2006-01-02", dateText)
		row.MissingFields = normalizeMissingFields(row.MissingFields)
		if err != nil || parsed.Format("2006-01-02") != dateText || !validOHLC(row) || !validOptionalMetric(row, "volume", row.Volume) || !validOptionalMetric(row, "amount", row.Amount) || !validOptionalMetric(row, "pct_chg", row.PctChg) {
			stats.DroppedInvalid++
			continue
		}
		row.Date = dateText
		if _, exists := byDate[dateText]; exists {
			stats.DuplicateDates++
		}
		// Match the legacy storage upsert rule: the last occurrence of a date
		// wins. This also makes duplicate handling deterministic for one payload.
		byDate[dateText] = row
	}
	out := make([]domain.DailyBar, 0, len(byDate))
	for _, row := range byDate {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, stats
}

func validOHLC(row domain.DailyBar) bool {
	values := []float64{row.Open, row.High, row.Low, row.Close}
	for _, value := range values {
		if !finite(value) || value <= 0 {
			return false
		}
	}
	return row.High >= row.Low && row.High >= row.Open && row.High >= row.Close && row.Low <= row.Open && row.Low <= row.Close
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func validOptionalMetric(row domain.DailyBar, field string, value float64) bool {
	if hasMissingField(row.MissingFields, field) {
		return true
	}
	if !finite(value) {
		return false
	}
	return field == "pct_chg" || value >= 0
}

func normalizeMissingFields(fields []string) []string {
	allowed := map[string]struct{}{
		"volume": {}, "amount": {}, "pct_chg": {},
	}
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(strings.ToLower(field))
		if _, ok := allowed[field]; !ok {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	sort.Strings(out)
	return out
}

func hasMissingField(fields []string, target string) bool {
	for _, field := range fields {
		if field == target {
			return true
		}
	}
	return false
}

// MarketDate converts an absolute timestamp to the local calendar date for the
// symbol's exchange. It is used to prevent provider responses from leaking bars
// later than the analysis cutoff. Unknown symbols default to China Standard Time.
func MarketDate(code string, at time.Time) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	zone := "Asia/Shanghai"
	switch {
	case strings.HasPrefix(code, "HK") || strings.HasSuffix(code, ".HK"):
		zone = "Asia/Hong_Kong"
	case strings.HasSuffix(code, ".TW"), strings.HasSuffix(code, ".TWO"):
		zone = "Asia/Taipei"
	case strings.HasSuffix(code, ".KS"), strings.HasSuffix(code, ".KQ"):
		zone = "Asia/Seoul"
	case strings.HasSuffix(code, ".T"):
		zone = "Asia/Tokyo"
	case strings.HasSuffix(code, ".SH"), strings.HasSuffix(code, ".SZ"), strings.HasSuffix(code, ".BJ"), strings.HasSuffix(code, ".CN"), strings.HasPrefix(code, "SH"), strings.HasPrefix(code, "SZ"), strings.HasPrefix(code, "BJ"), isNumericCode(code):
		zone = "Asia/Shanghai"
	case strings.HasSuffix(code, ".US") || containsASCIIAlpha(code):
		zone = "America/New_York"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return at.UTC().Format("2006-01-02")
	}
	return at.In(location).Format("2006-01-02")
}

// FilterBarsAfterDate returns an ascending copy with bars beyond the effective
// exchange-local date removed. The input should already be normalized.
func FilterBarsAfterDate(bars []domain.DailyBar, cutoffDate string) ([]domain.DailyBar, int) {
	if _, err := time.Parse("2006-01-02", cutoffDate); err != nil {
		return append([]domain.DailyBar(nil), bars...), 0
	}
	out := make([]domain.DailyBar, 0, len(bars))
	dropped := 0
	for _, bar := range bars {
		if bar.Date > cutoffDate {
			dropped++
			continue
		}
		out = append(out, bar)
	}
	return out, dropped
}

func containsASCIIAlpha(value string) bool {
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return true
		}
	}
	return false
}

func isNumericCode(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// MarkLatestCNIntradayBar marks today's last daily bar as partial only when the
// analysis is a live run, on a weekday and within the mainland A-share session
// clock. This is a clock-only heuristic; holidays and special sessions remain
// unverified until an exchange calendar capability is wired.
func MarkLatestCNIntradayBar(bars []domain.DailyBar, code string, asOf, now time.Time) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !(isNumericCode(code) || strings.HasSuffix(code, ".SH") || strings.HasSuffix(code, ".SZ") || strings.HasSuffix(code, ".BJ") || strings.HasSuffix(code, ".CN") || strings.HasPrefix(code, "SH") || strings.HasPrefix(code, "SZ") || strings.HasPrefix(code, "BJ")) {
		return false
	}
	delta := now.Sub(asOf)
	if delta < 0 {
		delta = -delta
	}
	if delta > 90*time.Second || len(bars) == 0 {
		return false
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return false
	}
	local := asOf.In(location)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	clock := local.Hour()*60 + local.Minute()
	if clock < 9*60+15 || clock >= 15*60 || bars[len(bars)-1].Date != local.Format("2006-01-02") {
		return false
	}
	bars[len(bars)-1].IsPartial = true
	return true
}
