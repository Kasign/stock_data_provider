package domain

import "time"

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusBlocked   RunStatus = "blocked_data_quality"
	RunStatusRejected  RunStatus = "rejected_request"
	RunStatusPartial   RunStatus = "partial"
	RunStatusFailed    RunStatus = "failed"
)

type TaskType string

const (
	TaskPostMarketScan TaskType = "post_market_scan"
	TaskWatchlistScan  TaskType = "watchlist_scan"
	TaskPositionReview TaskType = "position_review"
	TaskHistoricalRun  TaskType = "historical_replay"
)

type AsOfMode string

const (
	// AsOfModeExplicit 表示调用方明确指定了分析时点，必须严格遵守。
	AsOfModeExplicit AsOfMode = "explicit"

	// AsOfModeRealtimeSnapshot 表示调用方没有指定时点，
	// 系统在数据采集完成后确定本次分析的有效截止时间。
	AsOfModeRealtimeSnapshot AsOfMode = "realtime_snapshot"
)

type RunRequest struct {
	RunID                 string            `json:"run_id,omitempty"`
	TaskType              TaskType          `json:"task_type"`
	AsOf                  time.Time         `json:"as_of"`
	AsOfMode              AsOfMode          `json:"as_of_mode,omitempty"`
	UniverseID            string            `json:"universe_id"`
	Symbols               []string          `json:"symbols,omitempty"`
	PortfolioID           string            `json:"portfolio_id"`
	StrategyIDs           []string          `json:"strategy_ids"`
	MarketID              string            `json:"market_id"`
	StrategyConfigVersion string            `json:"strategy_config_version"`
	RiskConfigVersion     string            `json:"risk_config_version"`
	TradingRulesVersion   string            `json:"trading_rules_version"`
	DataPolicyVersion     string            `json:"data_policy_version"`
	Parameters            map[string]string `json:"parameters,omitempty"`
}

type RunResult struct {
	RunID               string              `json:"run_id"`
	Status              RunStatus           `json:"status"`
	Request             RunRequest          `json:"request"`
	StartedAt           time.Time           `json:"started_at"`
	FinishedAt          time.Time           `json:"finished_at,omitempty"`
	Universe            UniverseSnapshot    `json:"universe,omitempty"`
	RawSnapshotID       string              `json:"raw_snapshot_id,omitempty"`
	MarketSnapshotID    string              `json:"market_snapshot_id,omitempty"`
	PortfolioSnapshotID string              `json:"portfolio_snapshot_id,omitempty"`
	Portfolio           *PortfolioSnapshot  `json:"portfolio,omitempty"`
	Quality             QualityReport       `json:"quality,omitempty"`
	MarketState         MarketState         `json:"market_state,omitempty"`
	Factors             []FactorValue       `json:"factors,omitempty"`
	Signals             []CandidateSignal   `json:"signals,omitempty"`
	Tradability         []SignalDecision    `json:"tradability,omitempty"`
	Risk                []SignalDecision    `json:"risk,omitempty"`
	Plans               []TradePlan         `json:"plans,omitempty"`
	PlanningRejections  []PlanRejection     `json:"planning_rejections,omitempty"`
	Execution           ExecutionAssessment `json:"execution_assessment,omitempty"`
	Events              []StageEvent        `json:"events,omitempty"`
	Failure             *Failure            `json:"failure,omitempty"`
	Warnings            []string            `json:"warnings,omitempty"`
}

type Failure struct {
	Code    string `json:"code"`
	Stage   string `json:"stage,omitempty"`
	Message string `json:"message"`
}

type StageStatus string

const (
	StageStatusStarted   StageStatus = "started"
	StageStatusSucceeded StageStatus = "succeeded"
	StageStatusFailed    StageStatus = "failed"
	StageStatusSkipped   StageStatus = "skipped"
)

