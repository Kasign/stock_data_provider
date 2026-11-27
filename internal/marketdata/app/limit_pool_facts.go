package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

var ErrInvalidLimitPoolFacts = errors.New("invalid limit pool facts payload")

const (
	LimitPoolFactsPayloadKind          = "limit_pool"
	LimitPoolFactsPayloadSchemaVersion = "1.0.0"
)

// LimitPoolLimitStatus 是 price_limit 门禁事实的唯一取值。
// none 只有在涨停池与跌停池都完整时才可信；池不完整只能给出 unknown。
type LimitPoolLimitStatus string

const (
	LimitPoolStatusNone    LimitPoolLimitStatus = "none"
	LimitPoolStatusUp      LimitPoolLimitStatus = "limit_up"
	LimitPoolStatusDown    LimitPoolLimitStatus = "limit_down"
	LimitPoolStatusUnknown LimitPoolLimitStatus = "unknown"
)

// LimitPoolFactsPayload 是单证券当日涨跌停状态的授权快照载荷。
// 它只回答「是否触及涨跌停」，不推导涨跌幅比例，也不把「未在池中」解释为
// 除「当日未涨跌停」以外的任何结论。
type LimitPoolFactsPayload struct {
	SchemaVersion    string               `json:"schema_version"`
	PayloadKind      string               `json:"payload_kind"`
	Symbol           string               `json:"symbol"`
	TradeDate        string               `json:"trade_date"`
	ObservedAt       time.Time            `json:"observed_at"`
	SystemObservedAt time.Time            `json:"system_observed_at"`
	LimitStatus      LimitPoolLimitStatus `json:"limit_status"`
	UpPoolTotal      int                  `json:"up_pool_total"`
	DownPoolTotal    int                  `json:"down_pool_total"`
	UpPoolComplete   bool                 `json:"up_pool_complete"`
	DownPoolComplete bool                 `json:"down_pool_complete"`
	Source           string               `json:"source"`
	QualityStatus    QualityStatus        `json:"quality_status"`
}

// BuildLimitPoolFactsPayload 从两个全量池构造事实载荷。
// 完整性判定同时要求非下界与 Total == len(Items)：任何一方不成立都不得断言 none。
func BuildLimitPoolFactsPayload(
	up, down *model.LimitPoolSnapshot,
	symbol string,
) (LimitPoolFactsPayload, error) {
	if up == nil || down == nil {
		return LimitPoolFactsPayload{}, fmt.Errorf("%w: both limit pools are required", ErrInvalidLimitPoolFacts)
	}
	if up.PoolType != "up" || down.PoolType != "down" {
		return LimitPoolFactsPayload{}, fmt.Errorf("%w: pool types must be up/down", ErrInvalidLimitPoolFacts)
	}
	symbol = strings.TrimSpace(symbol)
	if !validLimitPoolSymbol(symbol) {
		return LimitPoolFactsPayload{}, fmt.Errorf("%w: symbol is invalid", ErrInvalidLimitPoolFacts)
	}
	if !validLimitPoolTradeDate(up.TradeDate) || up.TradeDate != down.TradeDate {
		return LimitPoolFactsPayload{}, fmt.Errorf("%w: trade dates are missing or inconsistent", ErrInvalidLimitPoolFacts)
	}
	if !up.HasData || !down.HasData {
		return LimitPoolFactsPayload{}, fmt.Errorf("%w: pool has no data", ErrInvalidLimitPoolFacts)
	}
	if up.Source == "" || down.Source == "" || up.Source != down.Source {
		return LimitPoolFactsPayload{}, fmt.Errorf("%w: pool sources are missing or inconsistent", ErrInvalidLimitPoolFacts)
	}
	if up.FetchedAt.IsZero() || down.FetchedAt.IsZero() {
		return LimitPoolFactsPayload{}, fmt.Errorf("%w: pool observation time is required", ErrInvalidLimitPoolFacts)
	}
	quality, err := limitPoolQuality(up.QualityStatus, down.QualityStatus)
	if err != nil {
		return LimitPoolFactsPayload{}, err
	}
	upComplete := limitPoolComplete(up)
	downComplete := limitPoolComplete(down)

	status := LimitPoolStatusUnknown
	switch {
	case limitPoolContainsSymbol(up, symbol):
		status = LimitPoolStatusUp
	case limitPoolContainsSymbol(down, symbol):
		status = LimitPoolStatusDown
	case upComplete && downComplete:
		status = LimitPoolStatusNone
	}

	observedAt := up.FetchedAt.UTC()
	if down.FetchedAt.After(observedAt) {
		observedAt = down.FetchedAt.UTC()
	}
	payload := LimitPoolFactsPayload{
		SchemaVersion: LimitPoolFactsPayloadSchemaVersion,
		PayloadKind:   LimitPoolFactsPayloadKind,
		Symbol:        symbol, TradeDate: up.TradeDate,
		ObservedAt: observedAt, SystemObservedAt: observedAt,
		LimitStatus:      status,
		UpPoolTotal:      up.Total,
		DownPoolTotal:    down.Total,
		UpPoolComplete:   upComplete,
		DownPoolComplete: downComplete,
		Source:           up.Source,
		QualityStatus:    quality,
	}
	if err := payload.Validate(); err != nil {
		return LimitPoolFactsPayload{}, err
	}
	return payload, nil
}

