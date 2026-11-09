package diagnostics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
)

type AssessmentStatus string

const (
	AssessmentPass          AssessmentStatus = "pass"
	AssessmentDegraded      AssessmentStatus = "degraded"
	AssessmentFail          AssessmentStatus = "fail"
	AssessmentNotComparable AssessmentStatus = "not_comparable"
)

type DiagnosticKind string

const (
	KindStageExecution DiagnosticKind = "stage_execution"
	KindDependency     DiagnosticKind = "dependency"
	KindQuality        DiagnosticKind = "quality"
	KindLLM            DiagnosticKind = "llm"
	KindResult         DiagnosticKind = "result"
)

// StageAssessment 是 Researchflow 领域诊断的安全投影输入。
// SafeMessage 必须是可公开展示的摘要；不得传入原始 Provider/LLM 错误。
type StageAssessment struct {
	StageRef   string
	StageCode  string
	Kind       DiagnosticKind
	Required   bool
	Status     AssessmentStatus
	ReasonCode string
	SafeMessage string
	EvidenceRef string
	CheckedAt  time.Time
	Duration   time.Duration
	Attributes map[string]string
}

// NewStageCheck 将 Researchflow 的四态诊断映射到公共 Platform Diagnostics。
// not_comparable 映射为 degraded，并通过 assessment_status 保留原始语义，
// 避免把“无法比较”误报成健康、跳过或硬失败。
func NewStageCheck(assessment StageAssessment) (platformdiagnostics.Check, error) {
	if err := validateAssessment(assessment); err != nil {
		return platformdiagnostics.Check{}, err
	}
	observation := platformdiagnostics.Observation{
		Status: platformStatus(assessment.Status),
		ReasonCode: assessment.ReasonCode,
		SafeMessage: assessment.SafeMessage,
		CheckedAt: assessment.CheckedAt,
		Duration: assessment.Duration,
		EvidenceRef: assessment.EvidenceRef,
		Attributes: cloneAttributes(assessment.Attributes),
	}
	if observation.Attributes == nil {
		observation.Attributes = make(map[string]string, 4)
	}
	observation.Attributes["assessment_status"] = string(assessment.Status)
	observation.Attributes["diagnostic_kind"] = string(assessment.Kind)
	observation.Attributes["stage_ref"] = assessment.StageRef
	observation.Attributes["stage_code"] = assessment.StageCode
	if err := observation.Validate(); err != nil {
		return platformdiagnostics.Check{}, fmt.Errorf("validate Researchflow diagnostic observation: %w", err)
	}
	sum := sha256.Sum256([]byte(string(assessment.Kind) + ":" + assessment.StageRef))
	checkID := "researchflow." + string(assessment.Kind) + "." + hex.EncodeToString(sum[:])
	return platformdiagnostics.Check{
		ID: checkID, Name: "Researchflow " + string(assessment.Kind),
		Required: assessment.Required,
		Run: func(context.Context) (platformdiagnostics.Observation, error) {
			copy := observation
			copy.Attributes = cloneAttributes(observation.Attributes)
			return copy, nil
		},
	}, nil
}

func validateAssessment(value StageAssessment) error {
	if strings.TrimSpace(value.StageRef) == "" || len(value.StageRef) > 255 ||
		strings.TrimSpace(value.StageRef) != value.StageRef ||
		strings.TrimSpace(value.StageCode) == "" || len(value.StageCode) > 128 ||
		strings.TrimSpace(value.StageCode) != value.StageCode {
		return errors.New("Researchflow diagnostic stage identity is invalid")
	}
	switch value.Kind {
	case KindStageExecution, KindDependency, KindQuality, KindLLM, KindResult:
	default:
		return errors.New("Researchflow diagnostic kind is invalid")
	}
	switch value.Status {
	case AssessmentPass:
	case AssessmentDegraded, AssessmentFail, AssessmentNotComparable:
		if strings.TrimSpace(value.ReasonCode) == "" {
			return errors.New("non-pass Researchflow assessment requires a reason code")
		}
	default:
		return errors.New("Researchflow assessment status is invalid")
	}
	if value.CheckedAt.IsZero() || value.Duration < 0 {
		return errors.New("Researchflow diagnostic timestamp or duration is invalid")
	}
	if len(value.Attributes) > 12 {
		return errors.New("too many Researchflow diagnostic attributes")
	}
	return nil
}

func platformStatus(status AssessmentStatus) platformdiagnostics.Status {
	switch status {
	case AssessmentPass:
		return platformdiagnostics.StatusHealthy
	case AssessmentFail:
		return platformdiagnostics.StatusFailed
	case AssessmentDegraded, AssessmentNotComparable:
		return platformdiagnostics.StatusDegraded
	default:
		return platformdiagnostics.StatusFailed
	}
}

func cloneAttributes(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
