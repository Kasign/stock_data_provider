// Package app 定义 Market Data 对其他模块公开的 Application Port。
// 该包不暴露 Provider、Repository、Cache 或 HTTP 传输类型。
package app

import (
	"context"
	"time"
)

// AdjustmentBasis 表示返回行情或日线数据的复权口径。
type AdjustmentBasis string

const (
	AdjustmentNone     AdjustmentBasis = "none"
	AdjustmentForward  AdjustmentBasis = "forward_adjusted"
	AdjustmentBackward AdjustmentBasis = "backward_adjusted"
	AdjustmentUnknown  AdjustmentBasis = "unknown"
)

// QualityStatus 表示由 Data 负责判定的数据质量状态。
type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityStale    QualityStatus = "stale"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

// QuoteSnapshot 是 Application DTO，不复用 Data HTTP JSON 响应结构。
// 可空数值继续使用指针；Volume 使用 int64 保留股数精度。
type QuoteSnapshot struct {
	Symbol               string
	Name                 string
	CurrentPrice         *float64
	Change               *float64
	ChangePercent        *float64
	Open                 *float64
	High                 *float64
	Low                  *float64
	PrevClose            *float64
	Volume               *int64
	Amount               *float64
	VolumeRatio          *float64
	TurnoverRate         *float64
	QuoteTime            *time.Time
	QuoteTimeSemantics   string
	FetchedAt            time.Time
	NormalizedAt         time.Time
	Source               string
	SourceVersion        string
	RawVolume            *float64
	RawVolumeUnit        string
	RawAmount            *float64
	RawAmountUnit        string
	RawAmountField       string
	VolumeUnit           string
	AmountUnit           string
	AdjustmentBasis      AdjustmentBasis
	QualityStatus        QualityStatus
	QualityReasons       []string
	FallbackFrom         string
	FallbackReason       string
	RequestID            string
	NormalizationVersion string
}

// DailyBar 是供下游分析使用的 Application DTO，保留来源和时点元数据。
// 未知数值字段保持 nil，不使用零值代替未知。
type DailyBar struct {
	Symbol                           string
	TradeDate                        string
	IsPartial                        bool
	Period                           string
	Open                             float64
	High                             float64
	Low                              float64
	Close                            float64
	MA5                              *float64
	MA20                             *float64
	MA120                            *float64
	MA250                            *float64
	Volume                           *int64
	Amount                           *float64
	ChangePercent                    *float64
	AdjustmentBasis                  AdjustmentBasis
	AdjustmentMethod                 string
	AdjustmentReferenceDate          string
	AdjustmentFactorSource           string
	AdjustmentFactorSourceVersion    string
	AdjustmentFactorSystemObservedAt *time.Time
	Source                           string
	SourceVersion                    string
	ProviderTime                     *time.Time
	FetchedAt                        time.Time
	NormalizedAt                     time.Time
	SystemObservedAt                 *time.Time
	RawVolume                        *float64
	RawVolumeUnit                    string
	RawAmount                        *float64
	RawAmountUnit                    string
	RawAmountField                   string
	VolumeUnit                       string
	AmountUnit                       string
	QualityStatus                    QualityStatus
	QualityReasons                   []string
	FallbackFrom                     string
	FallbackReason                   string
	RequestID                        string
	NormalizationVersion             string
}

// DailyBarsQuery 定义当前视图或历史日线读取的显式参数。
// SystemObservedAtCutoff 是系统观察/修订截止时点，不是交易日期，也不是行情的 ProviderTime。
// 该字段为 nil 时读取当前视图。
type DailyBarsQuery struct {
	Symbol                 string
	StartDate              string
	EndDate                string
	Limit                  int
	AdjustmentBasis        AdjustmentBasis
	SystemObservedAtCutoff *time.Time
}

// DailyBarsSnapshot 保留逐条日线元数据和 Data 汇总质量判定。
// QualityStatus 根据返回日线的质量状态及 Data warnings 汇总得出。
type DailyBarsSnapshot struct {
	Symbol                 string
	AdjustmentBasis        AdjustmentBasis
	VolumeUnit             string
	AmountUnit             string
	QualityStatus          QualityStatus
	Warnings               []string
	SystemObservedAtCutoff *time.Time
	Bars                   []DailyBar
}

// QuoteReader 是窄化的行情读取用例，不暴露 Provider 选择与调度。
type QuoteReader interface {
	GetQuote(ctx context.Context, symbol string) (QuoteSnapshot, error)
}

// DailyBarsReader 是窄化的历史日线读取用例。
// SystemObservedAtCutoff 非 nil 时，必须基于已记录的历史修订版本进行 PIT 重建。
type DailyBarsReader interface {
	GetDailyBars(ctx context.Context, query DailyBarsQuery) (DailyBarsSnapshot, error)
}

// MarketDataReader 是当前供 Analysis 使用的消费者 Port。
// 后续消费者应只依赖自己实际需要的最窄接口。
type MarketDataReader interface {
	QuoteReader
	DailyBarsReader
}
