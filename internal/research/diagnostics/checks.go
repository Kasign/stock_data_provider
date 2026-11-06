// Package diagnostics adapts Research domain snapshots to the Platform diagnostics contract.
package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
)

func NewContextCheck(id, name string, required bool, value researchcontext.Context) (platformdiagnostics.Check, error) {
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
			attributes := map[string]string{
				"quality_status": string(snapshot.QualitySummary.Status),
				"context_version": strconv.FormatInt(snapshot.ContextVersion, 10),
			}
			switch snapshot.QualitySummary.Status {
			case researchcontext.QualityValid:
				return healthyObservation("Research Context 校验通过", attributes), nil
			case researchcontext.QualityDegraded:
				return platformdiagnostics.Observation{
					Status: platformdiagnostics.StatusDegraded,
					ReasonCode: "research.context.degraded",
					SafeMessage: "Research Context 存在已记录的质量降级",
					Attributes: attributes,
				}, nil
			case researchcontext.QualityUnknown:
				return platformdiagnostics.Observation{
					Status: platformdiagnostics.StatusUnavailable,
					ReasonCode: "research.context.quality_unknown",
					SafeMessage: "Research Context 质量状态不可比较",
					Attributes: attributes,
				}, nil
			default:
				return failedObservation("research.context.quality_invalid", "Research Context 质量状态无效", attributes), nil
			}
		},
	}, nil
}

func NewEventCheck(id, name string, required bool, value researchevent.ResearchEvent) (platformdiagnostics.Check, error) {
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
			return healthyObservation("Research Event 校验通过",
				map[string]string{"event_version": strconv.Itoa(snapshot.EventVersion)}), nil
		},
	}, nil
}

func NewClaimCheck(id, name string, required bool, value researchclaim.Claim) (platformdiagnostics.Check, error) {
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
		Status: platformdiagnostics.StatusHealthy,
		SafeMessage: message,
		Attributes: attributes,
	}
}

func failedObservation(code, message string, attributes map[string]string) platformdiagnostics.Observation {
	return platformdiagnostics.Observation{
		Status: platformdiagnostics.StatusFailed,
		ReasonCode: code,
		SafeMessage: message,
		Attributes: attributes,
	}
}
