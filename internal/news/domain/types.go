package domain

import "time"

// Candidate 保留搜索 Provider 实际返回的事实，不将发布者身份等同于 Provider 身份。
type Candidate struct {
	ProviderCode    string
	ProviderItemID  string
	PublisherName   string
	PublisherDomain string
	Title           string
	Summary         string
	RawURL          string
	PublishedAtRaw  string
	PublishedAt     *time.Time
	ObservedAt      time.Time
	QueryDimension  string
	MissingFields   []string
}

// SearchRequest 描述一次 Provider 查询；凭证和 endpoint 不由外部请求控制。
type SearchRequest struct {
	Query          string
	QueryDimension string
	Limit          int
}

// ProviderStatus 表示单个 Provider 查询任务的业务状态。
type ProviderStatus string

const (
	ProviderSuccess ProviderStatus = "success"
	ProviderEmpty   ProviderStatus = "empty"
	ProviderFailed  ProviderStatus = "failed"
	ProviderSkipped ProviderStatus = "skipped"
)

// ProviderOutcome 不把 Provider 部分失败压缩成一个全局 error。
type ProviderOutcome struct {
	ProviderCode   string
	QueryDimension string
	Status         ProviderStatus
	CandidateCount int
	ErrorCode      string
	Retryable      bool
	Duration       time.Duration
	ObservedAt     time.Time
}

// SearchStatus 表示一次搜索的聚合状态。
type SearchStatus string

const (
	SearchSuccess SearchStatus = "success"
	SearchEmpty   SearchStatus = "empty"
	SearchPartial SearchStatus = "partial"
	SearchFailed  SearchStatus = "failed"
	SearchSkipped SearchStatus = "skipped"
)

// SearchResult 同时保留候选和每个 Provider 的执行结果。
type SearchResult struct {
	Status      SearchStatus
	FailureCode string
	Candidates  []Candidate
	Outcomes    []ProviderOutcome
}
