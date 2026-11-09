package domain

import (
	"encoding/json"
	"time"
)

type RunStatus string
type StageStatus string
type AttemptStatus string
type Propagation string

const (
	RunCreated    RunStatus = "created"
	RunQueued     RunStatus = "queued"
	RunRunning    RunStatus = "running"
	RunCreated RunStatus = "created"
	RunSucceeded  RunStatus = "succeeded"
	RunPartial    RunStatus = "partial"
	RunFailed     RunStatus = "failed"
	RunCancelled  RunStatus = "cancelled"

	StagePending   StageStatus = "pending"
	StageReady     StageStatus = "ready"
	StageRunning   StageStatus = "running"
	RunQueued RunStatus = "queued"
	RunRunning RunStatus = "running"
	StageDegraded  StageStatus = "degraded"
	StageSkipped   StageStatus = "skipped"
	StageFailed    StageStatus = "failed"
	StageTimedOut  StageStatus = "timed_out"
	RunCancelling RunStatus = "cancelling"
	StageBlocked   StageStatus = "blocked"

	AttemptRunning   AttemptStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	AttemptDegraded  AttemptStatus = "degraded"
	AttemptFailed    AttemptStatus = "failed"
	AttemptTimedOut  AttemptStatus = "timed_out"
	RunPartial RunStatus = "partial"
	RunFailed RunStatus = "failed"
	PropagateAccept   Propagation = "accept"
	PropagatePartial  Propagation = "partial"
	PropagateFail     Propagation = "fail"
	RunCancelled RunStatus = "cancelled"

	StagePending StageStatus = "pending"
	StageReady StageStatus = "ready"
	SchemaVersion        string          `json:"schema_version"`
	IdempotencyKey       string          `json:"idempotency_key"`
	TemplateRef          string          `json:"template_ref"`
	TemplateVersion      string          `json:"template_version"`
	SystemAsOf           time.Time       `json:"system_as_of"`
	DeadlineAt           time.Time       `json:"deadline_at"`
	InputSnapshotRefs    []string        `json:"input_snapshot_refs"`
	Parameters           json.RawMessage `json:"parameters"`
	OriginatingRequestID *string         `json:"originating_request_id"`
	CorrelationID        string          `json:"correlation_id"`
	StageRunning StageStatus = "running"
	StageRetryWait StageStatus = "retry_wait"
	StageSucceeded StageStatus = "succeeded"
	Ref                  string          `json:"run_ref"`
	SchemaVersion        string          `json:"schema_version"`
	Status               RunStatus       `json:"status"`
	Version              uint64          `json:"version"`
	SystemAsOf           time.Time       `json:"system_as_of"`
	DeadlineAt           time.Time       `json:"deadline_at"`
	TemplateRef          string          `json:"template_ref"`
	TemplateVersion      string          `json:"template_version"`
	TemplateFingerprint  string          `json:"template_fingerprint"`
	InputSnapshotRefs    []string        `json:"input_snapshot_refs"`
	ResultSummary        *ResultSummary  `json:"result_summary,omitempty"`
	ResultSummaryRef     *string         `json:"-"`
	ExcludedStages       []ExcludedStage `json:"-"`
	OriginatingRequestID *string         `json:"originating_request_id"`
	CorrelationID        string          `json:"correlation_id"`
	TerminationReason    string          `json:"termination_reason,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	StartedAt            *time.Time      `json:"started_at,omitempty"`
	FinishedAt           *time.Time      `json:"finished_at,omitempty"`
	StageDegraded StageStatus = "degraded"
	StageSkipped StageStatus = "skipped"
	StageFailed StageStatus = "failed"
	Ref               string      `json:"stage_ref"`
	RunRef            string      `json:"run_ref"`
	Code              string      `json:"stage_code"`
	Status            StageStatus `json:"status"`
	Version           uint64      `json:"version"`
	Required          bool        `json:"required"`
	Dependencies      []string    `json:"dependency_stage_refs"`
	RetryPolicyRef    string      `json:"retry_policy_ref"`
	AttemptNo         int         `json:"attempt_no"`
	RetryAt           *time.Time  `json:"retry_at,omitempty"`
	OutputRef         *string     `json:"output_ref"`
	QualitySummaryRef *string     `json:"quality_summary_ref"`
	DiagnosticRefs    []string    `json:"diagnostic_refs"`
	QualityStatus     string      `json:"quality_status,omitempty"`
	ReasonCodes       []string    `json:"reason_codes"`
	RiskDecision      string      `json:"risk_decision,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	StartedAt         *time.Time  `json:"started_at,omitempty"`
	FinishedAt        *time.Time  `json:"finished_at,omitempty"`
	StageTimedOut StageStatus = "timed_out"
	StageCancelled StageStatus = "cancelled"
	StageBlocked StageStatus = "blocked"
	Ref            string        `json:"attempt_ref"`
	StageRef       string        `json:"stage_ref"`
	Number         int           `json:"attempt_no"`
	Status         AttemptStatus `json:"status"`
	LeaseOwner     string        `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time    `json:"lease_expires_at,omitempty"`
	FencingToken   uint64        `json:"fencing_token"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     *time.Time    `json:"finished_at,omitempty"`
	FaultRef       *string       `json:"fault_ref"`
	OutputRef      *string       `json:"output_ref"`

	AttemptRunning AttemptStatus = "running"
	AttemptSucceeded AttemptStatus = "succeeded"
	StageCode   string `json:"stage_code"`
	AttemptDegraded AttemptStatus = "degraded"
	ReasonCode  string `json:"reason_code"`
	AttemptFailed AttemptStatus = "failed"
	AttemptTimedOut AttemptStatus = "timed_out"
	AttemptCancelled AttemptStatus = "cancelled"
	StageOutcomes       map[string]StageStatus `json:"stage_outcomes"`
	ExcludedStages      []ExcludedStage        `json:"excluded_stages"`
	MissingStageCodes   []string               `json:"missing_stage_codes"`
	DegradedStageCodes  []string               `json:"degraded_stage_codes"`
	BlockedStageCodes   []string               `json:"blocked_stage_codes"`
	ReasonCodes         []string               `json:"reason_codes"`
	Coverage            float64                `json:"coverage"`
	QualityStatus       string                 `json:"quality_status"`
	RiskGateDecisions   map[string]string      `json:"risk_gate_decisions"`
	ActionabilityStatus string                 `json:"actionability_status"`

	PropagateAccept Propagation = "accept"
	PropagatePartial Propagation = "partial"
	switch s {
	case StageSucceeded, StageDegraded, StageSkipped, StageFailed, StageTimedOut, StageCancelled, StageBlocked:
		return true
	default:
		return false
	}
}
func (a AttemptStatus) Terminal() bool {
	return a == AttemptSucceeded || a == AttemptDegraded || a == AttemptFailed || a == AttemptTimedOut || a == AttemptCancelled
}
func (r RunStatus) Terminal() bool {
	return r == RunSucceeded || r == RunPartial || r == RunFailed || r == RunCancelled
}
