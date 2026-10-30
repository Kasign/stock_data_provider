package model

import "time"

// ProviderProbeItem 是一次主动能力探测结果。ErrorCode 仅暴露分类，不回传上游原始错误、URL 或凭据。
type ProviderProbeItem struct {
	Provider string `json:"provider"`
	Capability string `json:"capability"`
	Status string `json:"status"`
	Persisted bool `json:"persisted"`
	ElapsedMS int64 `json:"elapsed_ms"`
	SourceVersion string `json:"source_version,omitempty"`
	QualityStatus QualityStatus `json:"quality_status"`
	QualityReasons []string `json:"quality_reasons,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// ProviderProbeRun 是隔离于业务流量的主动探测批次。
type ProviderProbeRun struct {
	RunID string `json:"run_id"`
	Status string `json:"status"`
	StartedAt time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Total int `json:"total"`
	Returned int `json:"returned"`
	Degraded int `json:"degraded"`
	Failed int `json:"failed"`
	Items []ProviderProbeItem `json:"items"`
}

// ProviderProbeHistory 是最近的主动探测批次列表。
type ProviderProbeHistory struct {
	Items []ProviderProbeRun `json:"items"`
	Total int `json:"total"`
}