func limitPoolComplete(pool *model.LimitPoolSnapshot) bool {
	return !pool.TotalIsLowerBound && pool.Total >= 0 && pool.Total == len(pool.Items)
}

func limitPoolContainsSymbol(pool *model.LimitPoolSnapshot, symbol string) bool {
	for _, item := range pool.Items {
		if strings.TrimSpace(item.Symbol) == symbol {
			return true
		}
	}
	return false
}

func limitPoolQuality(up, down model.QualityStatus) (QualityStatus, error) {
	degraded := false
	for _, status := range []model.QualityStatus{up, down} {
		switch status {
		case model.QualityValid:
		case model.QualityDegraded:
			degraded = true
		default:
			return "", fmt.Errorf("%w: unusable pool quality %q", ErrInvalidLimitPoolFacts, status)
		}
	}
	if degraded {
		return QualityDegraded, nil
	}
	return QualityValid, nil
}

func validLimitPoolSymbol(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 32 && !strings.ContainsAny(value, " \t\n")
}

func validLimitPoolTradeDate(value string) bool {
	if value == "" || len(value) != len("2006-01-02") {
		return false
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return false
	}
	return parsed.Format("2006-01-02") == value
}

// Validate 校验身份、时间顺序与状态自洽性。
// none 是唯一可能让门禁放行的取值，因此必须由两个完整池共同支撑。
func (payload LimitPoolFactsPayload) Validate() error {
	if payload.SchemaVersion != LimitPoolFactsPayloadSchemaVersion ||
		payload.PayloadKind != LimitPoolFactsPayloadKind ||
		!validLimitPoolSymbol(payload.Symbol) ||
		!validLimitPoolTradeDate(payload.TradeDate) ||
		payload.Source == "" {
		return ErrInvalidLimitPoolFacts
	}
	if payload.UpPoolTotal < 0 || payload.DownPoolTotal < 0 {
		return ErrInvalidLimitPoolFacts
	}
	if payload.ObservedAt.IsZero() || payload.SystemObservedAt.IsZero() ||
		payload.ObservedAt.Location() != time.UTC || payload.SystemObservedAt.Location() != time.UTC ||
		payload.ObservedAt.After(payload.SystemObservedAt) {
		return ErrInvalidLimitPoolFacts
	}
	switch payload.LimitStatus {
	case LimitPoolStatusNone:
		if !payload.UpPoolComplete || !payload.DownPoolComplete {
			return ErrInvalidLimitPoolFacts
		}
	case LimitPoolStatusUp, LimitPoolStatusDown, LimitPoolStatusUnknown:
	default:
		return ErrInvalidLimitPoolFacts
	}
	if payload.QualityStatus != QualityValid && payload.QualityStatus != QualityDegraded {
		return ErrInvalidLimitPoolFacts
	}
	return nil
}
