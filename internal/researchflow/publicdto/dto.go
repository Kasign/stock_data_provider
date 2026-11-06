package publicdto

import (
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// Run is the versioned public representation. Nullable fields are emitted as explicit nulls.
type Run struct {
	SchemaVersion string `json:"schema_version"`
	RunRef string `json:"run_ref"`
	Status domain.RunStatus `json:"status"`
	Version uint64 `json:"version"`
	SystemAsOf time.Time `json:"system_as_of"`
	DeadlineAt time.Time `json:"deadline_at"`
	TemplateRef string `json:"template_ref"`
	TemplateVersion string `json:"template_version"`
	TemplateFingerprint string `json:"template_fingerprint"`
	InputSnapshotRefs []string `json:"input_snapshot_refs"`
	ResultSummary *domain.ResultSummary `json:"result_summary"`
	OriginatingRequestID *string `json:"originating_request_id"`
	CorrelationID string `json:"correlation_id"`
	TerminationReason *string `json:"termination_reason"`
	CreatedAt time.Time `json:"created_at"`
	StartedAt *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type Stage struct {
	SchemaVersion string `json:"schema_version"`
	StageRef string `json:"stage_ref"`
	RunRef string `json:"run_ref"`
	StageCode string `json:"stage_code"`
	Status domain.StageStatus `json:"status"`
	Version uint64 `json:"version"`
	Required bool `json:"required"`
	DependencyStageRefs []string `json:"dependency_stage_refs"`
	RetryPolicyRef string `json:"retry_policy_ref"`
	AttemptNo int `json:"attempt_no"`
	RetryAt *time.Time `json:"retry_at"`
	OutputRef *string `json:"output_ref"`
	QualityStatus *string `json:"quality_status"`
	ReasonCodes []string `json:"reason_codes"`
	RiskDecision *string `json:"risk_decision"`
	CreatedAt time.Time `json:"created_at"`
	StartedAt *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type Attempt struct {
	SchemaVersion string `json:"schema_version"`
	AttemptRef string `json:"attempt_ref"`
	StageRef string `json:"stage_ref"`
	AttemptNo int `json:"attempt_no"`
	Status domain.AttemptStatus `json:"status"`
	LeaseOwner *string `json:"lease_owner"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at"`
	FencingToken uint64 `json:"fencing_token"`
	StartedAt time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	FaultRef *string `json:"fault_ref"`
	OutputRef *string `json:"output_ref"`
}

func RunFromDomain(v domain.Run) Run {
	return Run{SchemaVersion:v.SchemaVersion,RunRef:v.Ref,Status:v.Status,Version:v.Version,SystemAsOf:v.SystemAsOf,DeadlineAt:v.DeadlineAt,TemplateRef:v.TemplateRef,TemplateVersion:v.TemplateVersion,TemplateFingerprint:v.TemplateFingerprint,InputSnapshotRefs:cloneStrings(v.InputSnapshotRefs),ResultSummary:v.ResultSummary,OriginatingRequestID:v.OriginatingRequestID,CorrelationID:v.CorrelationID,TerminationReason:stringPointer(v.TerminationReason),CreatedAt:v.CreatedAt,StartedAt:v.StartedAt,FinishedAt:v.FinishedAt}
}
func StageFromDomain(v domain.Stage, schemaVersion string) Stage {
	return Stage{SchemaVersion:schemaVersion,StageRef:v.Ref,RunRef:v.RunRef,StageCode:v.Code,Status:v.Status,Version:v.Version,Required:v.Required,DependencyStageRefs:cloneStrings(v.Dependencies),RetryPolicyRef:v.RetryPolicyRef,AttemptNo:v.AttemptNo,RetryAt:v.RetryAt,OutputRef:v.OutputRef,QualityStatus:stringPointer(v.QualityStatus),ReasonCodes:cloneStrings(v.ReasonCodes),RiskDecision:stringPointer(v.RiskDecision),CreatedAt:v.CreatedAt,StartedAt:v.StartedAt,FinishedAt:v.FinishedAt}
}
func AttemptFromDomain(v domain.Attempt, schemaVersion string) Attempt {
	return Attempt{SchemaVersion:schemaVersion,AttemptRef:v.Ref,StageRef:v.StageRef,AttemptNo:v.Number,Status:v.Status,LeaseOwner:stringPointer(v.LeaseOwner),LeaseExpiresAt:v.LeaseExpiresAt,FencingToken:v.FencingToken,StartedAt:v.StartedAt,FinishedAt:v.FinishedAt,FaultRef:v.FaultRef,OutputRef:v.OutputRef}
}
func cloneStrings(v []string) []string { if v==nil{return []string{}}; return append([]string(nil),v...) }
func stringPointer(v string) *string { if v=="" { return nil }; return &v }
