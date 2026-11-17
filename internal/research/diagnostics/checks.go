// Package diagnostics adapts Research domain snapshots to the Platform diagnostics contract.
package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

func NewContextCheck(id, name string, required bool, value researchcontext.Context, runAsOf time.Time) (platformdiagnostics.Check, error) {
	if err := validateRunSystemAsOf(runAsOf); err != nil {
		return platformdiagnostics.Check{}, err
	}
	snapshot, err := cloneSnapshot(value)
	if err != nil {
		return platformdiagnostics.Check{}, fmt.Errorf("snapshot Research Context: %w", err)
	}
	return platformdiagnostics.Check{
		ID: id, Name: name, Required: required,
		Run: func(ctx context.Context) (platformdiagnostics.Observation, error) {
			if err := ctx.Err(); err != nil {
				return platformdiagnostics.Observation{}, err
			}
			if err := snapshot.Validate(); err != nil {
				return failedObservation("research.context.invalid", "Research Context 结构校验失败",
					map[string]string{"context_version": strconv.FormatInt(snapshot.ContextVersion, 10)}), nil
			}
			if err := snapshot.ValidateSystemAsOf(runAsOf); err != nil {
				return failedObservation("research.context.run_cutoff_mismatch", "Research Context system_as_of 与 Run 截止时间不一致",
					map[string]string{"context_version": strconv.FormatInt(snapshot.ContextVersion, 10)}), nil
			}
			attributes := map[string]string{
				"quality_status":  string(snapshot.QualitySummary.Status),
				"context_version": strconv.FormatInt(snapshot.ContextVersion, 10),
			}
			switch snapshot.QualitySummary.Status {
			case researchcontext.QualityValid:
				return healthyObservation("Research Context 校验通过", attributes), nil
			case researchcontext.QualityDegraded:
				return platformdiagnostics.Observation{
					Status:      platformdiagnostics.StatusDegraded,
					ReasonCode:  "research.context.degraded",
					SafeMessage: "Research Context 存在已记录的质量降级",
					Attributes:  attributes,
				}, nil
			case researchcontext.QualityUnknown:
				return platformdiagnostics.Observation{
					Status:      platformdiagnostics.StatusUnavailable,
					ReasonCode:  "research.context.quality_unknown",
					SafeMessage: "Research Context 质量状态不可比较",
					Attributes:  attributes,
				}, nil
			default:
				return failedObservation("research.context.quality_invalid", "Research Context 质量状态无效", attributes), nil
			}
		},
	}, nil
}

func NewEventCheck(id, name string, required bool, value researchevent.ResearchEvent, runAsOf time.Time) (platformdiagnostics.Check, error) {
	if err := validateRunSystemAsOf(runAsOf); err != nil {
		return platformdiagnostics.Check{}, err
	}
	snapshot, err := cloneSnapshot(value)
	if err != nil {
		return platformdiagnostics.Check{}, fmt.Errorf("snapshot Research Event: %w", err)
	}
	return platformdiagnostics.Check{
		ID: id, Name: name, Required: required,
		Run: func(ctx context.Context) (platformdiagnostics.Observation, error) {
			if err := ctx.Err(); err != nil {
				return platformdiagnostics.Observation{}, err
			}
			if err := snapshot.Validate(); err != nil {
				return failedObservation("research.event.invalid", "Research Event 结构校验失败",
					map[string]string{"event_version": strconv.Itoa(snapshot.EventVersion)}), nil
			}
			if snapshot.SystemAsOf.After(runAsOf) {
				return failedObservation("research.event.pit_invalid", "Research Event 晚于 Run 知识截止时间",
					map[string]string{"event_version": strconv.Itoa(snapshot.EventVersion)}), nil
			}
			return healthyObservation("Research Event 校验通过",
				map[string]string{"event_version": strconv.Itoa(snapshot.EventVersion)}), nil
		},
	}, nil
}

func NewClaimCheck(id, name string, required bool, value researchclaim.Claim, runAsOf time.Time) (platformdiagnostics.Check, error) {
	if err := validateRunSystemAsOf(runAsOf); err != nil {
		return platformdiagnostics.Check{}, err
	}
	snapshot, err := cloneSnapshot(value)
	if err != nil {
		return platformdiagnostics.Check{}, fmt.Errorf("snapshot Research Claim: %w", err)
	}
	return platformdiagnostics.Check{
		ID: id, Name: name, Required: required,
		Run: func(ctx context.Context) (platformdiagnostics.Observation, error) {
			if err := ctx.Err(); err != nil {
				return platformdiagnostics.Observation{}, err
			}
			if err := snapshot.Validate(); err != nil {
				return failedObservation("research.claim.invalid", "Research Claim 结构校验失败",
					map[string]string{"claim_version": strconv.Itoa(snapshot.ClaimVersion)}), nil
			}
			if snapshot.SystemAsOf.After(runAsOf) {
				return failedObservation("research.claim.pit_invalid", "Research Claim 晚于 Run 知识截止时间",
					map[string]string{"claim_version": strconv.Itoa(snapshot.ClaimVersion)}), nil
			}
			return healthyObservation("Research Claim 校验通过",
				map[string]string{"claim_version": strconv.Itoa(snapshot.ClaimVersion)}), nil
		},
	}, nil
}

