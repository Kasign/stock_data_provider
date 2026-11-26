package application

import (
	"errors"
	"math/big"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/quant/registry"
)

// 本文件抽出 SMA20 与首批 Decimal Factor 共用的 Close Series Snapshot 门禁。
// 身份、来源绑定、PIT/质量证据与覆盖率口径属于 Data Snapshot 的公开语义，
// 不应在两个 Application 用例里各复制一份；各用例只保留自己的错误类型映射。
var (
	ErrInvalidCloseSeriesSnapshot = errors.New("invalid Quant Close Series input snapshot")
	ErrCloseSeriesSnapshotQuality = errors.New("Quant Close Series input snapshot does not satisfy the registered quality policy")
)

// closeSeriesValidationPolicy 是调用方用例对同一份 Snapshot 的差异化期望。
// purpose/source 由 Server 装配按 Data 授权与 Provider inventory 固定注入，不接受请求自报。
type closeSeriesValidationPolicy struct {
	purposeCode        string
	sourceRef          string
	providerAPIVersion string
	minimumCoverage    float64
}

// validateAuthorizedCloseSeriesSnapshot 校验 Data 已授权 Snapshot 的身份、Provider 绑定、
// PIT/日历/逐 bar 质量证据、覆盖率与可信时点边界。授权判定仍由 Data Owner Reader 完成，
// 本函数只校验返回投影与调用请求的绑定关系，不重新判定授权。
func validateAuthorizedCloseSeriesSnapshot(
	snapshot CloseSeriesSnapshot,
	requestSnapshotRef string,
	requestSystemAsOf time.Time,
	allowedProviderCodes []string,
	policy closeSeriesValidationPolicy,
	trustedNow time.Time,
) (time.Time, error) {
	if snapshot.SnapshotRef != requestSnapshotRef ||
		snapshot.ManifestSchemaVersion != "2.0.0" || snapshot.PayloadSchemaVersion != "1.0.0" ||
		snapshot.PayloadKind != "daily_bars_close_series" || strings.TrimSpace(snapshot.CallerRef) == "" ||
		snapshot.PurposeCode != policy.purposeCode ||
		!validCloseSeriesProviderSources(snapshot.Sources, allowedProviderCodes, policy.sourceRef, policy.providerAPIVersion) ||
		strings.TrimSpace(snapshot.Symbol) == "" ||
		!validCloseSeriesISODate(snapshot.StartDate) || !validCloseSeriesISODate(snapshot.EndDate) ||
		snapshot.StartDate > snapshot.EndDate ||
		snapshot.AdjustmentBasis != "none" || snapshot.Interval != "1d" ||
		strings.TrimSpace(snapshot.TradingCalendarRef) == "" || snapshot.QualityStatus != "valid" ||
		snapshot.Coverage.CalendarQualityStatus != "valid" ||
		snapshot.Coverage.ExpectedTradingDays <= 0 || snapshot.Coverage.MinimumRequiredBars <= 0 ||
		!snapshot.Coverage.CoverageSufficient ||
		len(snapshot.Bars) < snapshot.Coverage.MinimumRequiredBars ||
		len(snapshot.Bars) > snapshot.Coverage.ExpectedTradingDays {
		return time.Time{}, ErrInvalidCloseSeriesSnapshot
	}
	coverage := new(big.Rat).SetFrac(big.NewInt(int64(len(snapshot.Bars))), big.NewInt(int64(snapshot.Coverage.ExpectedTradingDays)))
	minimumCoverage := new(big.Rat).SetFloat64(policy.minimumCoverage)
	if minimumCoverage == nil || coverage.Cmp(minimumCoverage) < 0 {
		return time.Time{}, ErrCloseSeriesSnapshotQuality
	}
	var previousDate string
	for index, bar := range snapshot.Bars {
		if !validCloseSeriesISODate(bar.TradeDate) || bar.TradeDate < snapshot.StartDate ||
			bar.TradeDate > snapshot.EndDate || (index > 0 && bar.TradeDate <= previousDate) ||
			bar.QualityStatus != "valid" || strings.TrimSpace(bar.Close) == "" {
			return time.Time{}, ErrInvalidCloseSeriesSnapshot
		}
		previousDate = bar.TradeDate
	}
	if len(snapshot.Bars) == 0 {
		return time.Time{}, ErrInvalidCloseSeriesSnapshot
	}
	systemAsOf, err := parseCanonicalCloseSeriesUTC(snapshot.SystemAsOf)
	if err != nil || !systemAsOf.Equal(requestSystemAsOf) ||
		trustedNow.IsZero() || trustedNow.Location() != time.UTC || trustedNow.Before(systemAsOf) {
		return time.Time{}, ErrInvalidCloseSeriesSnapshot
	}
	return systemAsOf, nil
}

// closeSeriesValidationPolicyFromEntry 从活动 Registry Entry 读取覆盖率门禁与输入要求，
// 避免 Facade 自行硬编码质量策略；purpose/source 仍由用例装配固定注入。
func closeSeriesValidationPolicyFromEntry(entry registry.Entry, purposeCode, sourceRef, providerAPIVersion string) (closeSeriesValidationPolicy, error) {
	if len(entry.InputRequirements) != 1 {
		return closeSeriesValidationPolicy{}, ErrInvalidCloseSeriesSnapshot
	}
	requirement := entry.InputRequirements[0]
	if len(requirement.Fields) != 1 || requirement.Fields[0] != "close" ||
		requirement.Interval != "1d" || requirement.Adjustment != "none" ||
		!requirement.PIT || !requirement.RequiresSnapshot ||
		!containsString(requirement.Quality, "valid") ||
		!containsString(entry.QualityPolicy.QualityStates, "valid") ||
		!containsString(entry.QualityPolicy.UnavailableConditions, "insufficient_data") {
		return closeSeriesValidationPolicy{}, ErrInvalidCloseSeriesSnapshot
	}
	return closeSeriesValidationPolicy{
		purposeCode:        purposeCode,
		sourceRef:          sourceRef,
		providerAPIVersion: providerAPIVersion,
		minimumCoverage:    entry.QualityPolicy.MinimumCoverage,
	}, nil
}

func parseCanonicalCloseSeriesUTC(value string) (time.Time, error) {
	if !strings.HasSuffix(value, "Z") {
		return time.Time{}, ErrInvalidCloseSeriesSnapshot
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || parsed.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, ErrInvalidCloseSeriesSnapshot
	}
	return parsed, nil
}

func validCloseSeriesISODate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validCloseSeriesProviderSources(sources []CloseSeriesSource, allowedProviderCodes []string, sourceRef, providerAPIVersion string) bool {
	if len(sources) != 1 || !containsSortedUnique(allowedProviderCodes, sourceRef) {
		return false
	}
	source := sources[0]
	return source.SourceRef == sourceRef && source.SourceVersion == nil &&
		source.ProviderInterfaceVersion == providerAPIVersion
}