type StageEvent struct {
	RunID      string            `json:"run_id"`
	Stage      string            `json:"stage"`
	Status     StageStatus       `json:"status"`
	At         time.Time         `json:"at"`
	DurationMS int64             `json:"duration_ms,omitempty"`
	Message    string            `json:"message,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type Security struct {
	Symbol       string            `json:"symbol"`
	Exchange     string            `json:"exchange"`
	AssetType    string            `json:"asset_type"`
	Board        string            `json:"board"`
	ListedAt     *time.Time        `json:"listed_at,omitempty"`
	DelistedAt   *time.Time        `json:"delisted_at,omitempty"`
	TradingState string            `json:"trading_state"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

type UniverseSnapshot struct {
	ID         string     `json:"id"`
	AsOf       time.Time  `json:"as_of"`
	Securities []Security `json:"securities"`
	Version    string     `json:"version"`
}

type PortfolioSnapshot struct {
	ID            string      `json:"id,omitempty"`
	PortfolioID   string      `json:"portfolio_id"`
	AsOf          time.Time   `json:"as_of"`
	Currency      string      `json:"currency"`
	AvailableCash float64     `json:"available_cash"`
	TotalEquity   float64     `json:"total_equity"`
	Positions     []Position  `json:"positions,omitempty"`
	OpenOrders    []OpenOrder `json:"open_orders,omitempty"`
	Version       string      `json:"version"`
}

type Position struct {
	Symbol           string  `json:"symbol"`
	Quantity         int64   `json:"quantity"`
	SellableQuantity int64   `json:"sellable_quantity"`
	AverageCost      float64 `json:"average_cost"`
	MarketValue      float64 `json:"market_value"`
	IndustryID       string  `json:"industry_id,omitempty"`
}

type OpenOrder struct {
	OrderID           string    `json:"order_id"`
	Symbol            string    `json:"symbol"`
	Side              string    `json:"side"`
	RemainingQuantity int64     `json:"remaining_quantity"`
	LimitPrice        float64   `json:"limit_price,omitempty"`
	Status            string    `json:"status"`
	ExpiresAt         time.Time `json:"expires_at,omitempty"`
}

type DataRequirement struct {
	Dataset  string   `json:"dataset"`
	Fields   []string `json:"fields,omitempty"`
	Required bool     `json:"required"`
	Reason   string   `json:"reason,omitempty"`
	Consumer string   `json:"consumer,omitempty"`
}

type RequirementSet struct {
	Core       []DataRequirement            `json:"core"`
	ByStrategy map[string][]DataRequirement `json:"by_strategy"`
	FactorIDs  []string                     `json:"factor_ids,omitempty"`
	ByFactor   map[string][]DataRequirement `json:"by_factor,omitempty"`
}

type RawSnapshot struct {
	ID          string            `json:"id"`
	AsOf        time.Time         `json:"as_of"`
	SourceNotes []SourceNote      `json:"source_notes,omitempty"`
	PayloadRef  string            `json:"payload_ref,omitempty"`
	Quotes      []RawQuote        `json:"quotes,omitempty"`
	Bars        []RawBar          `json:"bars,omitempty"`
	IndexBars   []RawBar          `json:"index_bars,omitempty"`
	Actions     []CorporateAction `json:"actions,omitempty"`
}

// RawQuote retains quote fields exactly as supplied by the upstream adapter.
// Nil optional values mean "not supplied", never zero or a positive/negative inference.
type RawQuote struct {
	Symbol        string     `json:"symbol"`
	Name          string     `json:"name,omitempty"`
	CurrentPrice  float64    `json:"current_price"`
	Change        *float64   `json:"change,omitempty"`
	ChangePercent *float64   `json:"change_percent,omitempty"`
	Open          *float64   `json:"open,omitempty"`
	High          *float64   `json:"high,omitempty"`
	Low           *float64   `json:"low,omitempty"`
	PrevClose     *float64   `json:"prev_close,omitempty"`
	Volume        *float64   `json:"volume,omitempty"`
	Amount        *float64   `json:"amount,omitempty"`
	QuoteTime     *time.Time `json:"quote_time,omitempty"`
	Source        string     `json:"source"`
	FetchedAt     time.Time  `json:"fetched_at"`
	AvailableAt   time.Time  `json:"available_at,omitempty"`
}

type SourceNote struct {
	Dataset         string    `json:"dataset"`
	Symbol          string    `json:"symbol,omitempty"`
	Source          string    `json:"source"`
	FetchedAt       time.Time `json:"fetched_at"`
	AvailableAt     time.Time `json:"available_at,omitempty"`
	QualityStatus   string    `json:"quality_status"`
	Diagnostic      string    `json:"diagnostic,omitempty"`
	FallbackFrom    string    `json:"fallback_from,omitempty"`
	MissingFields   []string  `json:"missing_fields,omitempty"`
	AdjustmentBasis string    `json:"adjustment_basis,omitempty"`
}

type RawBar struct {
	Symbol        string    `json:"symbol"`
	TradeDate     time.Time `json:"trade_date"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Close         float64   `json:"close"`
	Volume        *float64  `json:"volume,omitempty"`
	Amount        *float64  `json:"amount,omitempty"`
	Source        string    `json:"source"`
	FetchedAt     time.Time `json:"fetched_at"`
	AvailableAt   time.Time `json:"available_at,omitempty"`
	AdjustBasis   string    `json:"adjustment_basis"`
	MissingFields []string  `json:"missing_fields,omitempty"`
}

type CorporateAction struct {
	Symbol      string    `json:"symbol"`
	EffectiveAt time.Time `json:"effective_at"`
	PublishedAt time.Time `json:"published_at"`
	ActionType  string    `json:"action_type"`
	Adjustment  float64   `json:"adjustment"`
	Source      string    `json:"source"`
}

type Quote struct {
	Symbol        string     `json:"symbol"`
	Name          string     `json:"name,omitempty"`
	CurrentPrice  float64    `json:"current_price"`
	Change        *float64   `json:"change,omitempty"`
	ChangePercent *float64   `json:"change_percent,omitempty"`
	Open          *float64   `json:"open,omitempty"`
	High          *float64   `json:"high,omitempty"`
	Low           *float64   `json:"low,omitempty"`
	PrevClose     *float64   `json:"prev_close,omitempty"`
	Volume        *float64   `json:"volume,omitempty"`
	Amount        *float64   `json:"amount,omitempty"`
	QuoteTime     *time.Time `json:"quote_time,omitempty"`
	Source        string     `json:"source"`
	FetchedAt     time.Time  `json:"fetched_at"`
	AvailableAt   time.Time  `json:"available_at,omitempty"`
}

type Bar struct {
	Symbol        string    `json:"symbol"`
	TradeDate     time.Time `json:"trade_date"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Close         float64   `json:"close"`
	Volume        *float64  `json:"volume,omitempty"`
	Amount        *float64  `json:"amount,omitempty"`
	Source        string    `json:"source"`
	FetchedAt     time.Time `json:"fetched_at"`
	AvailableAt   time.Time `json:"available_at,omitempty"`
	Basis         string    `json:"adjustment_basis"`
	MissingFields []string  `json:"missing_fields,omitempty"`
}

type MarketSnapshot struct {
	ID        string            `json:"id"`
	AsOf      time.Time         `json:"as_of"`
	Quotes    map[string]Quote  `json:"quotes,omitempty"`
	Bars      map[string][]Bar  `json:"bars"`
	IndexBars map[string][]Bar  `json:"index_bars,omitempty"`
	Actions   []CorporateAction `json:"actions,omitempty"`
	Sources   []SourceNote      `json:"sources,omitempty"`
}

type DataIssue struct {
	Code       string `json:"code"`
	Dataset    string `json:"dataset,omitempty"`
	Symbol     string `json:"symbol,omitempty"`
	Field      string `json:"field,omitempty"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	StrategyID string `json:"strategy_id,omitempty"`
}

type QualityReport struct {
	CorePass         bool                `json:"core_pass"`
	Partial          bool                `json:"partial"`
	Issues           []DataIssue         `json:"issues,omitempty"`
	StrategyEligible map[string]bool     `json:"strategy_eligible"`
	StrategyBlockers map[string][]string `json:"strategy_blockers,omitempty"`
	CheckedAt        time.Time           `json:"checked_at"`
	PolicyVersion    string              `json:"policy_version"`
}

type MarketState struct {
	AsOf             time.Time          `json:"as_of"`
	Regime           string             `json:"regime"`
	Metrics          map[string]float64 `json:"metrics,omitempty"`
	Evidence         []string           `json:"evidence,omitempty"`
	AlgorithmVersion string             `json:"algorithm_version"`
}

type FactorValue struct {
	Symbol        string    `json:"symbol"`
	FactorID      string    `json:"factor_id"`
	FactorVersion string    `json:"factor_version"`
	Value         *float64  `json:"value,omitempty"`
	Unit          string    `json:"unit,omitempty"`
	AsOf          time.Time `json:"as_of"`
	InputQuality  string    `json:"input_quality"`
	MissingReason string    `json:"missing_reason,omitempty"`
	EvidenceRefs  []string  `json:"evidence_refs,omitempty"`
}

type CandidateSignal struct {
	ID               string            `json:"id"`
	Symbol           string            `json:"symbol"`
	StrategyID       string            `json:"strategy_id"`
	StrategyVersion  string            `json:"strategy_version"`
	Direction        string            `json:"direction"`
	SignalAt         time.Time         `json:"signal_at"`
	ValidUntil       time.Time         `json:"valid_until,omitempty"`
	EntryRule        string            `json:"entry_rule"`
	ExitRule         string            `json:"exit_rule"`
	InvalidationRule string            `json:"invalidation_rule"`
	ExpectedHoldDays int               `json:"expected_hold_days,omitempty"`
	ReferencePrice   *float64          `json:"reference_price,omitempty"`
	EvidenceRefs     []string          `json:"evidence_refs,omitempty"`
	Attributes       map[string]string `json:"attributes,omitempty"`
}

type RuleRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type SignalDecision struct {
	SignalID string    `json:"signal_id"`
	Allowed  bool      `json:"allowed"`
	Reasons  []string  `json:"reasons,omitempty"`
	RuleIDs  []string  `json:"rule_ids,omitempty"`
	RuleRefs []RuleRef `json:"rule_refs,omitempty"`
}

type TradePlan struct {
	SignalID          string    `json:"signal_id"`
	Symbol            string    `json:"symbol"`
	Side              string    `json:"side"`
	Quantity          int64     `json:"quantity"`
	ReferencePrice    float64   `json:"reference_price"`
	StopPrice         *float64  `json:"stop_price,omitempty"`
	TargetPrice       *float64  `json:"target_price,omitempty"`
	EarliestExecuteAt time.Time `json:"earliest_execute_at"`
	ExpireAt          time.Time `json:"expire_at,omitempty"`
	RiskBudget        float64   `json:"risk_budget"`
	CostEstimate      float64   `json:"cost_estimate"`
	RuleVersion       string    `json:"rule_version"`
}

type PlanRejection struct {
	SignalID string   `json:"signal_id"`
	Reasons  []string `json:"reasons"`
}

// SimulatedFill is an immutable execution event emitted by a later paper-execution
// worker or a backtest ledger. The single-cutoff Runner must not invent future fills.
type SimulatedFill struct {
	SignalID string    `json:"signal_id"`
	Symbol   string    `json:"symbol"`
	Side     string    `json:"side"`
	Quantity int64     `json:"quantity"`
	Price    float64   `json:"price"`
	Fees     float64   `json:"fees"`
	Slippage float64   `json:"slippage"`
	FilledAt time.Time `json:"filled_at"`
	Status   string    `json:"status"`
	Reason   string    `json:"reason,omitempty"`
}

type ExecutionOrderStatus string

const (
	ExecutionPending         ExecutionOrderStatus = "pending"
	ExecutionFilled          ExecutionOrderStatus = "filled"
	ExecutionPartiallyFilled ExecutionOrderStatus = "partially_filled"
	ExecutionUnfilled        ExecutionOrderStatus = "unfilled"
	ExecutionBlocked         ExecutionOrderStatus = "blocked"
	ExecutionExpired         ExecutionOrderStatus = "expired"
	ExecutionRejected        ExecutionOrderStatus = "rejected"
)

type ExecutionOrderAssessment struct {
	SignalID          string               `json:"signal_id"`
	Status            ExecutionOrderStatus `json:"status"`
	RequestedQuantity int64                `json:"requested_quantity"`
	FilledQuantity    int64                `json:"filled_quantity"`
	RemainingQuantity int64                `json:"remaining_quantity"`
	Reason            string               `json:"reason,omitempty"`
}

type ExecutionAssessment struct {
	ModelVersion string                     `json:"model_version"`
	Orders       []ExecutionOrderAssessment `json:"orders,omitempty"`
	Warnings     []string                   `json:"warnings,omitempty"`
}
