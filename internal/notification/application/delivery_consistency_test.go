package application

import (
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestAssessDeliveryConsistency(t *testing.T) {
	delivery, lease, _ := recoveryFixture(t)
	initial := validDeliveryForRepository()
	sendingSnapshot, sendingEvent, err := initial.Transition(domain.DeliveryTransition{
		To: domain.DeliverySending, At: initial.CreatedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	running := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAccepted, false)
	completed := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAccepted, true)

	tests := []struct {
		name       string
		input      DeliveryConsistencyInput
		want       DeliveryConsistencyStatus
		wantReason string
	}{
		{
			name: "all histories comparable and consistent",
			input: DeliveryConsistencyInput{Delivery: initial, AttemptsComparable: true, StateEventsComparable: true},
			want: DeliveryConsistencyPass,
		},
		{
			name: "attempt delivery intermediate",
			input: DeliveryConsistencyInput{
				Delivery: func() domain.DeliveryRecord { value := sendingSnapshot; value.AttemptCount = 1; return value }(),
				Attempts: []domain.DeliveryAttempt{completed}, AttemptsComparable: true,
				StateEvents: []domain.DeliveryStateEvent{sendingEvent}, StateEventsComparable: true,
			},
			want: DeliveryConsistencyDegrade, wantReason: "attempt_delivery_intermediate",
		},
		{
			name: "attempt snapshot contradiction",
			input: DeliveryConsistencyInput{
				Delivery: func() domain.DeliveryRecord { value := sendingSnapshot; value.Status = domain.DeliverySent; value.AttemptCount = 1; return value }(),
				Attempts: []domain.DeliveryAttempt{running}, AttemptsComparable: true,
				StateEventsComparable: false,
			},
			want: DeliveryConsistencyFail, wantReason: "attempt_history_conflict",
		},
		{
			name: "state history invalid",
			input: DeliveryConsistencyInput{
				Delivery: initial, AttemptsComparable: false, StateEvents: []domain.DeliveryStateEvent{{}},
				StateEventsComparable: true,
			},
			want: DeliveryConsistencyFail, wantReason: "state_event_history_conflict",
		},
		{
			name: "history read unavailable",
			input: DeliveryConsistencyInput{Delivery: initial},
			want: DeliveryConsistencyNotComparable,
		},
		{
			name: "known conflict dominates unavailable history",
			input: DeliveryConsistencyInput{
				Delivery: func() domain.DeliveryRecord { value := delivery; value.Status = domain.DeliverySent; value.AttemptCount = 1; return value }(),
				Attempts: []domain.DeliveryAttempt{running}, AttemptsComparable: true,
			},
			want: DeliveryConsistencyFail, wantReason: "attempt_history_conflict",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AssessDeliveryConsistency(tc.input)
			if got.Status != tc.want {
				t.Fatalf("status=%q want=%q reasons=%v", got.Status, tc.want, got.ReasonCodes)
			}
			if got.Evidence.DeliveryStatus != tc.input.Delivery.Status ||
				got.Evidence.DeliveryVersion != tc.input.Delivery.Version ||
				got.Evidence.ObservedAttemptCount != len(tc.input.Attempts) ||
				got.Evidence.StateEventCount != len(tc.input.StateEvents) {
				t.Fatalf("unexpected safe evidence summary: %+v", got.Evidence)
			}
			if tc.wantReason != "" {
				found := false
				for _, reason := range got.ReasonCodes {
					if reason == tc.wantReason {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("missing reason %q in %v", tc.wantReason, got.ReasonCodes)
				}
			}
		})
	}
}
