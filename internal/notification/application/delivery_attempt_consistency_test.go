package application

import (
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestAssessDeliveryAttemptConsistencyDistinguishesIntermediateAndConflict(t *testing.T) {
	delivery, lease, _ := recoveryFixture(t)
	running := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAccepted, false)

	cases := []struct {
		name string
		status domain.DeliveryStatus
		attempts []domain.DeliveryAttempt
		want DeliveryAttemptConsistency
		wantErr bool
	}{
		{name:"sending without attempt", status:domain.DeliverySending, attempts:nil, want:DeliveryAttemptConsistencyConsistent},
		{name:"sending with running attempt", status:domain.DeliverySending, attempts:[]domain.DeliveryAttempt{running}, want:DeliveryAttemptConsistencyConsistent},
		{name:"unknown with running attempt", status:domain.DeliveryUnknown, attempts:[]domain.DeliveryAttempt{running}, want:DeliveryAttemptConsistencyConsistent},
		{name:"sent with running attempt", status:domain.DeliverySent, attempts:[]domain.DeliveryAttempt{running}, wantErr:true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot:=delivery
			snapshot.Status=tc.status
			snapshot.AttemptCount=len(tc.attempts)
			if tc.status==domain.DeliverySending { snapshot.AttemptCount=len(tc.attempts) }
			got,err:=AssessDeliveryAttemptConsistency(snapshot,tc.attempts)
			if tc.wantErr {
				if !errors.Is(err,ErrDeliveryAttemptConsistency) { t.Fatalf("expected conflict, got %v",err) }
				return
			}
			if err!=nil || got!=tc.want { t.Fatalf("got=%q err=%v want=%q",got,err,tc.want) }
		})
	}
}
