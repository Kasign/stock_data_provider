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
// 字段来源与 model.Quote 一一对应；可空数值继续使用指针，Volume 使用 int64 保留股数精度。
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

// DailyBar 是供下游分析使用的 Application DTO，字段来源与 model.DailyBar 一一对应。
// 保留来源和时点元数据；未知数值字段保持 nil，不使用零值代替未知。
type DailyBar struct {
	Symbol                                string
	TradeDate                             string
	IsPartial                             bool
	Period                                string
	Open                                  float64
	High                                  float64
	Low                                   float64
	Close                                 float64
	MA5                                   *float64
	MA20                                  *float64
	MA120                                 *float64
	MA250                                 *float64
	Volume                                *int64
	Amount                                *float64
	ChangePercent                         *float64
	AdjustmentBasis                       AdjustmentBasis
	AdjustmentMethod                      string
	AdjustmentReferenceDate               string
	AdjustmentFactorSource                string
	AdjustmentFactorSourceVersion         string
	AdjustmentFactorRevisionHash          string
	AdjustmentFactorSetHash               string
	AdjustmentFactorReferenceRevisionHash string
	AdjustmentAlgorithmVersion            string
	AdjustmentFactorSystemObservedAt      *time.Time
	Source                                string
	SourceVersion                         string
	ProviderTime                          *time.Time
	FetchedAt                             time.Time
	NormalizedAt                          time.Time
	SystemObservedAt                      *time.Time
	RawVolume                             *float64
	RawVolumeUnit                         string
	RawAmount                             *float64
	RawAmountUnit                         string
	RawAmountField                        string
	VolumeUnit                            string
	AmountUnit                            string
	QualityStatus                         QualityStatus
	QualityReasons                        []string
	FallbackFrom                          string
	FallbackReason                        string
	RequestID                             string
	NormalizationVersion                  string
}

// DailyBarsQuery 定义当前视图或历史日线读取的显式参数。
// SystemObservedAtCutoff 是系统观察/修订截止时点，不是交易日期，也不是行情的 ProviderTime。
// 该字段为 nil 时读取当前视图。
type DailyBarsQuery struct {
	Symbol                 string
	StartDate              string // 包含边界，格式为 YYYY-MM-DD；空值由 DataService 按现有规则处理。
	EndDate                string // 包含边界，格式为 YYYY-MM-DD；PIT 查询不得晚于截止时点所在交易日。
	Limit                  int    // 请求日线条数；非正数和上限由 DataService 现有配置规则处理。
	AdjustmentBasis        AdjustmentBasis
	SystemObservedAtCutoff *time.Time
}

// DailyBarsSnapshot 保留逐条日线元数据和 Data 汇总质量判定。
// QualityStatus 根据返回日线的质量状态及 Data warnings 汇总得出。
type DailyBarsSnapshot struct {
	Symbol                 string
	AdjustmentBasis        AdjustmentBasis
	QualityStatus          QualityStatus
	Warnings               []string
	SystemObservedAtCutoff *time.Time
	Bars                   []DailyBar
}

// FundamentalsQuery 定义单证券、单报告期的财务数据读取请求。
// SystemObservedAtCutoff 非 nil 时只读取该系统观察时点已持久化的历史修订。
type FundamentalsQuery struct {
	Symbol                 string
	Year                   int
	Quarter                int
	SystemObservedAtCutoff *time.Time
}

// FundamentalMetric 保留财务指标标准键、展示名称、值、单位和源字段映射。
type FundamentalMetric struct {
	Key         string
	Name        string
	Value       float64
	Unit        string
	SourceField string
}

// FundamentalSection 是具有独立披露时间和质量状态的财务数据分区。
type FundamentalSection struct {
	Category        string
	ReportDate      string
	PublicationDate string
	AvailableAt     *time.Time
	HasData         bool
	Metrics         []FundamentalMetric
	QualityStatus   QualityStatus
	QualityReasons  []string
}

// FundamentalsSnapshot 是 Data 对外提供的财务快照；SystemObservedAt 不等于源端披露时间。
type FundamentalsSnapshot struct {
	Symbol           string
	Year             int
	Quarter          int
	HasData          bool
	Sections         []FundamentalSection
	Source           string
	SourceVersion    string
	FetchedAt        time.Time
	SystemObservedAt *time.Time
	QualityStatus    QualityStatus
	QualityReasons   []string
}

// ValuationsQuery 定义批量估值读取请求。Symbols 的数量、格式和去重由 DataService 校验。
type ValuationsQuery struct {
	Symbols                []string
	SystemObservedAtCutoff *time.Time
}

// ValuationItem 是单证券估值数据；源端缺失指标保持 nil，不补零。
type ValuationItem struct {
	Symbol                   string
	Code                     string
	Name                     string
	PETTM                    *float64
	PEMRQ                    *float64
	PEStatic                 *float64
	PBMRQ                    *float64
	PSTTM                    *float64
	PCFTTM                   *float64
	TurnoverRatePercent      *float64
	FloatTurnoverRatePercent *float64
	VolumeRatio              *float64
	DividendYieldPercent     *float64
	DividendYieldTTMPercent  *float64
	Source                   string
	SourceVersion            string
	FetchedAt                time.Time
	QualityStatus            QualityStatus
	QualityReasons           []string
}

// ValuationsSnapshot 是一次批量估值快照。ProviderTimestamp 是聚合时间，不代表每项指标更新时间。
type ValuationsSnapshot struct {
	RequestedSymbols   []string
	Total              int
	Items              []ValuationItem
	MissingSymbols     []string
	ProviderReportDate string
	ProviderTimestamp  *time.Time
	Source             string
	SourceVersion      string
	FetchedAt          time.Time
	SystemObservedAt   *time.Time
	QualityStatus      QualityStatus
	QualityReasons     []string
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

// FundamentalsReader 是窄化的财务数据读取用例。
type FundamentalsReader interface {
	GetFundamentals(ctx context.Context, query FundamentalsQuery) (FundamentalsSnapshot, error)
}

// ValuationsReader 是窄化的批量估值读取用例。
type ValuationsReader interface {
	GetValuations(ctx context.Context, query ValuationsQuery) (ValuationsSnapshot, error)
}

// MarketDataReader 是 Data 对外提供的完整首批只读 Application Port。
// 消费者仍应优先依赖自己实际需要的最窄接口。
type MarketDataReader interface {
	QuoteReader
	DailyBarsReader
	FundamentalsReader
	ValuationsReader
}