func cloneSnapshot[T any](value T) (T, error) {
	var snapshot T
	payload, err := json.Marshal(value)
	if err != nil {
		return snapshot, err
	}
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func healthyObservation(message string, attributes map[string]string) platformdiagnostics.Observation {
	return platformdiagnostics.Observation{
		Status:      platformdiagnostics.StatusHealthy,
		SafeMessage: message,
		Attributes:  attributes,
	}
}

func failedObservation(code, message string, attributes map[string]string) platformdiagnostics.Observation {
	return platformdiagnostics.Observation{
		Status:      platformdiagnostics.StatusFailed,
		ReasonCode:  code,
		SafeMessage: message,
		Attributes:  attributes,
	}
}

// NewEvidenceCheck 只输出质量和来源元数据，不暴露 Evidence 正文、引用列表或授权快照。
func NewEvidenceCheck(id, name string, required bool, value researchevidence.Evidence, runAsOf time.Time) (platformdiagnostics.Check, error) {
	if err := validateRunSystemAsOf(runAsOf); err != nil {
		return platformdiagnostics.Check{}, err
	}
	snapshot, err := cloneSnapshot(value)
	if err != nil {
		return platformdiagnostics.Check{}, fmt.Errorf("snapshot Research Evidence: %w", err)
	}
	return platformdiagnostics.Check{
		ID: id, Name: name, Required: required,
		Run: func(ctx context.Context) (platformdiagnostics.Observation, error) {
			if err := ctx.Err(); err != nil {
				return platformdiagnostics.Observation{}, err
			}
			if err := snapshot.Validate(); err != nil {
				return failedObservation("research.evidence.invalid", "Research Evidence 结构校验失败",
					map[string]string{"quality_status": string(snapshot.QualityStatus)}), nil
			}
			attributes := map[string]string{
				"source_owner":   string(snapshot.SourceOwner),
				"quality_status": string(snapshot.QualityStatus),
				"content_level":  string(snapshot.ContentLevel),
			}
			if snapshot.SystemAsOf.After(runAsOf) {
				return failedObservation("research.evidence.pit_invalid", "Research Evidence 不满足严格 PIT 约束", attributes), nil
			}
			if snapshot.AvailableAt == nil {
				if snapshot.QualityStatus == researchevidence.QualityInvalid {
					return failedObservation("research.evidence.quality_invalid", "Research Evidence 质量状态无效", attributes), nil
				}
				return platformdiagnostics.Observation{
					Status:      platformdiagnostics.StatusUnavailable,
					ReasonCode:  "research.evidence.pit_unavailable",
					SafeMessage: "Research Evidence 缺少可验证的 available_at，无法进行严格 PIT 比较",
					Attributes:  attributes,
				}, nil
			}
			if err := snapshot.ValidateForStrictPIT(runAsOf); err != nil {
				return failedObservation("research.evidence.pit_invalid", "Research Evidence 不满足严格 PIT 约束", attributes), nil
			}
			switch snapshot.QualityStatus {
			case researchevidence.QualityValid:
				return healthyObservation("Research Evidence 结构与严格 PIT 校验通过", attributes), nil
			case researchevidence.QualityDegraded:
				return platformdiagnostics.Observation{
					Status:      platformdiagnostics.StatusDegraded,
					ReasonCode:  "research.evidence.degraded",
					SafeMessage: "Research Evidence 存在已记录的质量降级",
					Attributes:  attributes,
				}, nil
			case researchevidence.QualityUnknown:
				return platformdiagnostics.Observation{
					Status:      platformdiagnostics.StatusUnavailable,
					ReasonCode:  "research.evidence.quality_unknown",
					SafeMessage: "Research Evidence 质量状态不可比较",
					Attributes:  attributes,
				}, nil
			default:
				return failedObservation("research.evidence.quality_invalid", "Research Evidence 质量状态无效", attributes), nil
			}
		},
	}, nil
}

func validateRunSystemAsOf(value time.Time) error {
	if value.IsZero() || value.Location() != time.UTC {
		return errors.New("Run system_as_of is required and must use UTC")
	}
	return nil
}
