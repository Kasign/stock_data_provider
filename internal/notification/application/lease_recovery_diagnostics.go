package application

import (
	"fmt"
	"strconv"
	"time"

	"example.com/stock-analysis-flow/internal/platform/diagnostics"
)

// ToDiagnosticsObservation 将批次报告映射为公共 Platform Diagnostics 观察结果。
// 该映射只表达本批次执行质量，不把提交不确定解释为可重试 Fault。
func (r LeaseRecoveryReport) ToDiagnosticsObservation(checkedAt time.Time, duration time.Duration) (diagnostics.Observation, error) {
	counts := []int{r.Scanned, r.Recovered, r.Conflicted, r.OutcomeUnknown, r.Failed, r.Unprocessed}
	for _, count := range counts {
		if count < 0 {
			return diagnostics.Observation{}, fmt.Errorf("notification: recovery report contains a negative count")
		}
	}
	if r.Scanned != r.Recovered+r.Conflicted+r.OutcomeUnknown+r.Failed+r.Unprocessed ||
		r.OutcomeUnknown != len(r.OutcomeUnknownDeliveryIDs) {
		return diagnostics.Observation{}, fmt.Errorf("notification: recovery report counts do not reconcile")
	}
	if checkedAt.IsZero() || duration < 0 {
		return diagnostics.Observation{}, fmt.Errorf("notification: invalid recovery diagnostic timing")
	}

	observation := diagnostics.Observation{
		Status: diagnostics.StatusHealthy,
		SafeMessage: "lease recovery batch completed",
		CheckedAt: checkedAt.UTC(),
		Duration: duration,
		Attributes: map[string]string{
			"scanned": strconv.Itoa(r.Scanned),
			"recovered": strconv.Itoa(r.Recovered),
			"conflicted": strconv.Itoa(r.Conflicted),
			"outcome_unknown": strconv.Itoa(r.OutcomeUnknown),
			"failed": strconv.Itoa(r.Failed),
			"unprocessed": strconv.Itoa(r.Unprocessed),
		},
	}
	switch {
	case r.Failed > 0:
		observation.Status = diagnostics.StatusFailed
		observation.ReasonCode = "notification.lease_recovery_failed"
		observation.SafeMessage = "one or more lease recovery items failed"
	case r.OutcomeUnknown > 0:
		observation.Status = diagnostics.StatusDegraded
		observation.ReasonCode = "notification.persistence_outcome_unknown"
		observation.SafeMessage = "one or more persistence outcomes require snapshot reconciliation"
	case r.Conflicted > 0:
		observation.Status = diagnostics.StatusDegraded
		observation.ReasonCode = "notification.lease_recovery_conflict"
		observation.SafeMessage = "one or more lease recovery items changed concurrently"
	case r.Unprocessed > 0:
		observation.Status = diagnostics.StatusDegraded
		observation.ReasonCode = "notification.lease_recovery_incomplete"
		observation.SafeMessage = "one or more lease recovery candidates were not processed"
	}
	if err := observation.Validate(); err != nil {
		return diagnostics.Observation{}, fmt.Errorf("notification: invalid recovery diagnostic observation: %w", err)
	}
	return observation, nil
}
