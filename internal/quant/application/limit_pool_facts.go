package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Quant 消费的涨跌停事实边界。
//
// price_limit 只回答「该证券当日是否触及涨跌停」，不推导涨跌幅比例。
// Quant 不读 Provider、不接受调用方声称的池内容；只读 Data 已授权、已持久化
// 且通过双指纹复核的 limit_pool 快照。
const (
	LimitPoolFactsPayloadKind          = "limit_pool"
	LimitPoolFactsPayloadSchemaVersion = "1.0.0"
)

const (
	LimitPoolStatusNone    = "none"
	LimitPoolStatusUp      = "limit_up"
	LimitPoolStatusDown    = "limit_down"
	LimitPoolStatusUnknown = "unknown"
)

// limitPoolFactKeyPriceLimit 是 BlockingFacts 中使用的稳定事实键，
// 与门禁事实 risk.TradabilityFactPriceLimit 同名，便于审计对齐。
const limitPoolFactKeyPriceLimit = "price_limit"

var ErrInvalidLimitPoolFactsSnapshot = errors.New("invalid limit-pool facts snapshot")

// LimitPoolFactsSnapshot 是 Quant 消费的可追溯涨跌停事实证据。
type LimitPoolFactsSnapshot struct {
	SnapshotRef           string
	AuthorizationRef      string
	ManifestSchemaVersion string
	PayloadSchemaVersion  string
	PayloadKind           string
	CallerRef             string
	PurposeCode           string
	Symbol                string
	TradeDate             string
	ObservedAt            string
	SystemAsOf            string
	LimitStatus           string
	UpPoolTotal           int
	DownPoolTotal         int
	UpPoolComplete        bool
	DownPoolComplete      bool
	Source                string
	QualityStatus         string
}

// Validate 校验身份、状态枚举、完整性自洽与 PIT 时间。
// none 是唯一能让 price_limit 可核验的取值，必须由两个完整池支撑。
func (snapshot LimitPoolFactsSnapshot) Validate() error {
	if !nonBlankTradingFacts(snapshot.SnapshotRef) || !nonBlankTradingFacts(snapshot.AuthorizationRef) ||
		!nonBlankTradingFacts(snapshot.Symbol) || !nonBlankTradingFacts(snapshot.CallerRef) ||
		!nonBlankTradingFacts(snapshot.SystemAsOf) || !nonBlankTradingFacts(snapshot.ObservedAt) ||
		!nonBlankTradingFacts(snapshot.Source) || !nonBlankTradingFacts(snapshot.ManifestSchemaVersion) ||
		!nonBlankTradingFacts(snapshot.TradeDate) {
		return fmt.Errorf("%w: identity or evidence references are missing", ErrInvalidLimitPoolFactsSnapshot)
	}
	if snapshot.PayloadKind != LimitPoolFactsPayloadKind ||
		snapshot.PayloadSchemaVersion != LimitPoolFactsPayloadSchemaVersion {
		return fmt.Errorf("%w: payload kind or schema version mismatch", ErrInvalidLimitPoolFactsSnapshot)
	}
	if snapshot.UpPoolTotal < 0 || snapshot.DownPoolTotal < 0 {
		return fmt.Errorf("%w: pool totals must not be negative", ErrInvalidLimitPoolFactsSnapshot)
	}
	switch snapshot.LimitStatus {
	case LimitPoolStatusNone:
		if !snapshot.UpPoolComplete || !snapshot.DownPoolComplete {
			return fmt.Errorf("%w: none requires both pools to be complete", ErrInvalidLimitPoolFactsSnapshot)
		}
	case LimitPoolStatusUp, LimitPoolStatusDown, LimitPoolStatusUnknown:
	default:
		return fmt.Errorf("%w: limit_status %q is not admissible", ErrInvalidLimitPoolFactsSnapshot, snapshot.LimitStatus)
	}
	if snapshot.QualityStatus != "valid" && snapshot.QualityStatus != "degraded" {
		return fmt.Errorf("%w: quality_status %q is not admissible", ErrInvalidLimitPoolFactsSnapshot, snapshot.QualityStatus)
	}
	tradeDate, err := time.Parse("2006-01-02", snapshot.TradeDate)
	if err != nil || tradeDate.Format("2006-01-02") != snapshot.TradeDate {
		return fmt.Errorf("%w: trade_date is not canonical", ErrInvalidLimitPoolFactsSnapshot)
	}
	observedAt, err := time.Parse(time.RFC3339Nano, snapshot.ObservedAt)
	if err != nil || observedAt.Location() != time.UTC {
		return fmt.Errorf("%w: observed_at is not a canonical UTC time", ErrInvalidLimitPoolFactsSnapshot)
	}
	systemAsOf, err := time.Parse(time.RFC3339Nano, snapshot.SystemAsOf)
	if err != nil || systemAsOf.Location() != time.UTC {
		return fmt.Errorf("%w: system_as_of is not a canonical UTC time", ErrInvalidLimitPoolFactsSnapshot)
	}
	if observedAt.After(systemAsOf) {
		return fmt.Errorf("%w: observed_at is later than system_as_of", ErrInvalidLimitPoolFactsSnapshot)
	}
	return nil
}

// LimitPoolFactsReader 读取涨跌停事实授权快照。生产实现由 Server 组合根注入。
type LimitPoolFactsReader interface {
	ReadLimitPoolFactsSnapshot(ctx context.Context, snapshotRef, authorizationRef, purpose string) (LimitPoolFactsSnapshot, error)
}
