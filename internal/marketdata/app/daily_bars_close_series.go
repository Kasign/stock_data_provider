package app

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DailyBarsCloseSeriesPayloadSchemaVersion = "1.0.0"
	DailyBarsCloseSeriesPayloadKind           = "daily_bars_close_series"
	dailyBarsCloseSeriesSourceEastmoney       = "eastmoney"
	dailyBarsCloseSeriesEastmoneyAPIVersion   = "eastmoney-stock-api-v1"
)

var (
	dailyBarsCloseSeriesDecimalRE = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\\.[0-9]+)?package app

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DailyBarsCloseSeriesPayloadSchemaVersion = "1.0.0"
	DailyBarsCloseSeriesPayloadKind           = "daily_bars_close_series"
	dailyBarsCloseSeriesSourceEastmoney       = "eastmoney"
	dailyBarsCloseSeriesEastmoneyAPIVersion   = "eastmoney-stock-api-v1"
)

)
	dailyBarsCloseSeriesSourceTokenRE = regexp.MustCompile(`^-?[0-9]+(?:\\.[0-9]+)?package app

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DailyBarsCloseSeriesPayloadSchemaVersion = "1.0.0"
	DailyBarsCloseSeriesPayloadKind           = "daily_bars_close_series"
	dailyBarsCloseSeriesSourceEastmoney       = "eastmoney"
	dailyBarsCloseSeriesEastmoneyAPIVersion   = "eastmoney-stock-api-v1"
)

)
)

// DailyBarsCloseSeriesPayload 是首批 SMA 使用的窄化、可内容寻址输入。
// 只接收未复权日线的精确 Close，不从 float64 反向生成 Decimal。
type DailyBarsCloseSeriesPayload struct {
	SchemaVersion    string                       `json:"schema_version"`
	PayloadKind      string                       `json:"payload_kind"`
	Symbol           string                       `json:"symbol"`
	Interval         string                       `json:"interval"`
	StartDate        string                       `json:"start_date"`
	EndDate          string                       `json:"end_date"`
	AdjustmentBasis  AdjustmentBasis              `json:"adjustment_basis"`
	TradingCalendarRef string                     `json:"trading_calendar_ref"`
	QualityStatus    QualityStatus                `json:"quality_status"`
	Warnings         []string                     `json:"warnings"`
	CoverageEvidence DailyBarCoverageEvidence     `json:"coverage_evidence"`
	Bars             []DailyBarsCloseSeriesBar    `json:"bars"`
}

// DailyBarsCloseSeriesBar 保留计算所需的精确收盘价及逐条来源/时点证据。
type DailyBarsCloseSeriesBar struct {
	TradeDate           string                  `json:"trade_date"`
	Close               string                  `json:"close"`
	CloseEvidence       DailyBarDecimalEvidence `json:"close_evidence"`
	SourceRef           string                  `json:"source_ref"`
	NormalizationVersion string                 `json:"normalization_version"`
	FetchedAt           string                  `json:"fetched_at"`
	NormalizedAt        string                  `json:"normalized_at"`
	ProviderTime        *string                 `json:"provider_time"`
	SystemObservedAt    *string                 `json:"system_observed_at"`
	QualityStatus       QualityStatus           `json:"quality_status"`
	QualityReasons      []string                `json:"quality_reasons"`
}

// DailyBarsCloseSeriesSource 是 Manifest 中的来源条目。
// SourceVersion 只表示 Provider 数据版本；当前 Eastmoney 不提供逐条数据版本，因此保持 nil。
type DailyBarsCloseSeriesSource struct {
	SourceRef               string  `json:"source_ref"`
	SourceVersion           *string `json:"source_version"`
	ProviderInterfaceVersion string `json:"provider_interface_version,omitempty"`
}

// DailyBarsCloseSeriesInput 是后续 Manifest Builder 的唯一输入，SystemAsOf 来自可信 PIT 截止时间。
type DailyBarsCloseSeriesInput struct {
	Payload   DailyBarsCloseSeriesPayload
	Sources   []DailyBarsCloseSeriesSource
	SystemAsOf time.Time
}

// BuildDailyBarsCloseSeriesInput 将 Data V2 Reader 的同次读取结果转换为 DATA-CTR-007 首批 Payload。
// 该首批实现刻意只接受 Eastmoney 未复权日线，避免把不同 Provider 的 SourceVersion 误标为同一种接口版本。
func BuildDailyBarsCloseSeriesInput(query DailyBarsQuery, snapshot DailyBarsV2Snapshot) (DailyBarsCloseSeriesInput, error) {
	if strings.TrimSpace(query.Symbol) == "" || query.Symbol != strings.TrimSpace(query.Symbol) {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars close snapshot symbol is required and must be canonical")
	}
	if !validCloseSeriesDate(query.StartDate) || !validCloseSeriesDate(query.EndDate) || query.StartDate > query.EndDate {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars close snapshot requires a valid explicit date range")
	}
	if query.Limit <= 0 {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars close snapshot requires an explicit positive limit")
	}
	if query.AdjustmentBasis != AdjustmentNone {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars close snapshot only supports unadjusted bars")
	}
	if query.SystemObservedAtCutoff == nil || query.SystemObservedAtCutoff.IsZero() {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars close snapshot requires an explicit PIT cutoff")
	}
	cutoff := query.SystemObservedAtCutoff.UTC()
	if snapshot.SystemObservedAtCutoff == nil || !snapshot.SystemObservedAtCutoff.Equal(cutoff) {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars snapshot cutoff does not match the query cutoff")
	}
	if snapshot.Symbol != query.Symbol || snapshot.Interval != "1d" || snapshot.AdjustmentBasis != AdjustmentNone {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars snapshot identity does not match the requested close series")
	}
	if snapshot.TradingCalendarRef == nil || strings.TrimSpace(*snapshot.TradingCalendarRef) == "" {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars close snapshot requires the same-evaluation trading calendar reference")
	}
	coverage := snapshot.CoverageEvidence
	if coverage.CalendarQualityStatus != QualityValid || coverage.CoverageBasis != "exchange_calendar" ||
		!coverage.CoverageSufficient || coverage.ExpectedTradingDays <= 0 || coverage.MinimumRequiredBars <= 0 {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars close snapshot requires valid and sufficient exchange-calendar coverage evidence")
	}
	if len(snapshot.Bars) < coverage.MinimumRequiredBars {
		return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot has %d bars; coverage requires at least %d", len(snapshot.Bars), coverage.MinimumRequiredBars)
	}

	payloadWarnings, err := canonicalDailyBarsCloseSeriesStringSet(snapshot.Warnings)
	if err != nil {
		return DailyBarsCloseSeriesInput{}, fmt.Errorf("snapshot warnings: %w", err)
	}
	coverageWarnings, err := canonicalDailyBarsCloseSeriesStringSet(coverage.Warnings)
	if err != nil {
		return DailyBarsCloseSeriesInput{}, fmt.Errorf("coverage warnings: %w", err)
	}
	if !validDailyBarsCloseSeriesQualityStatus(snapshot.QualityStatus) {
		return DailyBarsCloseSeriesInput{}, errors.New("daily-bars snapshot quality status is invalid")
	}
	payload := DailyBarsCloseSeriesPayload{
		SchemaVersion: DailyBarsCloseSeriesPayloadSchemaVersion,
		PayloadKind: DailyBarsCloseSeriesPayloadKind,
		Symbol: snapshot.Symbol,
		Interval: snapshot.Interval,
		StartDate: query.StartDate,
		EndDate: query.EndDate,
		AdjustmentBasis: AdjustmentNone,
		TradingCalendarRef: strings.TrimSpace(*snapshot.TradingCalendarRef),
		QualityStatus: snapshot.QualityStatus,
		Warnings: payloadWarnings,
		CoverageEvidence: DailyBarCoverageEvidence{
			CalendarQualityStatus: coverage.CalendarQualityStatus,
			ExpectedTradingDays: coverage.ExpectedTradingDays,
			MinimumRequiredBars: coverage.MinimumRequiredBars,
			CoverageBasis: coverage.CoverageBasis,
			CoverageSufficient: coverage.CoverageSufficient,
			Warnings: coverageWarnings,
		},
		Bars: make([]DailyBarsCloseSeriesBar, 0, len(snapshot.Bars)),
	}
	var source DailyBarsCloseSeriesSource
	previousDate := ""
	for index, bar := range snapshot.Bars {
		if !validCloseSeriesDate(bar.TradeDate) || bar.TradeDate < query.StartDate || bar.TradeDate > query.EndDate {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d has an invalid/out-of-range trade date", index)
		}
		if previousDate != "" && bar.TradeDate <= previousDate {
			return DailyBarsCloseSeriesInput{}, errors.New("daily-bars close snapshot bars must be sorted by unique ascending trade date")
		}
		previousDate = bar.TradeDate
		if bar.Symbol != snapshot.Symbol || bar.Period != "1d" || bar.AdjustmentBasis != AdjustmentNone {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d identity or adjustment basis mismatch", index)
		}
		if bar.Source != dailyBarsCloseSeriesSourceEastmoney || bar.SourceVersion != dailyBarsCloseSeriesEastmoneyAPIVersion {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot source %q/version %q is not an explicitly supported source interface", bar.Source, bar.SourceVersion)
		}
		if !validDailyBarsCloseSeriesQualityStatus(bar.QualityStatus) {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d has an invalid quality status", index)
		}
		if strings.TrimSpace(bar.NormalizationVersion) == "" {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d has no normalization version", index)
		}
		if bar.FetchedAt.IsZero() || bar.NormalizedAt.IsZero() || bar.SystemObservedAt == nil {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d is missing PIT observation metadata", index)
		}
		if bar.FetchedAt.After(cutoff) || bar.NormalizedAt.After(cutoff) || bar.SystemObservedAt.After(cutoff) {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d is newer than the PIT cutoff", index)
		}
		if bar.ExactValues == nil || strings.TrimSpace(bar.ExactValues.Close) == "" || bar.ExactValues.CloseEvidence == nil {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d has no exact Close value/evidence", index)
		}
		evidence, err := validateEastmoneyCloseEvidence(bar.ExactValues.Close, bar.ExactValues.CloseEvidence)
		if err != nil {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d: %w", index, err)
		}
		qualityReasons, reasonErr := canonicalDailyBarsCloseSeriesStringSet(bar.QualityReasons)
		if reasonErr != nil {
			return DailyBarsCloseSeriesInput{}, fmt.Errorf("daily-bars close snapshot bar %d quality reasons: %w", index, reasonErr)
		}
		payload.Bars = append(payload.Bars, DailyBarsCloseSeriesBar{
			TradeDate: bar.TradeDate,
			Close: bar.ExactValues.Close,
			CloseEvidence: evidence,
			SourceRef: bar.Source,
			NormalizationVersion: bar.NormalizationVersion,
			FetchedAt: canonicalCloseSeriesTime(bar.FetchedAt),
			NormalizedAt: canonicalCloseSeriesTime(bar.NormalizedAt),
			ProviderTime: optionalCloseSeriesTime(bar.ProviderTime),
			SystemObservedAt: optionalCloseSeriesTime(bar.SystemObservedAt),
			QualityStatus: bar.QualityStatus,
			QualityReasons: qualityReasons,
		})
	}
	source = DailyBarsCloseSeriesSource{
		SourceRef: dailyBarsCloseSeriesSourceEastmoney,
		SourceVersion: nil,
		ProviderInterfaceVersion: dailyBarsCloseSeriesEastmoneyAPIVersion,
	}
	return DailyBarsCloseSeriesInput{
		Payload: payload,
		Sources: []DailyBarsCloseSeriesSource{source},
		SystemAsOf: cutoff,
	}, nil
}

func validateEastmoneyCloseEvidence(closeValue string, evidence *DailyBarDecimalEvidence) (DailyBarDecimalEvidence, error) {
	if !dailyBarsCloseSeriesDecimalRE.MatchString(closeValue) || !dailyBarsCloseSeriesSourceTokenRE.MatchString(evidence.SourceToken) {
		return DailyBarDecimalEvidence{}, errors.New("exact Close and observed source token must be decimal strings")
	}
	if evidence.EvidenceKind != "observed_token" || evidence.SourceFractionDigits == nil || *evidence.SourceFractionDigits < 0 {
		return DailyBarDecimalEvidence{}, errors.New("Eastmoney Close requires observed-token precision evidence")
	}
	fractionDigits := 0
	if dot := strings.IndexByte(evidence.SourceToken, '.'); dot >= 0 {
		fractionDigits = len(evidence.SourceToken) - dot - 1
	}
	if fractionDigits != *evidence.SourceFractionDigits {
		return DailyBarDecimalEvidence{}, errors.New("Close source fraction digits do not match the observed token")
	}
	closeRat, ok := new(big.Rat).SetString(closeValue)
	if !ok || closeRat.Sign() <= 0 {
		return DailyBarDecimalEvidence{}, errors.New("exact Close must be a positive decimal")
	}
	sourceRat, ok := new(big.Rat).SetString(evidence.SourceToken)
	if !ok || sourceRat.Cmp(closeRat) != 0 {
		return DailyBarDecimalEvidence{}, errors.New("exact Close does not equal its original source token")
	}
	if evidence.ProtocolScale != nil {
		return DailyBarDecimalEvidence{}, errors.New("observed-token Close must not also claim protocol fixed scale")
	}
	if strings.TrimSpace(evidence.UnitConversionFactor) != "" && evidence.UnitConversionFactor != "1" {
		return DailyBarDecimalEvidence{}, errors.New("Eastmoney price Close must not apply a unit conversion factor")
	}
	return DailyBarDecimalEvidence{
		EvidenceKind: evidence.EvidenceKind,
		SourceToken: evidence.SourceToken,
		SourceFractionDigits: cloneDailyBarsCloseSeriesInt(evidence.SourceFractionDigits),
		ProtocolScale: nil,
		UnitConversionFactor: evidence.UnitConversionFactor,
	}, nil
}

func validCloseSeriesDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func canonicalCloseSeriesTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func optionalCloseSeriesTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := canonicalCloseSeriesTime(*value)
	return &formatted
}

func canonicalDailyBarsCloseSeriesStringSet(values []string) ([]string, error) {
	result := append([]string(nil), values...)
	for _, value := range result {
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("string set contains an empty value")
		}
	}
	sort.Strings(result)
	unique := result[:0]
	for _, value := range result {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	if unique == nil {
		return []string{}, nil
	}
	return unique, nil
}

func validDailyBarsCloseSeriesQualityStatus(value QualityStatus) bool {
	switch value {
	case QualityValid, QualityDegraded, QualityStale, QualityInvalid, QualityUnknown:
		return true
	default:
		return false
	}
}

func cloneDailyBarsCloseSeriesInt(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
