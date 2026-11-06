package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type AdjustmentBasis string

const (
	AdjustmentBasisNone     AdjustmentBasis = "none"
	AdjustmentBasisForward  AdjustmentBasis = "forward_adjusted"
	AdjustmentBasisBackward AdjustmentBasis = "backward_adjusted"
	AdjustmentBasisUnknown  AdjustmentBasis = "unknown"
)

type QualityPolicy string

const (
	QualityPolicyStrict        QualityPolicy = "strict"
	QualityPolicyAllowDegraded QualityPolicy = "allow_degraded"
)

var (
	ErrMissingSystemAsOf      = errors.New("system_as_of is required")
	ErrSystemAsOfNotUTC       = errors.New("system_as_of must use the UTC location")
	ErrSystemAsOfInFuture     = errors.New("system_as_of must not be later than trusted current time")
	ErrInvalidTrustedNow      = errors.New("trusted current time must be a non-zero UTC timestamp")
	ErrInvalidTradeDate       = errors.New("trade_date must use YYYY-MM-DD")
	ErrMissingSnapshotRefs    = errors.New("at least one input_snapshot_ref is required")
	ErrInvalidSnapshotRef     = errors.New("input_snapshot_refs must contain non-empty opaque references")
	ErrDuplicateSnapshotRef   = errors.New("input_snapshot_refs must not contain duplicates")
	ErrInvalidAdjustmentBasis = errors.New("adjustment_basis is unsupported")
	ErrMissingAdjustmentBasis = errors.New("adjustment_basis is required for price-series calculations")
	ErrUnknownAdjustmentBasis = errors.New("price-series calculations require a known adjustment_basis")
	ErrInvalidQualityPolicy   = errors.New("quality_policy must be strict or allow_degraded")
)

// CalculationContext 仅承载一次确定性计算所需的公共时间、快照和质量边界。
// 具体行情字段、单位和复权参考日期由输入数据 DTO 自身携带，不在此处重复保存。
type CalculationContext struct {
	SystemAsOf        time.Time
	TradeDate         string
	InputSnapshotRefs []string
	AdjustmentBasis   AdjustmentBasis
	QualityPolicy     QualityPolicy
}

// Validate 校验可信截止时间和输入边界。now 必须由 Application/可信 Clock 注入，Domain 不读取系统时钟。
// requiresPrice=true 表示本次计算使用价格序列，必须明确且可比较复权口径。
func (c CalculationContext) Validate(now time.Time, requiresPrice bool) error {
	if c.SystemAsOf.IsZero() {
		return ErrMissingSystemAsOf
	}
	if !isUTC(c.SystemAsOf) {
		return ErrSystemAsOfNotUTC
	}
	if now.IsZero() || !isUTC(now) {
		return ErrInvalidTrustedNow
	}
	if c.SystemAsOf.After(now) {
		return ErrSystemAsOfInFuture
	}

	if c.TradeDate != "" {
		parsed, err := time.Parse("2006-01-02", c.TradeDate)
		if err != nil || parsed.Format("2006-01-02") != c.TradeDate {
			return ErrInvalidTradeDate
		}
	}

	if len(c.InputSnapshotRefs) == 0 {
		return ErrMissingSnapshotRefs
	}
	seen := make(map[string]struct{}, len(c.InputSnapshotRefs))
	for _, ref := range c.InputSnapshotRefs {
		if strings.TrimSpace(ref) == "" {
			return ErrInvalidSnapshotRef
		}
		if _, exists := seen[ref]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateSnapshotRef, ref)
		}
		seen[ref] = struct{}{}
	}

	if c.AdjustmentBasis != "" {
		switch c.AdjustmentBasis {
		case AdjustmentBasisNone, AdjustmentBasisForward, AdjustmentBasisBackward, AdjustmentBasisUnknown:
		default:
			return ErrInvalidAdjustmentBasis
		}
	}
	if requiresPrice {
		if c.AdjustmentBasis == "" {
			return ErrMissingAdjustmentBasis
		}
		if c.AdjustmentBasis == AdjustmentBasisUnknown {
			return ErrUnknownAdjustmentBasis
		}
	}

	switch c.QualityPolicy {
	case QualityPolicyStrict, QualityPolicyAllowDegraded:
	default:
		return ErrInvalidQualityPolicy
	}
	return nil
}

func isUTC(value time.Time) bool {
	return value.Location() == time.UTC
}
