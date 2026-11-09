package application

import (
	"context"
	"testing"
	"time"
)

type outboxDeliveryRepoFake struct {
	claimed []NotificationEventEnvelope
	claimWorker string
	claimLimit int
	leaseUntil time.Time
	confirmedEvent string
	confirmedToken string
	confirmedWorker string
	confirmedAt time.Time
	retriedEvent string
	retriedWorker string
	retriedToken string
	retryAt time.Time
	retryReason string
	retryUpdatedAt time.Time
	rejectedEvent string
	rejectedWorker string
	rejectedToken string
	rejectedReason string
	rejectedAt time.Time
	err error
}

func (f *outboxDeliveryRepoFake) ClaimOutbox(_ context.Context, worker string, limit int, leaseUntil time.Time) ([]NotificationEventEnvelope,error) {
	f.claimWorker=worker; f.claimLimit=limit; f.leaseUntil=leaseUntil
	return f.claimed,f.err
}
func (f *outboxDeliveryRepoFake) MarkOutboxPublished(_ context.Context,event,worker,token string,at time.Time) error {
	f.confirmedEvent=event; f.confirmedWorker=worker; f.confirmedToken=token; f.confirmedAt=at; return f.err
}
func (f *outboxDeliveryRepoFake) ScheduleOutboxRetry(_ context.Context,event,worker,token string,at time.Time,reason string,updated time.Time) error {
	f.retriedEvent=event; f.retriedWorker=worker; f.retriedToken=token; f.retryAt=at; f.retryReason=reason; f.retryUpdatedAt=updated; return f.err
}
func (f *outboxDeliveryRepoFake) RejectOutbox(_ context.Context,event,worker,token,reason string,at time.Time) error {
	f.rejectedEvent=event; f.rejectedWorker=worker; f.rejectedToken=token; f.rejectedReason=reason; f.rejectedAt=at; return f.err
}

func TestOutboxServiceUsesStableLeaseAndDeliveryCommands(t *testing.T) {
	now:=time.Date(2026,11,9,5,0,0,0,time.UTC)
	repo:=&outboxDeliveryRepoFake{}
	service,err:=NewOutboxService(repo,func()time.Time{return now})
	if err!=nil { t.Fatal(err) }
	if _,err:=service.Claim(context.Background(),"worker_1",10,30*time.Second);err!=nil { t.Fatal(err) }
	if repo.claimWorker!="worker_1" || repo.claimLimit!=10 || !repo.leaseUntil.Equal(now.Add(30*time.Second)) {
		t.Fatalf("unexpected claim request: %+v",repo)
	}
	if err:=service.Confirm(context.Background(),"nte_1","worker_1","0123456789abcdef0123456789abcdef");err!=nil { t.Fatal(err) }
	if repo.confirmedEvent!="nte_1" || repo.confirmedToken=="" || !repo.confirmedAt.Equal(now) { t.Fatalf("unexpected confirmation: %+v",repo) }
	if err:=service.Retry(context.Background(),"nte_2","worker_1","0123456789abcdef0123456789abcdef",time.Minute,"notification_unavailable");err!=nil { t.Fatal(err) }
	if repo.retryReason!="notification_unavailable" || repo.retriedToken=="" || !repo.retryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected retry request: %+v",repo)
	}
	if err:=service.Reject(context.Background(),"nte_3","worker_1","0123456789abcdef0123456789abcdef","notification_schema_rejected");err!=nil { t.Fatal(err) }
	if repo.rejectedEvent!="nte_3" || repo.rejectedToken=="" || repo.rejectedReason!="notification_schema_rejected" || !repo.rejectedAt.Equal(now) {
		t.Fatalf("unexpected rejection request: %+v",repo)
	}
}

func TestOutboxServiceRejectsUnsafeClaimAndRetryParameters(t *testing.T) {
	now:=time.Date(2026,11,9,5,0,0,0,time.UTC)
	service,err:=NewOutboxService(&outboxDeliveryRepoFake{},func()time.Time{return now})
	if err!=nil { t.Fatal(err) }
	if _,err:=service.Claim(context.Background(),"bad worker",10,time.Minute);err==nil { t.Fatal("unsafe worker ID must be rejected") }
	if _,err:=service.Claim(context.Background(),"worker_1",101,time.Minute);err==nil { t.Fatal("oversized batch must be rejected") }
	if _,err:=service.Claim(context.Background(),"worker_1",10,0);err==nil { t.Fatal("zero lease must be rejected") }
	if err:=service.Retry(context.Background(),"nte_1","worker_1","0123456789abcdef0123456789abcdef",time.Minute,"invalid reason");err==nil { t.Fatal("unsafe retry reason must be rejected") }
}
