package publicdto

import (
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// Run is the versioned public representation. Nullable fields are emitted as explicit nulls.
type ExcludedStage struct {
	StageCode   string `json:"stage_code"`
	RuleVersion string `json:"rule_version"`
	ReasonCode  string `json:"reason_code"`
}

type ResultSummary struct {
	StageOutcomes       map[string]string `json:"stage_outcomes"`
	ExcludedStages      []ExcludedStage   `json:"excluded_stages"`
	MissingStageCodes   []string          `json:"missing_stage_codes"`
	DegradedStageCodes  []string          `json:"degraded_stage_codes"`
	BlockedStageCodes   []string          `json:"blocked_stage_codes"`
	ReasonCodes         []string          `json:"reason_codes"`
	Coverage            float64           `json:"coverage"`
	QualityStatus       string            `json:"quality_status"`
	RiskGateDecisions   map[string]string `json:"risk_gate_decisions"`
	ActionabilityStatus string            `json:"actionability_status"`
}

type Run struct {
	SchemaVersion        string     `json:"schema_version"`
	RunRef               string     `json:"run_ref"`
	Status               string     `json:"status"`
	Version              uint64     `json:"version"`
	SystemAsOf           time.Time  `json:"system_as_of"`
	DeadlineAt           time.Time  `json:"deadline_at"`
	TemplateRef          string     `json:"template_ref"`
	TemplateVersion      string     `json:"template_version"`
	TemplateFingerprint  string     `json:"template_fingerprint"`
	InputSnapshotRefs    []string   `json:"input_snapshot_refs"`
	ResultSummaryRef     *string    `json:"result_summary_ref"`
	OriginatingRequestID *string    `json:"originating_request_id"`
	CorrelationID        string     `json:"correlation_id"`
	TerminationReason    *string    `json:"termination_reason"`
	CreatedAt            time.Time  `json:"created_at"`
	StartedAt            *time.Time `json:"started_at"`
	FinishedAt           *time.Time `json:"finished_at"`
}

type Stage struct {
	SchemaVersion       string   `json:"schema_version"`
	StageRef            string   `json:"stage_ref"`
	RunRef              string   `json:"run_ref"`
	StageCode           string   `json:"stage_code"`
	Status              string   `json:"status"`
	Version             uint64   `json:"version"`
	Required            bool     `json:"required"`
	DependencyStageRefs []string `json:"dependency_stage_refs"`
	RetryPolicyRef      string   `json:"retry_policy_ref"`

	RetryAt           *time.Time `json:"retry_at"`
	OutputRef         *string    `json:"output_ref"`
	QualitySummaryRef *string    `json:"quality_summary_ref"`
	DiagnosticRefs    []string   `json:"diagnostic_refs"`
	ReasonCodes       []string   `json:"reason_codes"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at"`
}

type Attempt struct {
	SchemaVersion  string     `json:"schema_version"`
	AttemptRef     string     `json:"attempt_ref"`
	StageRef       string     `json:"stage_ref"`
	AttemptNo      int        `json:"attempt_no"`
	Status         string     `json:"status"`
	LeaseOwner     *string    `json:"lease_owner"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at"`
	FencingToken   uint64     `json:"fencing_token"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	FaultRef       *string    `json:"fault_ref"`
	OutputRef      *string    `json:"output_ref"`
}

func RunFromDomain(v domain.Run) Run {
	return Run{SchemaVersion: v.SchemaVersion, RunRef: v.Ref, Status: string(v.Status), Version: v.Version, SystemAsOf: v.SystemAsOf, DeadlineAt: v.DeadlineAt, TemplateRef: v.TemplateRef, TemplateVersion: v.TemplateVersion, TemplateFingerprint: v.TemplateFingerprint, InputSnapshotRefs: cloneStrings(v.InputSnapshotRefs), ResultSummaryRef: cloneStringPointer(v.ResultSummaryRef), OriginatingRequestID: cloneStringPointer(v.OriginatingRequestID), CorrelationID: v.CorrelationID, TerminationReason: stringPointer(v.TerminationReason), CreatedAt: v.CreatedAt, StartedAt: cloneTimePointer(v.StartedAt), FinishedAt: cloneTimePointer(v.FinishedAt)}
}
func StageFromDomain(v domain.Stage, schemaVersion string) Stage {
	return Stage{SchemaVersion: schemaVersion, StageRef: v.Ref, RunRef: v.RunRef, StageCode: v.Code, Status: string(v.Status), Version: v.Version, Required: v.Required, DependencyStageRefs: cloneStrings(v.Dependencies), RetryPolicyRef: v.RetryPolicyRef, RetryAt: cloneTimePointer(v.RetryAt), OutputRef: cloneStringPointer(v.OutputRef), QualitySummaryRef: cloneStringPointer(v.QualitySummaryRef), DiagnosticRefs: cloneStrings(v.DiagnosticRefs), ReasonCodes: cloneStrings(v.ReasonCodes), CreatedAt: v.CreatedAt, StartedAt: cloneTimePointer(v.StartedAt), FinishedAt: cloneTimePointer(v.FinishedAt)}
}
func AttemptFromDomain(v domain.Attempt, schemaVersion string) Attempt {
	return Attempt{SchemaVersion: schemaVersion, AttemptRef: v.Ref, StageRef: v.StageRef, AttemptNo: v.Number, Status: string(v.Status), LeaseOwner: stringPointer(v.LeaseOwner), LeaseExpiresAt: cloneTimePointer(v.LeaseExpiresAt), FencingToken: v.FencingToken, StartedAt: v.StartedAt, FinishedAt: cloneTimePointer(v.FinishedAt), FaultRef: cloneStringPointer(v.FaultRef), OutputRef: cloneStringPointer(v.OutputRef)}
}
func cloneStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return append([]string(nil), v...)
}
func stringPointer(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func ResultSummaryFromDomain(v *domain.ResultSummary) *ResultSummary {
	if v == nil {
		return nil
	}
	out := &ResultSummary{
		StageOutcomes:       make(map[string]string, len(v.StageOutcomes)),
		ExcludedStages:      make([]ExcludedStage, 0, len(v.ExcludedStages)),
		MissingStageCodes:   cloneStrings(v.MissingStageCodes),
		DegradedStageCodes:  cloneStrings(v.DegradedStageCodes),
		BlockedStageCodes:   cloneStrings(v.BlockedStageCodes),
		ReasonCodes:         cloneStrings(v.ReasonCodes),
		Coverage:            v.Coverage,
		QualityStatus:       v.QualityStatus,
		RiskGateDecisions:   make(map[string]string, len(v.RiskGateDecisions)),
		ActionabilityStatus: v.ActionabilityStatus,
	}
	for code, status := range v.StageOutcomes {
		out.StageOutcomes[code] = string(status)
	}
	for _, item := range v.ExcludedStages {
		out.ExcludedStages = append(out.ExcludedStages, ExcludedStage{StageCode: item.StageCode, RuleVersion: item.RuleVersion, ReasonCode: item.ReasonCode})
	}
	for code, decision := range v.RiskGateDecisions {
		out.RiskGateDecisions[code] = decision
	}
	return out
}
func cloneStringPointer(v *string) *string {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}
func cloneTimePointer(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}
